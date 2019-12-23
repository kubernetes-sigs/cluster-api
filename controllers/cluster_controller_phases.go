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

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func (r *ClusterReconciler) reconcilePhase(_ context.Context, cluster *clusterv1.Cluster) {
	if cluster.Status.Phase == "" {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhasePending)
	}

	if cluster.Spec.InfrastructureRef != nil {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseProvisioning)
	}

	if cluster.Status.InfrastructureReady && !cluster.Spec.ControlPlaneEndpoint.IsZero() {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseProvisioned)
	}

	if cluster.Status.FailureReason != nil || cluster.Status.FailureMessage != nil {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseFailed)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseDeleting)
	}
}

// reconcileExternal handles generic unstructured objects referenced by a Cluster.
func (r *ClusterReconciler) reconcileExternal(ctx context.Context, cluster *clusterv1.Cluster, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	logger := r.Log.WithValues("cluster", cluster.Name, "namespace", cluster.Namespace)

	obj, err := external.Get(ctx, r.Client, ref, cluster.Namespace)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: 30 * time.Second},
				"could not find %v %q for Cluster %q in namespace %q, requeuing",
				ref.GroupVersionKind(), ref.Name, cluster.Name, cluster.Namespace)
		}
		return nil, err
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return nil, err
	}

	// Set external object OwnerReference to the Cluster.
	ownerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}

	// Add ownerRef to object.
	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), ownerRef))

	// Set the Cluster label.
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[clusterv1.ClusterLabelName] = cluster.Name
	obj.SetLabels(labels)

	// Always attempt to Patch the external object.
	if err := patchHelper.Patch(ctx, obj); err != nil {
		return nil, err
	}

	// Add watcher for external object, if there isn't one already.
	_, loaded := r.externalWatchers.LoadOrStore(obj.GroupVersionKind().String(), struct{}{})
	if !loaded && r.controller != nil {
		logger.Info("Adding watcher on external object", "gvk", obj.GroupVersionKind())
		err := r.controller.Watch(
			&source.Kind{Type: obj},
			&handler.EnqueueRequestForOwner{OwnerType: &clusterv1.Cluster{}},
		)
		if err != nil {
			r.externalWatchers.Delete(obj.GroupVersionKind().String())
			return nil, errors.Wrapf(err, "failed to add watcher on external object %q", obj.GroupVersionKind())
		}
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
		cluster.Status.FailureMessage = pointer.StringPtr(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), failureMessage),
		)
	}

	return obj, nil
}

// reconcileInfrastructure reconciles the Spec.InfrastructureRef object on a Cluster.
func (r *ClusterReconciler) reconcileInfrastructure(ctx context.Context, cluster *clusterv1.Cluster) error {
	logger := r.Log.WithValues("cluster", cluster.Name, "namespace", cluster.Namespace)

	if cluster.Spec.InfrastructureRef == nil {
		return nil
	}

	// Call generic external reconciler.
	infraConfig, err := r.reconcileExternal(ctx, cluster, cluster.Spec.InfrastructureRef)
	if err != nil {
		return err
	}

	// There's no need to go any further if the Cluster is marked for deletion.
	if !infraConfig.GetDeletionTimestamp().IsZero() {
		return nil
	}

	// Determine if the infrastructure provider is ready.
	ready, err := external.IsReady(infraConfig)
	if err != nil {
		return err
	}
	cluster.Status.InfrastructureReady = ready
	if !ready {
		logger.V(3).Info("Infrastructure provider is not ready yet")
		return nil
	}

	// Get and parse Status.APIEndpoint field from the infrastructure provider.
	if cluster.Spec.ControlPlaneEndpoint.IsZero() {
		if err := util.UnstructuredUnmarshalField(infraConfig, &cluster.Spec.ControlPlaneEndpoint, "spec", "controlPlaneEndpoint"); err != nil {
			return errors.Wrapf(err, "failed to retrieve Spec.ControlPlaneEndpoint from infrastructure provider for Cluster %q in namespace %q",
				cluster.Name, cluster.Namespace)
		}
	}

	// Get and parse Status.FailureDomains from the infrastructure provider.
	if err := util.UnstructuredUnmarshalField(infraConfig, &cluster.Status.FailureDomains, "status", "failureDomains"); err != nil && err != util.ErrUnstructuredFieldNotFound {
		return errors.Wrapf(err, "failed to retrieve Status.FailureDomains from infrastructure provider for Cluster %q in namespace %q",
			cluster.Name, cluster.Namespace)
	}

	return nil
}

// reconcileControlPlane reconciles the Spec.ControlPlaneRef object on a Cluster.
func (r *ClusterReconciler) reconcileControlPlane(ctx context.Context, cluster *clusterv1.Cluster) error {
	if cluster.Spec.ControlPlaneRef == nil {
		return nil
	}

	// Call generic external reconciler.
	controlPlaneConfig, err := r.reconcileExternal(ctx, cluster, cluster.Spec.ControlPlaneRef)
	if err != nil {
		return err
	}

	// There's no need to go any further if the control plane resource is marked for deletion.
	if !controlPlaneConfig.GetDeletionTimestamp().IsZero() {
		return nil
	}

	// Update cluster.Status.ControlPlaneInitialized if it hasn't already been set
	// Determine if the control plane provider is initialized.
	if !cluster.Status.ControlPlaneInitialized {
		initialized, err := external.IsInitialized(controlPlaneConfig)
		if err != nil {
			return err
		}
		cluster.Status.ControlPlaneInitialized = initialized
	}

	// Determine if the control plane provider is ready.
	ready, err := external.IsReady(controlPlaneConfig)
	if err != nil {
		return err
	}
	cluster.Status.ControlPlaneReady = ready

	return nil
}

func (r *ClusterReconciler) reconcileKubeconfig(ctx context.Context, cluster *clusterv1.Cluster) error {
	if cluster.Spec.ControlPlaneEndpoint.IsZero() {
		return nil
	}

	// Do not generate the Kubeconfig if there is a ControlPlaneRef, since the Control Plane provider is
	// responsible for the management of the Kubeconfig. We continue to manage it here only for backward
	// compatibility when a Control Plane provider is not in use.
	if cluster.Spec.ControlPlaneRef != nil {
		return nil
	}

	_, err := secret.Get(r.Client, cluster, secret.Kubeconfig)
	switch {
	case apierrors.IsNotFound(err):
		if err := kubeconfig.CreateSecret(ctx, r.Client, cluster); err != nil {
			if err == kubeconfig.ErrDependentCertificateNotFound {
				return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: 30 * time.Second},
					"could not find secret %q for Cluster %q in namespace %q, requeuing",
					secret.ClusterCA, cluster.Name, cluster.Namespace)
			}
			return err
		}
	case err != nil:
		return errors.Wrapf(err, "failed to retrieve Kubeconfig Secret for Cluster %q in namespace %q", cluster.Name, cluster.Namespace)
	}

	return nil
}
