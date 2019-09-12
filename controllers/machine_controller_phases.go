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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	externalReadyWait = 30 * time.Second
)

func (r *MachineReconciler) reconcilePhase(ctx context.Context, m *clusterv1.Machine) {
	// Set the phase to "pending" if nil.
	if m.Status.Phase == "" {
		m.Status.SetTypedPhase(clusterv1.MachinePhasePending)
	}

	// Set the phase to "provisioning" if bootstrap is ready and the infrastructure isn't.
	if m.Status.BootstrapReady && !m.Status.InfrastructureReady {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseProvisioning)
	}

	// Set the phase to "provisioned" if the infrastructure is ready.
	if m.Status.InfrastructureReady {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseProvisioned)
	}

	// Set the phase to "running" if there is a NodeRef field.
	if m.Status.NodeRef != nil && m.Status.InfrastructureReady {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseRunning)
	}

	// Set the phase to "failed" if any of Status.ErrorReason or Status.ErrorMessage is not-nil.
	if m.Status.ErrorReason != nil || m.Status.ErrorMessage != nil {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseFailed)
	}

	// Set the phase to "deleting" if the deletion timestamp is set.
	if !m.DeletionTimestamp.IsZero() {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseDeleting)
	}
}

// reconcileExternal handles generic unstructured objects referenced by a Machine.
func (r *MachineReconciler) reconcileExternal(ctx context.Context, m *clusterv1.Machine, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	obj, err := external.Get(r.Client, ref, m.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: externalReadyWait},
				"could not find %v %q for Machine %q in namespace %q, requeuing",
				ref.GroupVersionKind(), ref.Name, m.Name, m.Namespace)
		}
		return nil, err
	}

	objPatch := client.MergeFrom(obj.DeepCopy())

	// Set external object OwnerReference to the Machine.
	machineOwnerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Machine",
		Name:       m.Name,
		UID:        m.UID,
	}

	if !util.HasOwnerRef(obj.GetOwnerReferences(), machineOwnerRef) {
		obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), machineOwnerRef))
		if err := r.Client.Patch(ctx, obj, objPatch); err != nil {
			return nil, errors.Wrapf(err,
				"failed to set OwnerReference on %v %q for Machine %q in namespace %q",
				obj.GroupVersionKind(), ref.Name, m.Name, m.Namespace)
		}
	}

	// Add watcher for external object, if there isn't one already.
	_, loaded := r.externalWatchers.LoadOrStore(obj.GroupVersionKind().String(), struct{}{})
	if !loaded && r.controller != nil {
		klog.Infof("Adding watcher on external object %q", obj.GroupVersionKind())
		err := r.controller.Watch(
			&source.Kind{Type: obj},
			&handler.EnqueueRequestForOwner{OwnerType: &clusterv1.Machine{}},
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
		machineStatusError := capierrors.MachineStatusError(errorReason)
		m.Status.ErrorReason = &machineStatusError
	}
	if errorMessage != "" {
		m.Status.ErrorMessage = pointer.StringPtr(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), errorMessage),
		)
	}

	return obj, nil
}

// reconcileBootstrap reconciles the Spec.Bootstrap.ConfigRef object on a Machine.
func (r *MachineReconciler) reconcileBootstrap(ctx context.Context, m *clusterv1.Machine) error {
	// TODO(vincepri): Move this validation in kubebuilder / webhook.
	if m.Spec.Bootstrap.ConfigRef == nil && m.Spec.Bootstrap.Data == nil {
		return errors.Errorf(
			"Expected at least one of `Bootstrap.ConfigRef` or `Bootstrap.Data` to be populated for Machine %q in namespace %q",
			m.Name, m.Namespace,
		)
	}

	// Call generic external reconciler if we have an external reference.
	var bootstrapConfig *unstructured.Unstructured
	if m.Spec.Bootstrap.ConfigRef != nil {
		var err error
		bootstrapConfig, err = r.reconcileExternal(ctx, m, m.Spec.Bootstrap.ConfigRef)
		if err != nil {
			return err
		}
	}

	// If the bootstrap data is populated, set ready and return.
	if m.Spec.Bootstrap.Data != nil {
		m.Status.BootstrapReady = true
		return nil
	}

	// If the bootstrap config is being deleted, return early.
	if !bootstrapConfig.GetDeletionTimestamp().IsZero() {
		return nil
	}

	// Determine if the bootstrap provider is ready.
	ready, err := external.IsReady(bootstrapConfig)
	if err != nil {
		return err
	} else if !ready {
		return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: externalReadyWait},
			"Bootstrap provider for Machine %q in namespace %q is not ready, requeuing", m.Name, m.Namespace)
	}

	// Get and set data from the bootstrap provider.
	data, _, err := unstructured.NestedString(bootstrapConfig.Object, "status", "bootstrapData")
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve data from bootstrap provider for Machine %q in namespace %q", m.Name, m.Namespace)
	} else if data == "" {
		return errors.Errorf("retrieved empty data from bootstrap provider for Machine %q in namespace %q", m.Name, m.Namespace)
	}

	m.Spec.Bootstrap.Data = pointer.StringPtr(data)
	m.Status.BootstrapReady = true
	return nil
}

// reconcileInfrastructure reconciles the Spec.InfrastructureRef object on a Machine.
func (r *MachineReconciler) reconcileInfrastructure(ctx context.Context, m *clusterv1.Machine) error {
	// Call generic external reconciler.
	infraConfig, err := r.reconcileExternal(ctx, m, &m.Spec.InfrastructureRef)
	if infraConfig == nil && err == nil {
		return nil
	} else if err != nil {
		return err
	}

	if m.Status.InfrastructureReady || !infraConfig.GetDeletionTimestamp().IsZero() {
		return nil
	}

	// Determine if the infrastructure provider is ready.
	ready, err := external.IsReady(infraConfig)
	if err != nil {
		return err
	} else if !ready {
		return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: externalReadyWait},
			"Infrastructure provider for Machine %q in namespace %q is not ready, requeuing", m.Name, m.Namespace,
		)
	}

	// Get Spec.ProviderID from the infrastructure provider.
	var providerID string
	if err := util.UnstructuredUnmarshalField(infraConfig, &providerID, "spec", "providerID"); err != nil {
		return errors.Wrapf(err, "failed to retrieve data from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	} else if providerID == "" {
		return errors.Errorf("retrieved empty Spec.ProviderID from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	}

	// Get and set Status.Addresses from the infrastructure provider.
	err = util.UnstructuredUnmarshalField(infraConfig, &m.Status.Addresses, "status", "addresses")

	if err != nil {
		if err != util.ErrUnstructuredFieldNotFound {
			return errors.Wrapf(err, "failed to retrieve addresses from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
		}
	}

	m.Spec.ProviderID = pointer.StringPtr(providerID)
	m.Status.InfrastructureReady = true
	return nil
}
