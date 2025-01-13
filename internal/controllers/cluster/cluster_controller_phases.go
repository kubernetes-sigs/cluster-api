/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	"sigs.k8s.io/cluster-api/util/secret"
)

var externalReadyWait = 30 * time.Second

func (r *Reconciler) reconcilePhase(_ context.Context, cluster *clusterv1.Cluster) {
	preReconcilePhase := cluster.Status.GetTypedPhase()

	if cluster.Status.Phase == "" {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhasePending)
	}

	if cluster.Spec.InfrastructureRef != nil || cluster.Spec.ControlPlaneRef != nil {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseProvisioning)
	}

	if cluster.Status.InfrastructureReady && cluster.Spec.ControlPlaneEndpoint.IsValid() {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseProvisioned)
	}

	if cluster.Status.FailureReason != nil || cluster.Status.FailureMessage != nil {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseFailed)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseDeleting)
	}

	// Only record the event if the status has changed
	if preReconcilePhase != cluster.Status.GetTypedPhase() {
		// Failed clusters should get a Warning event
		if cluster.Status.GetTypedPhase() == clusterv1.ClusterPhaseFailed {
			r.recorder.Eventf(cluster, corev1.EventTypeWarning, string(cluster.Status.GetTypedPhase()), "Cluster %s is %s: %s", cluster.Name, string(cluster.Status.GetTypedPhase()), ptr.Deref(cluster.Status.FailureMessage, "unknown"))
		} else {
			r.recorder.Eventf(cluster, corev1.EventTypeNormal, string(cluster.Status.GetTypedPhase()), "Cluster %s is %s", cluster.Name, string(cluster.Status.GetTypedPhase()))
		}
	}
}

// reconcileExternal handles generic unstructured objects referenced by a Cluster.
func (r *Reconciler) reconcileExternal(ctx context.Context, cluster *clusterv1.Cluster, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	log := ctrl.LoggerFrom(ctx)

	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, ref); err != nil {
		if apierrors.IsNotFound(err) {
			// We want to surface the NotFound error only for the referenced object, so we use a generic error in case CRD is not found.
			return nil, errors.New(err.Error())
		}
		return nil, err
	}

	obj, err := external.Get(ctx, r.Client, ref)
	if err != nil {
		return nil, err
	}

	// Ensure we add a watcher to the external object.
	if err := r.externalTracker.Watch(log, obj, handler.EnqueueRequestForOwner(r.Client.Scheme(), r.Client.RESTMapper(), &clusterv1.Cluster{}), predicates.ResourceIsChanged(r.Client.Scheme(), *r.externalTracker.PredicateLogger)); err != nil {
		return nil, err
	}

	if err := ensureOwnerRefAndLabel(ctx, r.Client, obj, cluster); err != nil {
		return nil, err
	}

	// Set failure reason and message, if any.
	failureReason, failureMessage, err := external.FailuresFrom(obj)
	if err != nil {
		return nil, err
	}
	if failureReason != "" {
		clusterStatusError := capierrors.ClusterStatusError(failureReason)
		cluster.Status.FailureReason = &clusterStatusError
	}
	if failureMessage != "" {
		cluster.Status.FailureMessage = ptr.To(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), failureMessage),
		)
	}

	return obj, nil
}

func ensureOwnerRefAndLabel(ctx context.Context, c client.Client, obj *unstructured.Unstructured, cluster *clusterv1.Cluster) error {
	desiredOwnerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		Controller: ptr.To(true),
	}

	if util.HasExactOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef) &&
		obj.GetLabels()[clusterv1.ClusterNameLabel] == cluster.Name {
		return nil
	}

	patchHelper, err := patch.NewHelper(obj, c)
	if err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(cluster, obj, c.Scheme()); err != nil {
		return err
	}

	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[clusterv1.ClusterNameLabel] = cluster.Name
	obj.SetLabels(labels)

	return patchHelper.Patch(ctx, obj)
}

// reconcileInfrastructure reconciles the Spec.InfrastructureRef object on a Cluster.
func (r *Reconciler) reconcileInfrastructure(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster

	// If the infrastructure ref is not set, no-op.
	if s.cluster.Spec.InfrastructureRef == nil {
		// if the cluster is not deleted, and the cluster is not using a ClusterClass, mark the infrastructure as ready to unblock other provisioning workflows.
		if s.cluster.DeletionTimestamp.IsZero() {
			cluster.Status.InfrastructureReady = true
			conditions.MarkTrue(cluster, clusterv1.InfrastructureReadyCondition)
		}
		return ctrl.Result{}, nil
	}

	// Call generic external reconciler.
	obj, err := r.reconcileExternal(ctx, cluster, cluster.Spec.InfrastructureRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.infraClusterIsNotFound = true

			if !cluster.DeletionTimestamp.IsZero() {
				// Tolerate infra cluster not found when the cluster is being deleted.
				return ctrl.Result{}, nil
			}

			if cluster.Status.InfrastructureReady {
				// Infra object went missing after the cluster was up and running
				return ctrl.Result{}, errors.Errorf("%s has been deleted after being ready", cluster.Spec.InfrastructureRef.Kind)
			}
			log.Info(fmt.Sprintf("Could not find %s, requeuing", cluster.Spec.InfrastructureRef.Kind))
			return ctrl.Result{RequeueAfter: externalReadyWait}, nil
		}
		return ctrl.Result{}, err
	}
	s.infraCluster = obj

	// Determine if the infrastructure provider is ready.
	ready, err := external.IsReady(s.infraCluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	if ready && !cluster.Status.InfrastructureReady {
		log.Info("Infrastructure provider has completed cluster infrastructure provisioning and reports status.ready", cluster.Spec.InfrastructureRef.Kind, klog.KObj(s.infraCluster))
	}

	// Report a summary of current status of the infrastructure object defined for this cluster.
	fallBack := conditions.WithFallbackValue(ready, clusterv1.WaitingForInfrastructureFallbackReason, clusterv1.ConditionSeverityInfo, "")
	if !s.cluster.DeletionTimestamp.IsZero() {
		fallBack = conditions.WithFallbackValue(false, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
	}
	conditions.SetMirror(cluster, clusterv1.InfrastructureReadyCondition,
		conditions.UnstructuredGetter(s.infraCluster),
		fallBack,
	)

	// There's no need to go any further if the infrastructure object is marked for deletion.
	if !s.infraCluster.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// If the infrastructure provider is not ready (and it wasn't ready before), return early.
	if !ready && !cluster.Status.InfrastructureReady {
		log.V(3).Info("Infrastructure provider is not ready yet")
		return ctrl.Result{}, nil
	}

	// Get and parse Spec.ControlPlaneEndpoint field from the infrastructure provider.
	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		if err := util.UnstructuredUnmarshalField(s.infraCluster, &cluster.Spec.ControlPlaneEndpoint, "spec", "controlPlaneEndpoint"); err != nil && err != util.ErrUnstructuredFieldNotFound {
			return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve Spec.ControlPlaneEndpoint from infrastructure provider for Cluster %q in namespace %q",
				cluster.Name, cluster.Namespace)
		}
	}

	// Get and parse Status.FailureDomains from the infrastructure provider.
	failureDomains := clusterv1.FailureDomains{}
	if err := util.UnstructuredUnmarshalField(s.infraCluster, &failureDomains, "status", "failureDomains"); err != nil && err != util.ErrUnstructuredFieldNotFound {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve Status.FailureDomains from infrastructure provider for Cluster %q in namespace %q",
			cluster.Name, cluster.Namespace)
	}
	cluster.Status.FailureDomains = failureDomains

	// Only record the event if the status has changed
	if !cluster.Status.InfrastructureReady {
		r.recorder.Eventf(cluster, corev1.EventTypeNormal, "InfrastructureReady", "Cluster %s InfrastructureReady is now True", cluster.Name)
	}
	cluster.Status.InfrastructureReady = true

	return ctrl.Result{}, nil
}

// reconcileControlPlane reconciles the Spec.ControlPlaneRef object on a Cluster.
func (r *Reconciler) reconcileControlPlane(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster

	if cluster.Spec.ControlPlaneRef == nil {
		return ctrl.Result{}, nil
	}

	// Call generic external reconciler.
	obj, err := r.reconcileExternal(ctx, cluster, cluster.Spec.ControlPlaneRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.controlPlaneIsNotFound = true

			if !cluster.DeletionTimestamp.IsZero() {
				// Tolerate control plane not found when the cluster is being deleted.
				return ctrl.Result{}, nil
			}

			if cluster.Status.ControlPlaneReady {
				// Control plane went missing after the cluster was up and running
				return ctrl.Result{}, errors.Errorf("%s has been deleted after being ready", cluster.Spec.ControlPlaneRef.Kind)
			}
			log.Info(fmt.Sprintf("Could not find %s, requeuing", cluster.Spec.ControlPlaneRef.Kind))
			return ctrl.Result{RequeueAfter: externalReadyWait}, nil
		}
		return ctrl.Result{}, err
	}
	s.controlPlane = obj

	// Determine if the control plane provider is ready.
	ready, err := external.IsReady(s.controlPlane)
	if err != nil {
		return ctrl.Result{}, err
	}
	if ready && !cluster.Status.ControlPlaneReady {
		log.Info("ControlPlane provider has completed provisioning and reports status.ready", cluster.Spec.ControlPlaneRef.Kind, klog.KObj(s.controlPlane))
	}

	// Report a summary of current status of the control plane object defined for this cluster.
	fallBack := conditions.WithFallbackValue(ready, clusterv1.WaitingForControlPlaneFallbackReason, clusterv1.ConditionSeverityInfo, "")
	if !s.cluster.DeletionTimestamp.IsZero() {
		fallBack = conditions.WithFallbackValue(false, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
	}
	conditions.SetMirror(cluster, clusterv1.ControlPlaneReadyCondition,
		conditions.UnstructuredGetter(s.controlPlane),
		fallBack,
	)

	// There's no need to go any further if the control plane object is marked for deletion.
	if !s.controlPlane.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// Update cluster.Status.ControlPlaneInitialized if it hasn't already been set
	// Determine if the control plane provider is initialized.
	if !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		initialized, err := external.IsInitialized(s.controlPlane)
		if err != nil {
			return ctrl.Result{}, err
		}
		if initialized {
			conditions.MarkTrue(cluster, clusterv1.ControlPlaneInitializedCondition)
		} else {
			conditions.MarkFalse(cluster, clusterv1.ControlPlaneInitializedCondition, clusterv1.WaitingForControlPlaneProviderInitializedReason, clusterv1.ConditionSeverityInfo, "Waiting for control plane provider to indicate the control plane has been initialized")
		}
	}

	// If the control plane is not ready (and it wasn't ready before), return early.
	if !ready && !cluster.Status.ControlPlaneReady {
		log.V(3).Info("Control Plane provider is not ready yet")
		return ctrl.Result{}, nil
	}

	// Get and parse Spec.ControlPlaneEndpoint field from the control plane provider.
	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		if err := util.UnstructuredUnmarshalField(s.controlPlane, &cluster.Spec.ControlPlaneEndpoint, "spec", "controlPlaneEndpoint"); err != nil && err != util.ErrUnstructuredFieldNotFound {
			return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve Spec.ControlPlaneEndpoint from control plane provider for Cluster %q in namespace %q",
				cluster.Name, cluster.Namespace)
		}
	}

	// Only record the event if the status has changed
	if !cluster.Status.ControlPlaneReady {
		r.recorder.Eventf(cluster, corev1.EventTypeNormal, "ControlPlaneReady", "Cluster %s ControlPlaneReady is now True", cluster.Name)
	}
	cluster.Status.ControlPlaneReady = true

	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileKubeconfig(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster

	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		return ctrl.Result{}, nil
	}

	// Do not generate the Kubeconfig if there is a ControlPlaneRef, since the Control Plane provider is
	// responsible for the management of the Kubeconfig. We continue to manage it here only for backward
	// compatibility when a Control Plane provider is not in use.
	if cluster.Spec.ControlPlaneRef != nil {
		return ctrl.Result{}, nil
	}

	_, err := secret.Get(ctx, r.Client, util.ObjectKey(cluster), secret.Kubeconfig)
	switch {
	case apierrors.IsNotFound(err):
		if err := kubeconfig.CreateSecret(ctx, r.Client, cluster); err != nil {
			if err == kubeconfig.ErrDependentCertificateNotFound {
				log.Info("Could not find secret for cluster, requeuing", "Secret", secret.ClusterCA)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			return ctrl.Result{}, err
		}
	case err != nil:
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve Kubeconfig Secret for Cluster %q in namespace %q", cluster.Name, cluster.Namespace)
	}

	return ctrl.Result{}, nil
}
