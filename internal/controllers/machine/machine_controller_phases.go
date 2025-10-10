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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

var externalReadyWait = 30 * time.Second

// reconcileExternal handles generic unstructured objects referenced by a Machine.
func (r *Reconciler) reconcileExternal(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine, ref clusterv1.ContractVersionedObjectReference) (*unstructured.Unstructured, error) {
	obj, err := r.ensureExternalOwnershipAndWatch(ctx, cluster, m, ref)
	if err != nil {
		return nil, err
	}

	// Set failure reason and message, if any.
	failureReason, failureMessage, err := external.FailuresFrom(obj)
	if err != nil {
		return nil, err
	}
	if failureReason != "" {
		machineStatusError := capierrors.MachineStatusError(failureReason)
		if m.Status.Deprecated == nil {
			m.Status.Deprecated = &clusterv1.MachineDeprecatedStatus{}
		}
		if m.Status.Deprecated.V1Beta1 == nil {
			m.Status.Deprecated.V1Beta1 = &clusterv1.MachineV1Beta1DeprecatedStatus{}
		}
		m.Status.Deprecated.V1Beta1.FailureReason = &machineStatusError
	}
	if failureMessage != "" {
		if m.Status.Deprecated == nil {
			m.Status.Deprecated = &clusterv1.MachineDeprecatedStatus{}
		}
		if m.Status.Deprecated.V1Beta1 == nil {
			m.Status.Deprecated.V1Beta1 = &clusterv1.MachineV1Beta1DeprecatedStatus{}
		}
		m.Status.Deprecated.V1Beta1.FailureMessage = ptr.To(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), failureMessage),
		)
	}

	return obj, nil
}

// ensureExternalOwnershipAndWatch ensures that only the Machine owns the external object,
// adds a watch to the external object if one does not already exist and adds the necessary labels.
func (r *Reconciler) ensureExternalOwnershipAndWatch(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine, ref clusterv1.ContractVersionedObjectReference) (*unstructured.Unstructured, error) {
	log := ctrl.LoggerFrom(ctx)

	obj, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, ref, m.Namespace)
	if err != nil {
		return nil, err
	}

	// Ensure we add a watch to the external object, if there isn't one already.
	if err := r.externalTracker.Watch(log, obj, handler.EnqueueRequestForOwner(r.Client.Scheme(), r.Client.RESTMapper(), &clusterv1.Machine{}), predicates.ResourceIsChanged(r.Client.Scheme(), *r.externalTracker.PredicateLogger)); err != nil {
		return nil, err
	}

	desiredOwnerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Machine",
		Name:       m.Name,
		UID:        m.UID,
		Controller: ptr.To(true),
	}

	hasOnCreateOwnerRefs, err := hasOnCreateOwnerRefs(cluster, m, obj)
	if err != nil {
		return nil, err
	}

	if !hasOnCreateOwnerRefs &&
		util.HasExactOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef) &&
		obj.GetLabels()[clusterv1.ClusterNameLabel] == m.Spec.ClusterName {
		return obj, nil
	}

	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return nil, err
	}

	// removeOnCreateOwnerRefs removes MachineSet and control plane owners from the objects referred to by a Machine.
	// These owner references are added initially because Machines don't exist when those objects are created.
	// At this point the Machine exists and can be set as the controller reference.
	if err := removeOnCreateOwnerRefs(cluster, m, obj); err != nil {
		return nil, err
	}

	// Set external object ControllerReference to the Machine.
	if err := controllerutil.SetControllerReference(m, obj, r.Client.Scheme()); err != nil {
		return nil, err
	}

	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName
	obj.SetLabels(labels)

	if err := patchHelper.Patch(ctx, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// reconcileBootstrap reconciles the BootstrapConfig of a Machine.
func (r *Reconciler) reconcileBootstrap(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster
	m := s.machine

	// If the Bootstrap ref is nil (and so the machine should use user generated data secret), return.
	if !m.Spec.Bootstrap.ConfigRef.IsDefined() {
		return ctrl.Result{}, nil
	}

	// Call generic external reconciler if we have an external reference.
	obj, err := r.reconcileExternal(ctx, cluster, m, m.Spec.Bootstrap.ConfigRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.bootstrapConfigIsNotFound = true

			if !s.machine.DeletionTimestamp.IsZero() {
				// Tolerate bootstrap object not found when the machine is being deleted.
				// TODO: we can also relax this and tolerate the absence of the bootstrap ref way before, e.g. after node ref is set
				return ctrl.Result{}, nil
			}
			log.Info("Could not find bootstrap config object, requeuing", m.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(m.Namespace, m.Spec.Bootstrap.ConfigRef.Name))
			// TODO: we can make this smarter and requeue only if we are before node ref is set
			return ctrl.Result{RequeueAfter: externalReadyWait}, nil
		}
		return ctrl.Result{}, err
	}
	s.bootstrapConfig = obj

	// If the bootstrap data is populated, set ready and return.
	if m.Spec.Bootstrap.DataSecretName != nil {
		m.Status.Initialization.BootstrapDataSecretCreated = ptr.To(true)
		v1beta1conditions.MarkTrue(m, clusterv1.BootstrapReadyV1Beta1Condition)
		return ctrl.Result{}, nil
	}

	// Determine contract version used by the BootstrapConfig.
	contractVersion, err := contract.GetContractVersion(ctx, r.Client, s.bootstrapConfig.GroupVersionKind().GroupKind())
	if err != nil {
		return ctrl.Result{}, err
	}

	// Determine if the data secret was created.
	var dataSecretCreated bool
	if dataSecretCreatedPtr, err := contract.Bootstrap().DataSecretCreated(contractVersion).Get(s.bootstrapConfig); err != nil {
		if !errors.Is(err, contract.ErrFieldNotFound) {
			return ctrl.Result{}, err
		}
	} else {
		dataSecretCreated = *dataSecretCreatedPtr
	}

	// Report a summary of current status of the bootstrap object defined for this machine.
	fallBack := v1beta1conditions.WithFallbackValue(dataSecretCreated, clusterv1.WaitingForDataSecretFallbackV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
	if !s.machine.DeletionTimestamp.IsZero() {
		fallBack = v1beta1conditions.WithFallbackValue(dataSecretCreated, clusterv1.DeletingV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
	}
	v1beta1conditions.SetMirror(m, clusterv1.BootstrapReadyV1Beta1Condition, v1beta1conditions.UnstructuredGetter(s.bootstrapConfig), fallBack)

	if !s.bootstrapConfig.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// If the data secret was not created yet, return.
	if !dataSecretCreated {
		log.Info(fmt.Sprintf("Waiting for bootstrap provider to generate data secret and set %s",
			contract.Bootstrap().DataSecretCreated(contractVersion).Path().String()),
			s.bootstrapConfig.GetKind(), klog.KObj(s.bootstrapConfig))
		return ctrl.Result{}, nil
	}

	// Get and set the dataSecretName containing the bootstrap data.
	secretName, err := contract.Bootstrap().DataSecretName().Get(s.bootstrapConfig)
	switch {
	case err != nil:
		return ctrl.Result{}, errors.Wrapf(err, "failed to read dataSecretName from %s %s",
			s.bootstrapConfig.GetKind(), klog.KObj(s.bootstrapConfig))
	case *secretName == "":
		return ctrl.Result{}, errors.Errorf("got empty %s field from %s %s",
			contract.Bootstrap().DataSecretName().Path().String(),
			s.bootstrapConfig.GetKind(), klog.KObj(s.bootstrapConfig))
	default:
		m.Spec.Bootstrap.DataSecretName = secretName
	}

	if !ptr.Deref(m.Status.Initialization.BootstrapDataSecretCreated, false) {
		log.Info("Bootstrap provider generated data secret", s.bootstrapConfig.GetKind(), klog.KObj(s.bootstrapConfig), "Secret", klog.KRef(m.Namespace, *secretName))
	}
	m.Status.Initialization.BootstrapDataSecretCreated = ptr.To(true)
	return ctrl.Result{}, nil
}

// reconcileInfrastructure reconciles the InfrastructureMachine of a Machine.
func (r *Reconciler) reconcileInfrastructure(ctx context.Context, s *scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	cluster := s.cluster
	m := s.machine

	// Call generic external reconciler.
	obj, err := r.reconcileExternal(ctx, cluster, m, m.Spec.InfrastructureRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.infraMachineIsNotFound = true

			if !s.machine.DeletionTimestamp.IsZero() {
				// Tolerate infra machine not found when the machine is being deleted.
				return ctrl.Result{}, nil
			}

			if ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false) {
				// Infra object went missing after the machine was up and running
				log.Error(err, "Machine infrastructure reference has been deleted after provisioning was completed, setting failure state")
				if m.Status.Deprecated == nil {
					m.Status.Deprecated = &clusterv1.MachineDeprecatedStatus{}
				}
				if m.Status.Deprecated.V1Beta1 == nil {
					m.Status.Deprecated.V1Beta1 = &clusterv1.MachineV1Beta1DeprecatedStatus{}
				}
				m.Status.Deprecated.V1Beta1.FailureReason = ptr.To(capierrors.InvalidConfigurationMachineError)
				m.Status.Deprecated.V1Beta1.FailureMessage = ptr.To(fmt.Sprintf("Machine infrastructure resource %s %s has been deleted after provisioning was completed",
					m.Spec.InfrastructureRef.Kind, klog.KRef(m.Namespace, m.Spec.InfrastructureRef.Name)))
				return ctrl.Result{}, errors.Errorf("could not find %s %s for Machine %s", m.Spec.InfrastructureRef.Kind, klog.KRef(m.Namespace, m.Spec.InfrastructureRef.Name), klog.KObj(m))
			}
			log.Info("Could not find InfrastructureMachine, requeuing", m.Spec.InfrastructureRef.Kind, klog.KRef(m.Namespace, m.Spec.InfrastructureRef.Name))
			return ctrl.Result{RequeueAfter: externalReadyWait}, nil
		}
		return ctrl.Result{}, err
	}
	s.infraMachine = obj

	// Determine contract version used by the InfraMachine.
	contractVersion, err := contract.GetContractVersion(ctx, r.Client, s.infraMachine.GroupVersionKind().GroupKind())
	if err != nil {
		return ctrl.Result{}, err
	}

	// Determine if the InfrastructureMachine is provisioned.
	var provisioned bool
	if provisionedPtr, err := contract.InfrastructureMachine().Provisioned(contractVersion).Get(s.infraMachine); err != nil {
		if !errors.Is(err, contract.ErrFieldNotFound) {
			return ctrl.Result{}, err
		}
	} else {
		provisioned = *provisionedPtr
	}
	if provisioned && !ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false) {
		log.Info("Infrastructure provider has completed provisioning", s.infraMachine.GetKind(), klog.KObj(s.infraMachine))
	}

	// Report a summary of current status of the InfrastructureMachine for this Machine.
	fallBack := v1beta1conditions.WithFallbackValue(provisioned, clusterv1.WaitingForInfrastructureFallbackV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
	if !s.machine.DeletionTimestamp.IsZero() {
		fallBack = v1beta1conditions.WithFallbackValue(provisioned, clusterv1.DeletingV1Beta1Reason, clusterv1.ConditionSeverityInfo, "")
	}
	v1beta1conditions.SetMirror(m, clusterv1.InfrastructureReadyV1Beta1Condition, v1beta1conditions.UnstructuredGetter(s.infraMachine), fallBack)

	if !s.infraMachine.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// If the InfrastructureMachine is not provisioned (and it wasn't already provisioned before), return.
	if !provisioned && !ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false) {
		log.Info(fmt.Sprintf("Waiting for infrastructure provider to create machine infrastructure and set %s",
			contract.InfrastructureMachine().Provisioned(contractVersion).Path().String()),
			s.infraMachine.GetKind(), klog.KObj(s.infraMachine))
		return ctrl.Result{}, nil
	}

	// Get providerID from the InfrastructureMachine (intentionally not setting it on the Machine yet).
	var providerID *string
	if providerID, err = contract.InfrastructureMachine().ProviderID().Get(s.infraMachine); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to read providerID from %s %s",
			s.infraMachine.GetKind(), klog.KObj(s.infraMachine))
	} else if *providerID == "" {
		return ctrl.Result{}, errors.Errorf("got empty %s field from %s %s",
			contract.InfrastructureMachine().ProviderID().Path().String(),
			s.infraMachine.GetKind(), klog.KObj(s.infraMachine))
	}

	// Get and set addresses from the InfrastructureMachine.
	addresses, err := contract.InfrastructureMachine().Addresses().Get(s.infraMachine)
	switch {
	case errors.Is(err, contract.ErrFieldNotFound): // no-op
	case err != nil:
		return ctrl.Result{}, errors.Wrapf(err, "failed to read addresses from %s %s",
			s.infraMachine.GetKind(), klog.KObj(s.infraMachine))
	default:
		m.Status.Addresses = *addresses
	}

	// Get and set failureDomain from the InfrastructureMachine.
	failureDomain, err := contract.InfrastructureMachine().FailureDomain().Get(s.infraMachine)
	switch {
	case errors.Is(err, contract.ErrFieldNotFound): // no-op
	case err != nil:
		return ctrl.Result{}, errors.Wrapf(err, "failed to read failureDomain from %s %s",
			s.infraMachine.GetKind(), klog.KObj(s.infraMachine))
	default:
		m.Spec.FailureDomain = ptr.Deref(failureDomain, "")
	}

	// When we hit this point providerID is set, and either:
	// - the infra machine is reporting provisioned for the first time
	// - the infra machine already reported provisioned (and thus m.Status.InfrastructureReady is already true and it should not flip back)
	m.Spec.ProviderID = *providerID
	m.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
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
		m.Status.CertificatesExpiryDate = expTime
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
			m.Status.CertificatesExpiryDate = expTime
		}
	}

	// If the certificates expiry information is not fond on the machine
	// and on the bootstrap config then reset machine.status.certificatesExpiryDate.
	if !expiryInfoFound {
		m.Status.CertificatesExpiryDate = metav1.Time{}
	}

	return ctrl.Result{}, nil
}

// removeOnCreateOwnerRefs will remove any MachineSet or control plane owner references from passed objects.
func removeOnCreateOwnerRefs(cluster *clusterv1.Cluster, m *clusterv1.Machine, obj *unstructured.Unstructured) error {
	cpGK := getControlPlaneGKForMachine(cluster, m)
	for _, owner := range obj.GetOwnerReferences() {
		ownerGV, err := schema.ParseGroupVersion(owner.APIVersion)
		if err != nil {
			return errors.Wrapf(err, "could not remove ownerReference %v from object %s/%s", owner.String(), obj.GetKind(), obj.GetName())
		}
		if (ownerGV.Group == clusterv1.GroupVersion.Group && owner.Kind == "MachineSet") ||
			(cpGK != nil && ownerGV.Group == cpGK.Group && owner.Kind == cpGK.Kind) {
			ownerRefs := util.RemoveOwnerRef(obj.GetOwnerReferences(), owner)
			obj.SetOwnerReferences(ownerRefs)
		}
	}
	return nil
}

// hasOnCreateOwnerRefs will check if any MachineSet or control plane owner references from passed objects are set.
func hasOnCreateOwnerRefs(cluster *clusterv1.Cluster, m *clusterv1.Machine, obj *unstructured.Unstructured) (bool, error) {
	cpGK := getControlPlaneGKForMachine(cluster, m)
	for _, owner := range obj.GetOwnerReferences() {
		ownerGV, err := schema.ParseGroupVersion(owner.APIVersion)
		if err != nil {
			return false, errors.Wrapf(err, "could not remove ownerReference %v from object %s/%s", owner.String(), obj.GetKind(), obj.GetName())
		}
		if (ownerGV.Group == clusterv1.GroupVersion.Group && owner.Kind == "MachineSet") ||
			(cpGK != nil && ownerGV.Group == cpGK.Group && owner.Kind == cpGK.Kind) {
			return true, nil
		}
	}
	return false, nil
}

// getControlPlaneGKForMachine returns the Kind of the control plane in the Cluster associated with the Machine.
// This function checks that the Machine is managed by a control plane, and then retrieves the Kind from the Cluster's
// .spec.controlPlaneRef.
func getControlPlaneGKForMachine(cluster *clusterv1.Cluster, machine *clusterv1.Machine) *schema.GroupKind {
	if _, ok := machine.GetLabels()[clusterv1.MachineControlPlaneLabel]; ok {
		if cluster.Spec.ControlPlaneRef.IsDefined() {
			gvk := cluster.Spec.ControlPlaneRef.GroupKind()
			return &gvk
		}
	}
	return nil
}
