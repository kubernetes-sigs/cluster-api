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
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
)

func (r *Reconciler) reconcilePhase(_ context.Context, cluster *clusterv1.Cluster) {
	preReconcilePhase := cluster.Status.GetTypedPhase()

	if cluster.Status.Phase == "" {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhasePending)
	}

	if cluster.Spec.InfrastructureRef != nil {
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
			r.recorder.Eventf(cluster, corev1.EventTypeWarning, string(cluster.Status.GetTypedPhase()), "Cluster %s is %s: %s", cluster.Name, string(cluster.Status.GetTypedPhase()), pointer.StringDeref(cluster.Status.FailureMessage, "unknown"))
		} else {
			r.recorder.Eventf(cluster, corev1.EventTypeNormal, string(cluster.Status.GetTypedPhase()), "Cluster %s is %s", cluster.Name, string(cluster.Status.GetTypedPhase()))
		}
	}
}

// reconcileExternal handles generic unstructured objects referenced by a Cluster.
func (r *Reconciler) reconcileExternal(ctx context.Context, cluster *clusterv1.Cluster, ref *corev1.ObjectReference) (external.ReconcileOutput, error) {
	log := ctrl.LoggerFrom(ctx)

	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, ref); err != nil {
		return external.ReconcileOutput{}, err
	}

	obj, err := external.Get(ctx, r.Client, ref, cluster.Namespace)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			log.Info("Could not find external object for cluster, requeuing", "refGroupVersionKind", ref.GroupVersionKind(), "refName", ref.Name)
			return external.ReconcileOutput{RequeueAfter: 30 * time.Second}, nil
		}
		return external.ReconcileOutput{}, err
	}

	// if external ref is paused, return error.
	if annotations.IsPaused(cluster, obj) {
		log.V(3).Info("External object referenced is paused")
		return external.ReconcileOutput{Paused: true}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return external.ReconcileOutput{}, err
	}

	// Set external object ControllerReference to the Cluster.
	if err := controllerutil.SetControllerReference(cluster, obj, r.Client.Scheme()); err != nil {
		return external.ReconcileOutput{}, err
	}

	// Set the Cluster label.
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[clusterv1.ClusterNameLabel] = cluster.Name
	obj.SetLabels(labels)

	// Always attempt to Patch the external object.
	if err := patchHelper.Patch(ctx, obj); err != nil {
		return external.ReconcileOutput{}, err
	}

	// Ensure we add a watcher to the external object.
	if err := r.externalTracker.Watch(log, obj, handler.EnqueueRequestForOwner(r.Client.Scheme(), r.Client.RESTMapper(), &clusterv1.Cluster{})); err != nil {
		return external.ReconcileOutput{}, err
	}

	// Set failure reason and message, if any.
	failureReason, failureMessage, err := external.FailuresFrom(obj)
	if err != nil {
		return external.ReconcileOutput{}, err
	}
	if failureReason != "" {
		clusterStatusError := capierrors.ClusterStatusError(failureReason)
		cluster.Status.FailureReason = &clusterStatusError
	}
	if failureMessage != "" {
		cluster.Status.FailureMessage = pointer.String(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), failureMessage),
		)
	}

	return external.ReconcileOutput{Result: obj}, nil
}

// reconcileInfrastructure reconciles the Spec.InfrastructureRef object on a Cluster.
func (r *Reconciler) reconcileInfrastructure(ctx context.Context, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if cluster.Spec.InfrastructureRef == nil {
		return ctrl.Result{}, nil
	}

	// Call generic external reconciler.
	infraReconcileResult, err := r.reconcileExternal(ctx, cluster, cluster.Spec.InfrastructureRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Return early if we need to requeue.
	if infraReconcileResult.RequeueAfter > 0 {
		return ctrl.Result{RequeueAfter: infraReconcileResult.RequeueAfter}, nil
	}
	// If the external object is paused, return without any further processing.
	if infraReconcileResult.Paused {
		return ctrl.Result{}, nil
	}
	infraConfig := infraReconcileResult.Result

	// There's no need to go any further if the Cluster is marked for deletion.
	if !infraConfig.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// Determine if the infrastructure provider is ready.
	preReconcileInfrastructureReady := cluster.Status.InfrastructureReady
	ready, err := external.IsReady(infraConfig)
	if err != nil {
		return ctrl.Result{}, err
	}
	cluster.Status.InfrastructureReady = ready
	// Only record the event if the status has changed
	if preReconcileInfrastructureReady != cluster.Status.InfrastructureReady {
		r.recorder.Eventf(cluster, corev1.EventTypeNormal, "InfrastructureReady", "Cluster %s InfrastructureReady is now %t", cluster.Name, cluster.Status.InfrastructureReady)
	}

	// Report a summary of current status of the infrastructure object defined for this cluster.
	conditions.SetMirror(cluster, clusterv1.InfrastructureReadyCondition,
		conditions.UnstructuredGetter(infraConfig),
		conditions.WithFallbackValue(ready, clusterv1.WaitingForInfrastructureFallbackReason, clusterv1.ConditionSeverityInfo, ""),
	)

	if !ready {
		log.V(3).Info("Infrastructure provider is not ready yet")
		return ctrl.Result{}, nil
	}

	// Get and parse Spec.ControlPlaneEndpoint field from the infrastructure provider.
	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		if err := util.UnstructuredUnmarshalField(infraConfig, &cluster.Spec.ControlPlaneEndpoint, "spec", "controlPlaneEndpoint"); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve Spec.ControlPlaneEndpoint from infrastructure provider for Cluster %q in namespace %q",
				cluster.Name, cluster.Namespace)
		}
	}

	// Get and parse Status.FailureDomains from the infrastructure provider.
	failureDomains := clusterv1.FailureDomains{}
	if err := util.UnstructuredUnmarshalField(infraConfig, &failureDomains, "status", "failureDomains"); err != nil && err != util.ErrUnstructuredFieldNotFound {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve Status.FailureDomains from infrastructure provider for Cluster %q in namespace %q",
			cluster.Name, cluster.Namespace)
	}
	cluster.Status.FailureDomains = failureDomains

	return ctrl.Result{}, nil
}

// reconcileControlPlane reconciles the Spec.ControlPlaneRef object on a Cluster.
func (r *Reconciler) reconcileControlPlane(ctx context.Context, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	if cluster.Spec.ControlPlaneRef == nil {
		return ctrl.Result{}, nil
	}

	// Call generic external reconciler.
	controlPlaneReconcileResult, err := r.reconcileExternal(ctx, cluster, cluster.Spec.ControlPlaneRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Return early if we need to requeue.
	if controlPlaneReconcileResult.RequeueAfter > 0 {
		return ctrl.Result{RequeueAfter: controlPlaneReconcileResult.RequeueAfter}, nil
	}
	// If the external object is paused, return without any further processing.
	if controlPlaneReconcileResult.Paused {
		return ctrl.Result{}, nil
	}
	controlPlaneConfig := controlPlaneReconcileResult.Result

	// There's no need to go any further if the control plane resource is marked for deletion.
	if !controlPlaneConfig.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	preReconcileControlPlaneReady := cluster.Status.ControlPlaneReady
	// Determine if the control plane provider is ready.
	ready, err := external.IsReady(controlPlaneConfig)
	if err != nil {
		return ctrl.Result{}, err
	}
	cluster.Status.ControlPlaneReady = ready
	// Only record the event if the status has changed
	if preReconcileControlPlaneReady != cluster.Status.ControlPlaneReady {
		r.recorder.Eventf(cluster, corev1.EventTypeNormal, "ControlPlaneReady", "Cluster %s ControlPlaneReady is now %t", cluster.Name, cluster.Status.ControlPlaneReady)
	}

	// Report a summary of current status of the control plane object defined for this cluster.
	conditions.SetMirror(cluster, clusterv1.ControlPlaneReadyCondition,
		conditions.UnstructuredGetter(controlPlaneConfig),
		conditions.WithFallbackValue(ready, clusterv1.WaitingForControlPlaneFallbackReason, clusterv1.ConditionSeverityInfo, ""),
	)

	// Update cluster.Status.ControlPlaneInitialized if it hasn't already been set
	// Determine if the control plane provider is initialized.
	if !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		initialized, err := external.IsInitialized(controlPlaneConfig)
		if err != nil {
			return ctrl.Result{}, err
		}
		if initialized {
			conditions.MarkTrue(cluster, clusterv1.ControlPlaneInitializedCondition)
		} else {
			conditions.MarkFalse(cluster, clusterv1.ControlPlaneInitializedCondition, clusterv1.WaitingForControlPlaneProviderInitializedReason, clusterv1.ConditionSeverityInfo, "Waiting for control plane provider to indicate the control plane has been initialized")
		}
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileKubeconfig(ctx context.Context, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

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
