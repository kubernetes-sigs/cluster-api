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
	"strings"
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

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	"sigs.k8s.io/cluster-api/util/secret"
)

var externalReadyWait = 30 * time.Second

// reconcileExternal handles generic unstructured objects referenced by a Cluster.
func (r *Reconciler) reconcileExternal(ctx context.Context, cluster *clusterv1.Cluster, ref clusterv1.ContractVersionedObjectReference) (*unstructured.Unstructured, error) {
	log := ctrl.LoggerFrom(ctx)

	obj, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, ref, cluster.Namespace)
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
		if cluster.Status.Deprecated == nil {
			cluster.Status.Deprecated = &clusterv1.ClusterDeprecatedStatus{}
		}
		if cluster.Status.Deprecated.V1Beta1 == nil {
			cluster.Status.Deprecated.V1Beta1 = &clusterv1.ClusterV1Beta1DeprecatedStatus{}
		}
		cluster.Status.Deprecated.V1Beta1.FailureReason = &clusterStatusError
	}
	if failureMessage != "" {
		if cluster.Status.Deprecated == nil {
			cluster.Status.Deprecated = &clusterv1.ClusterDeprecatedStatus{}
		}
		if cluster.Status.Deprecated.V1Beta1 == nil {
			cluster.Status.Deprecated.V1Beta1 = &clusterv1.ClusterV1Beta1DeprecatedStatus{}
		}
		cluster.Status.Deprecated.V1Beta1.FailureMessage = ptr.To(
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
		UID:        cluster.UID,
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
	if !s.cluster.Spec.InfrastructureRef.IsDefined() {
		// if the cluster is not deleted, and the cluster is not using a ClusterClass, mark the infrastructure as ready to unblock other provisioning workflows.
		if s.cluster.DeletionTimestamp.IsZero() {
			cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
			v1beta1conditions.MarkTrue(cluster, clusterv1.InfrastructureReadyV1Beta1Condition)
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

			if ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) {
				// Infra object went missing after the cluster was up and running
				return ctrl.Result{}, errors.Errorf("%s has been deleted after being provisioned", cluster.Spec.InfrastructureRef.Kind)
			}
			log.Info(fmt.Sprintf("Could not find %s, requeuing", cluster.Spec.InfrastructureRef.Kind))
			return ctrl.Result{RequeueAfter: externalReadyWait}, nil
		}
		return ctrl.Result{}, err
	}
	s.infraCluster = obj

	// Determine contract version used by the InfrastructureCluster.
	contractVersion, err := contract.GetContractVersion(ctx, r.Client, s.infraCluster.GroupVersionKind().GroupKind())
	if err != nil {
		return ctrl.Result{}, err
	}

	// Determine if the InfrastructureCluster is provisioned.
	var provisioned bool
	if provisionedPtr, err := contract.InfrastructureCluster().Provisioned(contractVersion).Get(s.infraCluster); err != nil {
		if !errors.Is(err, contract.ErrFieldNotFound) {
			return ctrl.Result{}, err
		}
	} else {
		provisioned = *provisionedPtr
	}
	if provisioned && !ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) {
		log.Info("Infrastructure provider has completed provisioning", cluster.Spec.InfrastructureRef.Kind, klog.KObj(s.infraCluster))
	}

	// Report a summary of current status of the infrastructure object defined for this cluster.
	fallBack := v1beta1conditions.WithFallbackValue(provisioned, clusterv1.WaitingForInfrastructureFallbackV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
	if !s.cluster.DeletionTimestamp.IsZero() {
		fallBack = v1beta1conditions.WithFallbackValue(false, clusterv1.DeletingV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
	}
	v1beta1conditions.SetMirror(cluster, clusterv1.InfrastructureReadyV1Beta1Condition,
		v1beta1conditions.UnstructuredGetter(s.infraCluster),
		fallBack,
	)

	// There's no need to go any further if the infrastructure object is marked for deletion.
	if !s.infraCluster.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// If the InfrastructureCluster is not provisioned (and it wasn't already provisioned before), return.
	if !provisioned && !ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) {
		log.V(3).Info("Infrastructure provider is not ready yet")
		return ctrl.Result{}, nil
	}

	// Get and parse Spec.ControlPlaneEndpoint field from the infrastructure provider.
	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		if endpoint, err := contract.InfrastructureCluster().ControlPlaneEndpoint().Get(obj); err == nil {
			cluster.Spec.ControlPlaneEndpoint = *endpoint
		} else {
			if !errors.Is(err, contract.ErrFieldNotFound) {
				return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve %s from infrastructure provider for Cluster %q in namespace %q",
					strings.Join(contract.InfrastructureCluster().ControlPlaneEndpoint().Path(), "."), cluster.Name, cluster.Namespace)
			}
			cluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{}
		}
	}

	// Get and parse Status.FailureDomains from the infrastructure provider.
	if failureDomains, err := contract.InfrastructureCluster().FailureDomains(contractVersion).Get(obj); err == nil {
		cluster.Status.FailureDomains = failureDomains
	} else {
		if !errors.Is(err, contract.ErrFieldNotFound) {
			return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve %s from infrastructure provider for Cluster %q in namespace %q",
				strings.Join(contract.InfrastructureCluster().FailureDomains(contractVersion).Path(), "."), cluster.Name, cluster.Namespace)
		}
		cluster.Status.FailureDomains = []clusterv1.FailureDomain{}
	}

	// Only record the event if the status has changed
	if !ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) {
		r.recorder.Eventf(cluster, corev1.EventTypeNormal, "InfrastructureReady", "Cluster %s InfrastructureProvisioned is now True", cluster.Name)
	}
	cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)

	return ctrl.Result{}, nil
}

// reconcileControlPlane reconciles the Spec.ControlPlaneRef object on a Cluster.
func (r *Reconciler) reconcileControlPlane(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster

	if !cluster.Spec.ControlPlaneRef.IsDefined() {
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

			if ptr.Deref(cluster.Status.Initialization.ControlPlaneInitialized, false) {
				// Control plane went missing after the cluster was up and running
				return ctrl.Result{}, errors.Errorf("%s has been deleted after being initialized", cluster.Spec.ControlPlaneRef.Kind)
			}
			log.Info(fmt.Sprintf("Could not find %s, requeuing", cluster.Spec.ControlPlaneRef.Kind))
			return ctrl.Result{RequeueAfter: externalReadyWait}, nil
		}
		return ctrl.Result{}, err
	}
	s.controlPlane = obj

	// Determine contract version used by the ControlPlane.
	contractVersion, err := contract.GetContractVersion(ctx, r.Client, s.controlPlane.GroupVersionKind().GroupKind())
	if err != nil {
		return ctrl.Result{}, err
	}

	// Determine if the ControlPlane is provisioned.
	var initialized bool
	if initializedPtr, err := contract.ControlPlane().Initialized(contractVersion).Get(s.controlPlane); err != nil {
		if !errors.Is(err, contract.ErrFieldNotFound) {
			return ctrl.Result{}, err
		}
	} else {
		initialized = *initializedPtr
	}
	if initialized && !ptr.Deref(cluster.Status.Initialization.ControlPlaneInitialized, false) {
		log.Info("ControlPlane has completed initialization", cluster.Spec.ControlPlaneRef.Kind, klog.KObj(s.controlPlane))
	}

	// Report a summary of current status of the control plane object defined for this cluster.
	fallBack := v1beta1conditions.WithFallbackValue(initialized, clusterv1.WaitingForControlPlaneFallbackV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
	if !s.cluster.DeletionTimestamp.IsZero() {
		fallBack = v1beta1conditions.WithFallbackValue(false, clusterv1.DeletingV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
	}
	v1beta1conditions.SetMirror(cluster, clusterv1.ControlPlaneReadyV1Beta1Condition,
		v1beta1conditions.UnstructuredGetter(s.controlPlane),
		fallBack,
	)

	// There's no need to go any further if the control plane object is marked for deletion.
	if !s.controlPlane.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// Update ControlPlaneInitializedV1Beta1Condition if it hasn't already been set.
	if !v1beta1conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedV1Beta1Condition) {
		if initialized {
			v1beta1conditions.MarkTrue(cluster, clusterv1.ControlPlaneInitializedV1Beta1Condition)
		} else {
			v1beta1conditions.MarkFalse(cluster, clusterv1.ControlPlaneInitializedV1Beta1Condition, clusterv1.WaitingForControlPlaneProviderInitializedV1Beta1Reason, clusterv1.ConditionSeverityInfo, "Waiting for control plane provider to indicate the control plane has been initialized")
		}
	}

	// If the control plane is not ready (and it wasn't ready before), return early.
	if !initialized && !ptr.Deref(cluster.Status.Initialization.ControlPlaneInitialized, false) {
		log.V(3).Info("Control Plane is not initialized yet")
		return ctrl.Result{}, nil
	}

	// Get and parse Spec.ControlPlaneEndpoint field from the control plane provider.
	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		if endpoint, err := contract.ControlPlane().ControlPlaneEndpoint().Get(obj); err == nil {
			cluster.Spec.ControlPlaneEndpoint = *endpoint
		} else {
			if !errors.Is(err, contract.ErrFieldNotFound) {
				return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve %s from control plane provider for Cluster %q in namespace %q",
					strings.Join(contract.ControlPlane().ControlPlaneEndpoint().Path(), "."), cluster.Name, cluster.Namespace)
			}
			cluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{}
		}
	}

	// Only record the event if the status has changed
	if !ptr.Deref(cluster.Status.Initialization.ControlPlaneInitialized, false) {
		r.recorder.Eventf(cluster, corev1.EventTypeNormal, "ControlPlaneInitialized", "Cluster %s ControlPlaneInitialized is now True", cluster.Name)
	}
	cluster.Status.Initialization.ControlPlaneInitialized = ptr.To(true)

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
	if cluster.Spec.ControlPlaneRef.IsDefined() {
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
