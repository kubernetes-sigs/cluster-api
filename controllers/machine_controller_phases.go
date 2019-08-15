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

func (r *MachineReconciler) reconcile(ctx context.Context, cluster *v1alpha2.Cluster, m *v1alpha2.Machine) (err error) {
	// TODO(vincepri): These can be generalized with an interface and possibly a for loop.
	errors := []error{}
	errors = append(errors, r.reconcileBootstrap(ctx, m))
	errors = append(errors, r.reconcileInfrastructure(ctx, m))
	errors = append(errors, r.reconcileNodeRef(ctx, cluster, m))
	errors = append(errors, r.reconcilePhase(ctx, m))
	errors = append(errors, r.reconcileClusterAnnotations(ctx, cluster, m))

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

func (r *MachineReconciler) reconcilePhase(ctx context.Context, m *v1alpha2.Machine) error {
	// Set the phase to "pending" if nil.
	if m.Status.Phase == "" {
		m.Status.SetTypedPhase(v1alpha2.MachinePhasePending)
	}

	// Set the phase to "provisioning" if bootstrap is ready and the infrastructure isn't.
	if m.Status.BootstrapReady && !m.Status.InfrastructureReady {
		m.Status.SetTypedPhase(v1alpha2.MachinePhaseProvisioning)
	}

	// Set the phase to "provisioned" if the infrastructure is ready.
	if m.Status.InfrastructureReady {
		m.Status.SetTypedPhase(v1alpha2.MachinePhaseProvisioned)
	}

	// Set the phase to "running" if there is a NodeRef field.
	if m.Status.NodeRef != nil && m.Status.InfrastructureReady {
		m.Status.SetTypedPhase(v1alpha2.MachinePhaseRunning)
	}

	// Set the phase to "failed" if any of Status.ErrorReason or Status.ErrorMessage is not-nil.
	if m.Status.ErrorReason != nil || m.Status.ErrorMessage != nil {
		m.Status.SetTypedPhase(v1alpha2.MachinePhaseFailed)
	}

	// Set the phase to "deleting" if the deletion timestamp is set.
	if !m.DeletionTimestamp.IsZero() {
		m.Status.SetTypedPhase(v1alpha2.MachinePhaseDeleting)
	}

	return nil
}

// reconcileExternal handles generic unstructured objects referenced by a Machine.
func (r *MachineReconciler) reconcileExternal(ctx context.Context, m *v1alpha2.Machine, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	obj, err := external.Get(r.Client, ref, m.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) && !m.DeletionTimestamp.IsZero() {
			return nil, nil
		} else if apierrors.IsNotFound(err) {
			return nil, errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: 30 * time.Second},
				"could not find %v %q for Machine %q in namespace %q, requeuing",
				ref.GroupVersionKind(), ref.Name, m.Name, m.Namespace)
		}
		return nil, err
	}

	objPatch := client.MergeFrom(obj.DeepCopy())

	// Delete the external object if the Machine is being deleted.
	if !m.DeletionTimestamp.IsZero() {
		if err := r.Delete(ctx, obj); err != nil {
			return nil, errors.Wrapf(err,
				"failed to delete %v %q for Machine %q in namespace %q",
				obj.GroupVersionKind(), ref.Name, m.Name, m.Namespace)
		}
		return obj, nil
	}

	// Set external object OwnerReference to the Machine.
	machineOwnerRef := metav1.OwnerReference{
		APIVersion: m.APIVersion,
		Kind:       m.Kind,
		Name:       m.Name,
		UID:        m.UID,
	}

	if !util.HasOwnerRef(obj.GetOwnerReferences(), machineOwnerRef) {
		obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), machineOwnerRef))
		if err := r.Patch(ctx, obj, objPatch); err != nil {
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
		m.Status.ErrorMessage = pointer.StringPtr(errorMessage)
	}

	return obj, nil
}

// reconcileBootstrap reconciles the Spec.Bootstrap.ConfigRef object on a Machine.
func (r *MachineReconciler) reconcileBootstrap(ctx context.Context, m *v1alpha2.Machine) error {
	// TODO(vincepri): Move this validation in kubebuilder / webhook.
	if m.Spec.Bootstrap.ConfigRef == nil && m.Spec.Bootstrap.Data == nil {
		return errors.Errorf(
			"Expected at least one of `Bootstrap.ConfigRef` or `Bootstrap.Data` to be populated for Machine %q in namespace %q",
			m.Name, m.Namespace,
		)
	}

	if m.Spec.Bootstrap.Data != nil {
		m.Status.BootstrapReady = true
		return nil
	}

	// Call generic external reconciler.
	bootstrapConfig, err := r.reconcileExternal(ctx, m, m.Spec.Bootstrap.ConfigRef)
	if bootstrapConfig == nil && err == nil {
		m.Status.BootstrapReady = false
		return nil
	} else if err != nil {
		return err
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
		klog.V(3).Infof("Bootstrap provider for Machine %q in namespace %q is not ready, requeuing", m.Name, m.Namespace)
		return &capierrors.RequeueAfterError{RequeueAfter: 30 * time.Second}
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
func (r *MachineReconciler) reconcileInfrastructure(ctx context.Context, m *v1alpha2.Machine) error {
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
		klog.V(3).Infof("Infrastructure provider for Machine %q in namespace %q is not ready, requeuing", m.Name, m.Namespace)
		return &capierrors.RequeueAfterError{RequeueAfter: 30 * time.Second}
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

// reconcileClusterAnnotations reconciles the annotations on the Cluster associated with Machines.
// TODO(vincepri): Move to cluster controller once the proposal merges.
func (r *MachineReconciler) reconcileClusterAnnotations(ctx context.Context, cluster *v1alpha2.Cluster, m *v1alpha2.Machine) error {
	if !m.DeletionTimestamp.IsZero() || cluster == nil {
		return nil
	}

	// If the Machine is a control plane, it has a NodeRef and it's ready, set an annotation on the Cluster.
	if util.IsControlPlaneMachine(m) && m.Status.NodeRef != nil && m.Status.InfrastructureReady {
		if cluster.Annotations == nil {
			cluster.SetAnnotations(map[string]string{})
		}

		if _, ok := cluster.Annotations[v1alpha2.ClusterAnnotationControlPlaneReady]; !ok {
			clusterPatch := client.MergeFrom(cluster.DeepCopy())
			cluster.Annotations[v1alpha2.ClusterAnnotationControlPlaneReady] = "true"
			if err := r.Client.Patch(ctx, cluster, clusterPatch); err != nil {
				return errors.Wrapf(err, "failed to set control-plane-ready annotation on Cluster %q in namespace %q",
					cluster.Name, cluster.Namespace)
			}
		}
	}

	return nil
}
