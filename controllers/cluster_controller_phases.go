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
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/api/v1alpha2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func (r *ClusterReconciler) reconcile(ctx context.Context, cluster *v1alpha2.Cluster) (err error) {
	// TODO(vincepri): These can be generalized with an interface and possibly a for loop.
	errors := []error{}
	errors = append(errors, r.reconcileInfrastructure(ctx, cluster))
	errors = append(errors, r.reconcilePhase(ctx, cluster))

	// Determine the return error, giving precedence to the first non-nil and non-requeueAfter errors.
	for _, e := range errors {
		if e == nil {
			continue
		}
		if err == nil || capierrors.IsRequeueAfter(err) {
			err = e
		}
	}
	return err
}

func (r *ClusterReconciler) reconcilePhase(ctx context.Context, cluster *v1alpha2.Cluster) error {
	// Set the phase to "pending" if nil.
	if cluster.Status.Phase == "" {
		cluster.Status.SetTypedPhase(v1alpha2.ClusterPhasePending)
	}

	// Set the phase to "provisioning" if the Cluster has an InfrastructureRef object associated.
	if cluster.Spec.InfrastructureRef != nil {
		cluster.Status.SetTypedPhase(v1alpha2.ClusterPhaseProvisioning)
	}

	// Set the phase to "provisioned" if the infrastructure is ready.
	if cluster.Status.InfrastructureReady {
		cluster.Status.SetTypedPhase(v1alpha2.ClusterPhaseProvisioned)
	}

	// Set the phase to "failed" if any of Status.ErrorReason or Status.ErrorMessage is not-nil.
	if cluster.Status.ErrorReason != nil || cluster.Status.ErrorMessage != nil {
		cluster.Status.SetTypedPhase(v1alpha2.ClusterPhaseFailed)
	}

	// Set the phase to "deleting" if the deletion timestamp is set.
	if !cluster.DeletionTimestamp.IsZero() {
		cluster.Status.SetTypedPhase(v1alpha2.ClusterPhaseDeleting)
	}

	return nil
}

// reconcileExternal handles generic unstructured objects referenced by a Cluster.
func (r *ClusterReconciler) reconcileExternal(ctx context.Context, cluster *v1alpha2.Cluster, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	obj, err := external.Get(r.Client, ref, cluster.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) && !cluster.DeletionTimestamp.IsZero() {
			return nil, nil
		} else if apierrors.IsNotFound(err) {
			return nil, errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: 30 * time.Second},
				"could not find %v %q for Cluster %q in namespace %q, requeuing",
				ref.GroupVersionKind(), ref.Name, cluster.Name, cluster.Namespace)
		}
		return nil, err
	}

	objPatch := client.MergeFrom(obj.DeepCopy())

	// Delete the external object if the Cluster is being deleted.
	if !cluster.DeletionTimestamp.IsZero() {
		if err := r.Delete(ctx, obj); err != nil {
			return nil, errors.Wrapf(err,
				"failed to delete %v %q for Cluster %q in namespace %q",
				obj.GroupVersionKind(), ref.Name, cluster.Name, cluster.Namespace)
		}
		return obj, nil
	}

	// Set external object OwnerReference to the Cluster.
	ownerRef := metav1.OwnerReference{
		APIVersion: cluster.APIVersion,
		Kind:       cluster.Kind,
		Name:       cluster.Name,
		UID:        cluster.UID,
	}

	if !util.HasOwnerRef(obj.GetOwnerReferences(), ownerRef) {
		obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), ownerRef))
		if err := r.Patch(ctx, obj, objPatch); err != nil {
			return nil, errors.Wrapf(err,
				"failed to set OwnerReference on %v %q for Cluster %q in namespace %q",
				obj.GroupVersionKind(), ref.Name, cluster.Name, cluster.Namespace)
		}
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
		cluster.Status.ErrorMessage = pointer.StringPtr(errorMessage)
	}

	return obj, nil
}

// reconcileInfrastructure reconciles the Spec.InfrastructureRef object on a Cluster.
func (r *ClusterReconciler) reconcileInfrastructure(ctx context.Context, cluster *v1alpha2.Cluster) error {
	if cluster.Spec.InfrastructureRef == nil {
		return nil
	}

	// Call generic external reconciler.
	infraConfig, err := r.reconcileExternal(ctx, cluster, cluster.Spec.InfrastructureRef)
	if infraConfig == nil && err == nil {
		return nil
	} else if err != nil {
		return err
	}

	if cluster.Status.InfrastructureReady || !infraConfig.GetDeletionTimestamp().IsZero() {
		return nil
	}

	// Determine if the infrastructure provider is ready.
	ready, err := external.IsReady(infraConfig)
	if err != nil {
		return err
	} else if !ready {
		klog.V(3).Infof("Infrastructure provider for Cluster %q in namespace %q is not ready, requeuing", cluster.Name, cluster.Namespace)
		return &capierrors.RequeueAfterError{RequeueAfter: 30 * time.Second}
	}

	// Get and parse Status.APIEndpoint field from the infrastructure provider.
	if err := util.UnstructuredUnmarshalField(infraConfig, &cluster.Status.APIEndpoints, "status", "apiEndpoints"); err != nil {
		return errors.Wrapf(err, "failed to retrieve Status.APIEndpoints from infrastructure provider for Cluster %q in namespace %q",
			cluster.Name, cluster.Namespace)
	} else if len(cluster.Status.APIEndpoints) == 0 {
		return errors.Wrapf(err, "retrieved empty Status.APIEndpoints from infrastructure provider for Cluster %q in namespace %q",
			cluster.Name, cluster.Namespace)
	}

	cluster.Status.InfrastructureReady = true
	return nil
}
