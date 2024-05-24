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

package machine

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
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
	"sigs.k8s.io/cluster-api/util/patch"
)

var externalReadyWait = 30 * time.Second

func (r *Reconciler) reconcilePhase(_ context.Context, m *clusterv1.Machine) {
	originalPhase := m.Status.Phase

	// Set the phase to "pending" if nil.
	if m.Status.Phase == "" {
		m.Status.SetTypedPhase(clusterv1.MachinePhasePending)
	}

	// Set the phase to "provisioning" if bootstrap is ready and the infrastructure isn't.
	if m.Status.BootstrapReady && !m.Status.InfrastructureReady {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseProvisioning)
	}

	// Set the phase to "provisioned" if there is a provider ID.
	if m.Spec.ProviderID != nil {
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

	// If the phase has changed, update the LastUpdated timestamp
	if m.Status.Phase != originalPhase {
		now := metav1.Now()
		m.Status.LastUpdated = &now
	}
}

// reconcileExternal handles generic unstructured objects referenced by a Machine.
func (r *Reconciler) reconcileExternal(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine, ref *corev1.ObjectReference) (external.ReconcileOutput, error) {
	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, ref); err != nil {
		return external.ReconcileOutput{}, err
	}

	result, err := r.ensureExternalOwnershipAndWatch(ctx, cluster, m, ref)
	if err != nil {
		return external.ReconcileOutput{}, err
	}
	if result.RequeueAfter > 0 || result.Paused {
		return result, nil
	}

	obj := result.Result

	// Set failure reason and message, if any.
	failureReason, failureMessage, err := external.FailuresFrom(obj)
	if err != nil {
		return external.ReconcileOutput{}, err
	}
	if failureReason != "" {
		machineStatusError := capierrors.MachineStatusError(failureReason)
		m.Status.FailureReason = &machineStatusError
	}
	if failureMessage != "" {
		m.Status.FailureMessage = ptr.To(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), failureMessage),
		)
	}

	return external.ReconcileOutput{Result: obj}, nil
}

// ensureExternalOwnershipAndWatch ensures that only the Machine owns the external object,
// adds a watch to the external object if one does not already exist and adds the necessary labels.
func (r *Reconciler) ensureExternalOwnershipAndWatch(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine, ref *corev1.ObjectReference) (external.ReconcileOutput, error) {
	log := ctrl.LoggerFrom(ctx)

	obj, err := external.Get(ctx, r.UnstructuredCachingClient, ref, m.Namespace)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			log.Info("could not find external ref, requeuing", ref.Kind, klog.KRef(ref.Namespace, ref.Name))
			return external.ReconcileOutput{RequeueAfter: externalReadyWait}, nil
		}
		return external.ReconcileOutput{}, err
	}

	// Ensure we add a watch to the external object, if there isn't one already.
	if err := r.externalTracker.Watch(log, obj, handler.EnqueueRequestForOwner(r.Client.Scheme(), r.Client.RESTMapper(), &clusterv1.Machine{})); err != nil {
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

	// removeOnCreateOwnerRefs removes MachineSet and control plane owners from the objects referred to by a Machine.
	// These owner references are added initially because Machines don't exist when those objects are created.
	// At this point the Machine exists and can be set as the controller reference.
	if err := removeOnCreateOwnerRefs(cluster, m, obj); err != nil {
		return external.ReconcileOutput{}, err
	}

	// Set external object ControllerReference to the Machine.
	if err := controllerutil.SetControllerReference(m, obj, r.Client.Scheme()); err != nil {
		return external.ReconcileOutput{}, err
	}

	// Set the Cluster label.
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName
	obj.SetLabels(labels)

	// Always attempt to Patch the external object.
	if err := patchHelper.Patch(ctx, obj); err != nil {
		return external.ReconcileOutput{}, err
	}

	return external.ReconcileOutput{Result: obj}, nil
}

// reconcileBootstrap reconciles the Spec.Bootstrap.ConfigRef object on a Machine.
func (r *Reconciler) reconcileBootstrap(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster
	m := s.machine

	// If the Bootstrap ref is nil (and so the machine should use user generated data secret), return.
	if m.Spec.Bootstrap.ConfigRef == nil {
		return ctrl.Result{}, nil
	}

	// Call generic external reconciler if we have an external reference.
	externalResult, err := r.reconcileExternal(ctx, cluster, m, m.Spec.Bootstrap.ConfigRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	s.bootstrapConfig = externalResult.Result

	// If the external object is paused return.
	if externalResult.Paused {
		return ctrl.Result{}, nil
	}

	if externalResult.RequeueAfter > 0 {
		return ctrl.Result{RequeueAfter: externalResult.RequeueAfter}, nil
	}

	// If the bootstrap data is populated, set ready and return.
	if m.Spec.Bootstrap.DataSecretName != nil {
		m.Status.BootstrapReady = true
		conditions.MarkTrue(m, clusterv1.BootstrapReadyCondition)
		return ctrl.Result{}, nil
	}
	bootstrapConfig := externalResult.Result

	// If the bootstrap config is being deleted, return early.
	if !bootstrapConfig.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// Determine if the bootstrap provider is ready.
	ready, err := external.IsReady(bootstrapConfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Report a summary of current status of the bootstrap object defined for this machine.
	conditions.SetMirror(m, clusterv1.BootstrapReadyCondition,
		conditions.UnstructuredGetter(bootstrapConfig),
		conditions.WithFallbackValue(ready, clusterv1.WaitingForDataSecretFallbackReason, clusterv1.ConditionSeverityInfo, ""),
	)

	// If the bootstrap provider is not ready, requeue.
	if !ready {
		log.Info("Waiting for bootstrap provider to generate data secret and report status.ready", bootstrapConfig.GetKind(), klog.KObj(bootstrapConfig))
		return ctrl.Result{}, nil
	}

	// Get and set the name of the secret containing the bootstrap data.
	secretName, _, err := unstructured.NestedString(bootstrapConfig.Object, "status", "dataSecretName")
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve dataSecretName from bootstrap provider for Machine %q in namespace %q", m.Name, m.Namespace)
	} else if secretName == "" {
		return ctrl.Result{}, errors.Errorf("retrieved empty dataSecretName from bootstrap provider for Machine %q in namespace %q", m.Name, m.Namespace)
	}
	m.Spec.Bootstrap.DataSecretName = ptr.To(secretName)
	if !m.Status.BootstrapReady {
		log.Info("Bootstrap provider generated data secret and reports status.ready", bootstrapConfig.GetKind(), klog.KObj(bootstrapConfig), "Secret", klog.KRef(m.Namespace, secretName))
	}
	m.Status.BootstrapReady = true
	return ctrl.Result{}, nil
}

// reconcileInfrastructure reconciles the Spec.InfrastructureRef object on a Machine.
func (r *Reconciler) reconcileInfrastructure(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster
	m := s.machine

	// Call generic external reconciler.
	infraReconcileResult, err := r.reconcileExternal(ctx, cluster, m, &m.Spec.InfrastructureRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	s.infraMachine = infraReconcileResult.Result
	if infraReconcileResult.RequeueAfter > 0 {
		// Infra object went missing after the machine was up and running
		if m.Status.InfrastructureReady {
			log.Error(err, "Machine infrastructure reference has been deleted after being ready, setting failure state")
			m.Status.FailureReason = ptr.To(capierrors.InvalidConfigurationMachineError)
			m.Status.FailureMessage = ptr.To(fmt.Sprintf("Machine infrastructure resource %v with name %q has been deleted after being ready",
				m.Spec.InfrastructureRef.GroupVersionKind(), m.Spec.InfrastructureRef.Name))
			return ctrl.Result{}, errors.Errorf("could not find %v %q for Machine %q in namespace %q, requeuing", m.Spec.InfrastructureRef.GroupVersionKind().String(), m.Spec.InfrastructureRef.Name, m.Name, m.Namespace)
		}
		return ctrl.Result{RequeueAfter: infraReconcileResult.RequeueAfter}, nil
	}
	// if the external object is paused, return without any further processing
	if infraReconcileResult.Paused {
		return ctrl.Result{}, nil
	}
	infraConfig := infraReconcileResult.Result

	if !infraConfig.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// Determine if the infrastructure provider is ready.
	ready, err := external.IsReady(infraConfig)
	if err != nil {
		return ctrl.Result{}, err
	}
	if ready && !m.Status.InfrastructureReady {
		log.Info("Infrastructure provider has completed machine infrastructure provisioning and reports status.ready", infraConfig.GetKind(), klog.KObj(infraConfig))
	}
	m.Status.InfrastructureReady = ready

	// Report a summary of current status of the infrastructure object defined for this machine.
	conditions.SetMirror(m, clusterv1.InfrastructureReadyCondition,
		conditions.UnstructuredGetter(infraConfig),
		conditions.WithFallbackValue(ready, clusterv1.WaitingForInfrastructureFallbackReason, clusterv1.ConditionSeverityInfo, ""),
	)

	// If the infrastructure provider is not ready, return early.
	if !ready {
		log.Info("Waiting for infrastructure provider to create machine infrastructure and report status.ready", infraConfig.GetKind(), klog.KObj(infraConfig))
		return ctrl.Result{}, nil
	}

	// Get Spec.ProviderID from the infrastructure provider.
	var providerID string
	if err := util.UnstructuredUnmarshalField(infraConfig, &providerID, "spec", "providerID"); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve Spec.ProviderID from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	} else if providerID == "" {
		return ctrl.Result{}, errors.Errorf("retrieved empty Spec.ProviderID from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	}

	// Get and set Status.Addresses from the infrastructure provider.
	err = util.UnstructuredUnmarshalField(infraConfig, &m.Status.Addresses, "status", "addresses")
	if err != nil && err != util.ErrUnstructuredFieldNotFound {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve addresses from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	}

	// Get and set the failure domain from the infrastructure provider.
	var failureDomain string
	err = util.UnstructuredUnmarshalField(infraConfig, &failureDomain, "spec", "failureDomain")
	switch {
	case err == util.ErrUnstructuredFieldNotFound: // no-op
	case err != nil:
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve failure domain from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	default:
		m.Spec.FailureDomain = ptr.To(failureDomain)
	}

	m.Spec.ProviderID = ptr.To(providerID)
	return ctrl.Result{}, nil
}

func (r *Reconciler) reconcileCertificateExpiry(_ context.Context, s *scope) (ctrl.Result, error) {
	m := s.machine
	var annotations map[string]string

	if !util.IsControlPlaneMachine(m) {
		// If the machine is not a control plane machine, return early.
		return ctrl.Result{}, nil
	}

	var expiryInfoFound bool

	// Check for certificate expiry information in the machine annotation.
	// This should take precedence over other information.
	annotations = m.GetAnnotations()
	if expiry, ok := annotations[clusterv1.MachineCertificatesExpiryDateAnnotation]; ok {
		expiryInfoFound = true
		expiryTime, err := time.Parse(time.RFC3339, expiry)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile certificates expiry: failed to parse expiry date from annotation on %s", klog.KObj(m))
		}
		expTime := metav1.NewTime(expiryTime)
		m.Status.CertificatesExpiryDate = &expTime
	} else if s.bootstrapConfig != nil {
		// If the expiry information is not available on the machine annotation
		// look for it on the bootstrap config.
		annotations = s.bootstrapConfig.GetAnnotations()
		if expiry, ok := annotations[clusterv1.MachineCertificatesExpiryDateAnnotation]; ok {
			expiryInfoFound = true
			expiryTime, err := time.Parse(time.RFC3339, expiry)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile certificates expiry: failed to parse expiry date from annotation on %s", klog.KObj(s.bootstrapConfig))
			}
			expTime := metav1.NewTime(expiryTime)
			m.Status.CertificatesExpiryDate = &expTime
		}
	}

	// If the certificates expiry information is not fond on the machine
	// and on the bootstrap config then reset machine.status.certificatesExpiryDate.
	if !expiryInfoFound {
		m.Status.CertificatesExpiryDate = nil
	}

	return ctrl.Result{}, nil
}

// removeOnCreateOwnerRefs will remove any MachineSet or control plane owner references from passed objects.
func removeOnCreateOwnerRefs(cluster *clusterv1.Cluster, m *clusterv1.Machine, obj *unstructured.Unstructured) error {
	cpGVK := getControlPlaneGVKForMachine(cluster, m)
	for _, owner := range obj.GetOwnerReferences() {
		ownerGV, err := schema.ParseGroupVersion(owner.APIVersion)
		if err != nil {
			return errors.Wrapf(err, "Could not remove ownerReference %v from object %s/%s", owner.String(), obj.GetKind(), obj.GetName())
		}
		if (ownerGV.Group == clusterv1.GroupVersion.Group && owner.Kind == "MachineSet") ||
			(cpGVK != nil && ownerGV.Group == cpGVK.GroupVersion().Group && owner.Kind == cpGVK.Kind) {
			ownerRefs := util.RemoveOwnerRef(obj.GetOwnerReferences(), owner)
			obj.SetOwnerReferences(ownerRefs)
		}
	}
	return nil
}

// getControlPlaneGVKForMachine returns the Kind of the control plane in the Cluster associated with the Machine.
// This function checks that the Machine is managed by a control plane, and then retrieves the Kind from the Cluster's
// .spec.controlPlaneRef.
func getControlPlaneGVKForMachine(cluster *clusterv1.Cluster, machine *clusterv1.Machine) *schema.GroupVersionKind {
	if _, ok := machine.GetLabels()[clusterv1.MachineControlPlaneLabel]; ok {
		if cluster.Spec.ControlPlaneRef != nil {
			gvk := cluster.Spec.ControlPlaneRef.GroupVersionKind()
			return &gvk
		}
	}
	return nil
}
