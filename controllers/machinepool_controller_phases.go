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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func (r *MachinePoolReconciler) reconcilePhase(mp *clusterv1.MachinePool) {
	// Set the phase to "pending" if nil.
	if mp.Status.Phase == "" {
		mp.Status.SetTypedPhase(clusterv1.MachinePoolPhasePending)
	}

	// Set the phase to "provisioning" if bootstrap is ready and the infrastructure isn't.
	if mp.Status.BootstrapReady && !mp.Status.InfrastructureReady {
		mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseProvisioning)
	}

	// Set the phase to "provisioned" if the infrastructure is ready.
	if mp.Status.InfrastructureReady {
		mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseProvisioned)
	}

	// Set the phase to "running" if there is a NodeRef field.
	if mp.Status.InfrastructureReady && mp.Status.NodeRefs != nil && len(mp.Status.NodeRefs) == int(mp.Status.ReadyReplicas) {
		mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseRunning)
	}

	// Set the phase to "failed" if any of Status.ErrorReason or Status.ErrorMessage is not-nil.
	if mp.Status.ErrorReason != nil || mp.Status.ErrorMessage != nil {
		mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseFailed)
	}

	// Set the phase to "deleting" if the deletion timestamp is set.
	if !mp.DeletionTimestamp.IsZero() {
		mp.Status.SetTypedPhase(clusterv1.MachinePoolPhaseDeleting)
	}
}

// reconcileExternal handles generic unstructured objects referenced by a MachinePool
func (r *MachinePoolReconciler) reconcileExternal(ctx context.Context, mp *clusterv1.MachinePool, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	obj, err := external.Get(ctx, r.Client, ref, mp.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: 30 * time.Second},
				"could not find %v %q for MachinePool %q in namespace %q, requeuing",
				ref.GroupVersionKind(), ref.Name, mp.Name, mp.Namespace)
		}
		return nil, err
	}

	objPatch := client.MergeFrom(obj.DeepCopy())

	// Set external object OwnerReference to the MachinePool.
	ownerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "MachinePool",
		Name:       mp.Name,
		UID:        mp.UID,
	}

	if !util.HasOwnerRef(obj.GetOwnerReferences(), ownerRef) {
		obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), ownerRef))
		if err := r.Client.Patch(ctx, obj, objPatch); err != nil {
			return nil, errors.Wrapf(err,
				"failed to set OwnerReference on %v %q for MachinePool %q in namespace %q",
				obj.GroupVersionKind(), ref.Name, mp.Name, mp.Namespace)
		}
	}

	// Add watcher for external object, if there isn't one already.
	_, loaded := r.externalWatchers.LoadOrStore(obj.GroupVersionKind().String(), struct{}{})
	if !loaded && r.controller != nil {
		klog.Infof("Adding watcher on external object %q", obj.GroupVersionKind())
		err := r.controller.Watch(
			&source.Kind{Type: obj},
			&handler.EnqueueRequestForOwner{OwnerType: &clusterv1.MachinePool{}},
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
		machinePoolStatusError := capierrors.MachinePoolStatusError(errorReason)
		mp.Status.ErrorReason = &machinePoolStatusError
	}
	if errorMessage != "" {
		mp.Status.ErrorMessage = pointer.StringPtr(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), errorMessage),
		)
	}

	return obj, nil
}

// reconcileBootstrap reconciles the Spec.Bootstrap.ConfigRef object on a MachinePool.
func (r *MachinePoolReconciler) reconcileBootstrap(ctx context.Context, mp *clusterv1.MachinePool) error {
	// TODO(vincepri): Move this validation in kubebuilder / webhook.
	if mp.Spec.Template.Spec.Bootstrap.ConfigRef == nil && mp.Spec.Template.Spec.Bootstrap.Data == nil {
		return errors.Errorf(
			"Expected at least one of `Bootstrap.ConfigRef` or `Bootstrap.Data` to be populated for MachinePool %q in namespace %q",
			mp.Name, mp.Namespace,
		)
	}

	// Call generic external reconciler if we have an external reference.
	var bootstrapConfig *unstructured.Unstructured
	if mp.Spec.Template.Spec.Bootstrap.ConfigRef != nil {
		var err error
		bootstrapConfig, err = r.reconcileExternal(ctx, mp, mp.Spec.Template.Spec.Bootstrap.ConfigRef)
		if err != nil {
			return err
		}
	}

	// If the bootstrap config is being deleted, return early.
	if !bootstrapConfig.GetDeletionTimestamp().IsZero() {
		return nil
	}

	// Determine if the bootstrap provider is ready.
	ready, err := external.IsReady(bootstrapConfig)
	if err != nil {
		return err
	}

	mp.Status.BootstrapReady = ready
	if !mp.Status.BootstrapReady {
		return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: externalReadyWait},
			"Bootstrap provider for MachinePool %q in namespace %q is not ready, requeuing", mp.Name, mp.Namespace)
	}

	// Get and set data from the bootstrap provider.
	data, _, err := unstructured.NestedString(bootstrapConfig.Object, "status", "bootstrapData")
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve data from bootstrap provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	} else if data == "" {
		return errors.Errorf("retrieved empty data from bootstrap provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	}

	mp.Spec.Template.Spec.Bootstrap.Data = pointer.StringPtr(data)
	return nil
}

// reconcileInfrastructure reconciles the Spec.InfrastructureRef object on a MachinePool.
func (r *MachinePoolReconciler) reconcileInfrastructure(ctx context.Context, mp *clusterv1.MachinePool) error {
	// Call generic external reconciler.
	infraConfig, err := r.reconcileExternal(ctx, mp, &mp.Spec.Template.Spec.InfrastructureRef)
	if infraConfig == nil && err == nil {
		return nil
	} else if err != nil {
		return err
	}

	if !infraConfig.GetDeletionTimestamp().IsZero() {
		return nil
	}

	ready, err := external.IsReady(infraConfig)
	if err != nil {
		return err
	}

	mp.Status.InfrastructureReady = ready
	if !mp.Status.InfrastructureReady {
		return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: externalReadyWait},
			"Infrastructure provider for MachinePool %q in namespace %q is not ready, requeuing", mp.Name, mp.Namespace,
		)
	}

	// Get Spec.ProviderIDs from the infrastructure provider.
	if err := util.UnstructuredUnmarshalField(infraConfig, &mp.Spec.ProviderIDs, "spec", "providerIDs"); err != nil {
		return errors.Wrapf(err, "failed to retrieve data from infrastructure provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
	} else if mp.Spec.ProviderIDs == nil {
		return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: externalReadyWait},
			"retrieved empty Spec.ProviderIDs from infrastructure provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace,
		)
	}

	// Get and set Status.Replicas from the infrastructure provider.
	err = util.UnstructuredUnmarshalField(infraConfig, &mp.Status.Replicas, "status", "replicas")
	if err != nil {
		if err != util.ErrUnstructuredFieldNotFound {
			return errors.Wrapf(err, "failed to retrieve replicas from infrastructure provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace)
		}
	} else if mp.Status.Replicas == 0 {
		return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: externalReadyWait},
			"retrieved unset Status.Replicas from infrastructure provider for MachinePool %q in namespace %q", mp.Name, mp.Namespace,
		)
	}
	return nil
}
