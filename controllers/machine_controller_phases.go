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
	"strings"
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
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	externalReadyWait = 30 * time.Second
)

func (r *MachineReconciler) reconcilePhase(_ context.Context, m *clusterv1.Machine) {
	// Set the phase to "pending" if nil.
	if m.Status.Phase == "" {
		m.Status.SetTypedPhase(clusterv1.MachinePhasePending)
	}

	// Set the phase to "provisioning" if bootstrap is ready and the infrastructure isn't.
	if m.Status.BootstrapReady && !m.Status.InfrastructureReady {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseProvisioning)
	}

	// Set the phase to "provisioned" if there is a NodeRef.
	if m.Status.NodeRef != nil {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseProvisioned)
	}

	// Set the phase to "running" if there is a NodeRef field and infrastructure is ready.
	if m.Status.NodeRef != nil && m.Status.InfrastructureReady {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseRunning)
	}

	// Set the phase to "failed" if any of Status.FailureReason or Status.FailureMessage is not-nil.
	if m.Status.FailureReason != nil || m.Status.FailureMessage != nil {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseFailed)
	}

	// Set the phase to "deleting" if the deletion timestamp is set.
	if !m.DeletionTimestamp.IsZero() {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseDeleting)
	}
}

// reconcileExternal handles generic unstructured objects referenced by a Machine.
func (r *MachineReconciler) reconcileExternal(ctx context.Context, m *clusterv1.Machine, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	logger := r.Log.WithValues("machine", m.Name, "namespace", m.Namespace)

	obj, err := external.Get(ctx, r.Client, ref, m.Namespace)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: externalReadyWait},
				"could not find %v %q for Machine %q in namespace %q, requeuing",
				ref.GroupVersionKind(), ref.Name, m.Name, m.Namespace)
		}
		return nil, err
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return nil, err
	}

	// Set external object OwnerReference to the Machine.
	ownerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Machine",
		Name:       m.Name,
		UID:        m.UID,
	}

	// Add ownerRef to object.
	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), ownerRef))

	// Set the Cluster label.
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[clusterv1.ClusterLabelName] = m.Spec.ClusterName
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
			&handler.EnqueueRequestForOwner{OwnerType: &clusterv1.Machine{}},
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
		machineStatusError := capierrors.MachineStatusError(failureReason)
		m.Status.FailureReason = &machineStatusError
	}
	if failureMessage != "" {
		m.Status.FailureMessage = pointer.StringPtr(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), failureMessage),
		)
	}

	return obj, nil
}

// reconcileBootstrap reconciles the Spec.Bootstrap.ConfigRef object on a Machine.
func (r *MachineReconciler) reconcileBootstrap(ctx context.Context, m *clusterv1.Machine) error {
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
	if m.Spec.Bootstrap.Data != nil || m.Spec.Bootstrap.DataSecretName != nil {
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

	// Get and set the name of the secret containing the bootstrap data.
	secretName, _, err := unstructured.NestedString(bootstrapConfig.Object, "status", "dataSecretName")
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve dataSecretName from bootstrap provider for Machine %q in namespace %q", m.Name, m.Namespace)
	} else if secretName == "" {
		return errors.Errorf("retrieved empty dataSecretName from bootstrap provider for Machine %q in namespace %q", m.Name, m.Namespace)
	}

	m.Spec.Bootstrap.DataSecretName = pointer.StringPtr(secretName)
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
		if m.Status.InfrastructureReady && strings.Contains(err.Error(), "could not find") {
			// Infra object went missing after the machine was up and running
			r.Log.Error(err, "Machine infrastructure reference has been deleted after being ready, setting failure state")
			m.Status.FailureReason = capierrors.MachineStatusErrorPtr(capierrors.InvalidConfigurationMachineError)
			m.Status.FailureMessage = pointer.StringPtr(fmt.Sprintf("Machine infrastructure resource %v with name %q has been deleted after being ready",
				m.Spec.InfrastructureRef.GroupVersionKind(), m.Spec.InfrastructureRef.Name))
		}
		return err
	}

	if !infraConfig.GetDeletionTimestamp().IsZero() {
		return nil
	}

	// Determine if the infrastructure provider is ready.
	ready, err := external.IsReady(infraConfig)
	if err != nil {
		return err
	}
	m.Status.InfrastructureReady = ready
	if !ready {
		return errors.Wrapf(&capierrors.RequeueAfterError{RequeueAfter: externalReadyWait},
			"Infrastructure provider for Machine %q in namespace %q is not ready, requeuing", m.Name, m.Namespace,
		)
	}

	// Get Spec.ProviderID from the infrastructure provider.
	var providerID string
	if err := util.UnstructuredUnmarshalField(infraConfig, &providerID, "spec", "providerID"); err != nil {
		return errors.Wrapf(err, "failed to retrieve Spec.ProviderID from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	} else if providerID == "" {
		return errors.Errorf("retrieved empty Spec.ProviderID from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	}

	// Get and set Status.Addresses from the infrastructure provider.
	err = util.UnstructuredUnmarshalField(infraConfig, &m.Status.Addresses, "status", "addresses")
	if err != nil && err != util.ErrUnstructuredFieldNotFound {
		return errors.Wrapf(err, "failed to retrieve addresses from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	}

	m.Spec.ProviderID = pointer.StringPtr(providerID)
	return nil
}
