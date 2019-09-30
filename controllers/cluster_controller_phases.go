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
	"k8s.io/klog"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func (r *ClusterReconciler) reconcilePhase(ctx context.Context, cluster *clusterv1.Cluster) {
	// Set the phase to "pending" if nil.
	if cluster.Status.Phase == "" {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhasePending)
	}

	// Set the phase to "provisioning" if the Cluster has an InfrastructureRef object associated.
	if cluster.Spec.InfrastructureRef != nil {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseProvisioning)
	}

	// Set the phase to "provisioned" if the infrastructure is ready.
	if cluster.Status.InfrastructureReady {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseProvisioned)
	}

	// Set the phase to "failed" if any of Status.ErrorReason or Status.ErrorMessage is not-nil.
	if cluster.Status.ErrorReason != nil || cluster.Status.ErrorMessage != nil {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseFailed)
	}

	// Set the phase to "deleting" if the deletion timestamp is set.
	if !cluster.DeletionTimestamp.IsZero() {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseDeleting)
	}
}

// reconcileExternal handles generic unstructured objects referenced by a Cluster.
func (r *ClusterReconciler) reconcileExternal(ctx context.Context, cluster *clusterv1.Cluster, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	obj, err := external.Get(r.Client, ref, cluster.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
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

	// Always attempt to Patch the external object.
	if err := patchHelper.Patch(ctx, obj); err != nil {
		return nil, err
	}

	// Add watcher for external object, if there isn't one already.
	_, loaded := r.externalWatchers.LoadOrStore(obj.GroupVersionKind().String(), struct{}{})
	if !loaded && r.controller != nil {
		klog.Infof("Adding watcher on external object %q", obj.GroupVersionKind())
		err := r.controller.Watch(
			&source.Kind{Type: obj},
			&handler.EnqueueRequestForOwner{OwnerType: &clusterv1.Cluster{}},
		)
		if err != nil {
			r.externalWatchers.Delete(obj.GroupVersionKind().String())
			return nil, errors.Wrapf(err, "failed to add watcher on external object %q", obj.GroupVersionKind())
		}
	}

	// Set error reason and message, if any.
	errorReason, errorMessage, err := external.ErrorsFrom(obj)
	if err != nil {
		return nil, err
	}
	if errorReason != "" {
		clusterStatusError := capierrors.ClusterStatusError(errorReason)
		cluster.Status.ErrorReason = &clusterStatusError
	}
	if errorMessage != "" {
		cluster.Status.ErrorMessage = pointer.StringPtr(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), errorMessage),
		)
	}

	return obj, nil
}

// reconcileInfrastructure reconciles the Spec.InfrastructureRef object on a Cluster.
func (r *ClusterReconciler) reconcileInfrastructure(ctx context.Context, cluster *clusterv1.Cluster) error {
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
	if !cluster.Status.InfrastructureReady {
		ready, err := external.IsReady(infraConfig)
		if err != nil {
			return err
		} else if !ready {
			klog.V(3).Infof("Infrastructure provider for Cluster %q in namespace %q is not ready yet", cluster.Name, cluster.Namespace)
			return nil
		}
		cluster.Status.InfrastructureReady = true
	}

	// Get and parse Status.APIEndpoint field from the infrastructure provider.
	if len(cluster.Status.APIEndpoints) == 0 {
		if err := util.UnstructuredUnmarshalField(infraConfig, &cluster.Status.APIEndpoints, "status", "apiEndpoints"); err != nil {
			return errors.Wrapf(err, "failed to retrieve Status.APIEndpoints from infrastructure provider for Cluster %q in namespace %q",
				cluster.Name, cluster.Namespace)
		} else if len(cluster.Status.APIEndpoints) == 0 {
			return errors.Wrapf(err, "retrieved empty Status.APIEndpoints from infrastructure provider for Cluster %q in namespace %q",
				cluster.Name, cluster.Namespace)
		}
	}

	return nil
}

func (r *ClusterReconciler) reconcileKubeconfig(ctx context.Context, cluster *clusterv1.Cluster) error {
	if len(cluster.Status.APIEndpoints) == 0 {
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
