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
	"sigs.k8s.io/cluster-api/util/conditions"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

var externalReadyWait = 30 * time.Second

// reconcileExternal handles generic unstructured objects referenced by a Machine.
func (r *Reconciler) reconcileExternal(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, ref); err != nil {
		if apierrors.IsNotFound(err) {
			// We want to surface the NotFound error only for the referenced object, so we use a generic error in case CRD is not found.
			return nil, errors.New(err.Error())
		}
		return nil, err
	}

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
		m.Status.FailureReason = &machineStatusError
	}
	if failureMessage != "" {
		m.Status.FailureMessage = ptr.To(
			fmt.Sprintf("Failure detected from referenced resource %v with name %q: %s",
				obj.GroupVersionKind(), obj.GetName(), failureMessage),
		)
	}

	return obj, nil
}

// ensureExternalOwnershipAndWatch ensures that only the Machine owns the external object,
// adds a watch to the external object if one does not already exist and adds the necessary labels.
func (r *Reconciler) ensureExternalOwnershipAndWatch(ctx context.Context, cluster *clusterv1.Cluster, m *clusterv1.Machine, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	log := ctrl.LoggerFrom(ctx)

	obj, err := external.Get(ctx, r.Client, ref)
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
	obj, err := r.reconcileExternal(ctx, cluster, m, m.Spec.Bootstrap.ConfigRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.bootstrapConfigIsNotFound = true

			if !s.machine.DeletionTimestamp.IsZero() {
				// Tolerate bootstrap object not found when the machine is being deleted.
				// TODO: we can also relax this and tolerate the absence of the bootstrap ref way before, e.g. after node ref is set
				return ctrl.Result{}, nil
			}
			log.Info("Could not find bootstrap config object, requeuing", m.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(m.Spec.Bootstrap.ConfigRef.Namespace, m.Spec.Bootstrap.ConfigRef.Name))
			// TODO: we can make this smarter and requeue only if we are before node ref is set
			return ctrl.Result{RequeueAfter: externalReadyWait}, nil
		}
		return ctrl.Result{}, err
	}
	s.bootstrapConfig = obj

	// If the bootstrap data is populated, set ready and return.
	if m.Spec.Bootstrap.DataSecretName != nil {
		m.Status.BootstrapReady = true
		conditions.MarkTrue(m, clusterv1.BootstrapReadyCondition)
		return ctrl.Result{}, nil
	}

	// Determine if the bootstrap provider is ready.
	ready, err := external.IsReady(s.bootstrapConfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Report a summary of current status of the bootstrap object defined for this machine.
	fallBack := conditions.WithFallbackValue(ready, clusterv1.WaitingForDataSecretFallbackReason, clusterv1.ConditionSeverityInfo, "")
	if !s.machine.DeletionTimestamp.IsZero() {
		fallBack = conditions.WithFallbackValue(ready, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
	}
	conditions.SetMirror(m, clusterv1.BootstrapReadyCondition,
		conditions.UnstructuredGetter(s.bootstrapConfig),
		fallBack,
	)

	if !s.bootstrapConfig.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// If the bootstrap provider is not ready, return.
	if !ready {
		log.Info("Waiting for bootstrap provider to generate data secret and report status.ready", s.bootstrapConfig.GetKind(), klog.KObj(s.bootstrapConfig))
		return ctrl.Result{}, nil
	}

	// Get and set the name of the secret containing the bootstrap data.
	secretName, _, err := unstructured.NestedString(s.bootstrapConfig.Object, "status", "dataSecretName")
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve dataSecretName from bootstrap provider for Machine %q in namespace %q", m.Name, m.Namespace)
	} else if secretName == "" {
		return ctrl.Result{}, errors.Errorf("retrieved empty dataSecretName from bootstrap provider for Machine %q in namespace %q", m.Name, m.Namespace)
	}
	m.Spec.Bootstrap.DataSecretName = ptr.To(secretName)
	if !m.Status.BootstrapReady {
		log.Info("Bootstrap provider generated data secret and reports status.ready", s.bootstrapConfig.GetKind(), klog.KObj(s.bootstrapConfig), "Secret", klog.KRef(m.Namespace, secretName))
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
	obj, err := r.reconcileExternal(ctx, cluster, m, &m.Spec.InfrastructureRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.infraMachineIsNotFound = true

			if !s.machine.DeletionTimestamp.IsZero() {
				// Tolerate infra machine not found when the machine is being deleted.
				return ctrl.Result{}, nil
			}

			if m.Status.InfrastructureReady {
				// Infra object went missing after the machine was up and running
				log.Error(err, "Machine infrastructure reference has been deleted after being ready, setting failure state")
				m.Status.FailureReason = ptr.To(capierrors.InvalidConfigurationMachineError)
				m.Status.FailureMessage = ptr.To(fmt.Sprintf("Machine infrastructure resource %v with name %q has been deleted after being ready",
					m.Spec.InfrastructureRef.GroupVersionKind(), m.Spec.InfrastructureRef.Name))
				return ctrl.Result{}, errors.Errorf("could not find %v %q for Machine %q in namespace %q", m.Spec.InfrastructureRef.GroupVersionKind().String(), m.Spec.InfrastructureRef.Name, m.Name, m.Namespace)
			}
			log.Info("Could not find infrastructure machine, requeuing", m.Spec.InfrastructureRef.Kind, klog.KRef(m.Spec.InfrastructureRef.Namespace, m.Spec.InfrastructureRef.Name))
			return ctrl.Result{RequeueAfter: externalReadyWait}, nil
		}
		return ctrl.Result{}, err
	}
	s.infraMachine = obj

	// Determine if the infrastructure provider is ready.
	ready, err := external.IsReady(s.infraMachine)
	if err != nil {
		return ctrl.Result{}, err
	}
	if ready && !m.Status.InfrastructureReady {
		log.Info("Infrastructure provider has completed machine infrastructure provisioning and reports status.ready", s.infraMachine.GetKind(), klog.KObj(s.infraMachine))
	}

	// Report a summary of current status of the infrastructure object defined for this machine.
	fallBack := conditions.WithFallbackValue(ready, clusterv1.WaitingForInfrastructureFallbackReason, clusterv1.ConditionSeverityInfo, "")
	if !s.machine.DeletionTimestamp.IsZero() {
		fallBack = conditions.WithFallbackValue(ready, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
	}
	conditions.SetMirror(m, clusterv1.InfrastructureReadyCondition,
		conditions.UnstructuredGetter(s.infraMachine),
		fallBack,
	)

	if !s.infraMachine.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// If the infrastructure provider is not ready (and it wasn't ready before), return early.
	if !ready && !m.Status.InfrastructureReady {
		log.Info("Waiting for infrastructure provider to create machine infrastructure and report status.ready", s.infraMachine.GetKind(), klog.KObj(s.infraMachine))
		return ctrl.Result{}, nil
	}

	// Get Spec.ProviderID from the infrastructure provider.
	var providerID string
	if err := util.UnstructuredUnmarshalField(s.infraMachine, &providerID, "spec", "providerID"); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve Spec.ProviderID from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	} else if providerID == "" {
		return ctrl.Result{}, errors.Errorf("retrieved empty Spec.ProviderID from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	}

	// Get and set Status.Addresses from the infrastructure provider.
	err = util.UnstructuredUnmarshalField(s.infraMachine, &m.Status.Addresses, "status", "addresses")
	if err != nil && err != util.ErrUnstructuredFieldNotFound {
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve addresses from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	}

	// Get and set the failure domain from the infrastructure provider.
	var failureDomain string
	err = util.UnstructuredUnmarshalField(s.infraMachine, &failureDomain, "spec", "failureDomain")
	switch {
	case err == util.ErrUnstructuredFieldNotFound: // no-op
	case err != nil:
		return ctrl.Result{}, errors.Wrapf(err, "failed to retrieve failure domain from infrastructure provider for Machine %q in namespace %q", m.Name, m.Namespace)
	default:
		m.Spec.FailureDomain = ptr.To(failureDomain)
	}

	// When we hit this point provider id is set, and either:
	// - the infra machine is reporting ready for the first time
	// - the infra machine already reported ready (and thus m.Status.InfrastructureReady is already true and it should not flip back)
	m.Spec.ProviderID = ptr.To(providerID)
	m.Status.InfrastructureReady = true
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
			return errors.Wrapf(err, "could not remove ownerReference %v from object %s/%s", owner.String(), obj.GetKind(), obj.GetName())
		}
		if (ownerGV.Group == clusterv1.GroupVersion.Group && owner.Kind == "MachineSet") ||
			(cpGVK != nil && ownerGV.Group == cpGVK.GroupVersion().Group && owner.Kind == cpGVK.Kind) {
			ownerRefs := util.RemoveOwnerRef(obj.GetOwnerReferences(), owner)
			obj.SetOwnerReferences(ownerRefs)
		}
	}
	return nil
}

// hasOnCreateOwnerRefs will check if any MachineSet or control plane owner references from passed objects are set.
func hasOnCreateOwnerRefs(cluster *clusterv1.Cluster, m *clusterv1.Machine, obj *unstructured.Unstructured) (bool, error) {
	cpGVK := getControlPlaneGVKForMachine(cluster, m)
	for _, owner := range obj.GetOwnerReferences() {
		ownerGV, err := schema.ParseGroupVersion(owner.APIVersion)
		if err != nil {
			return false, errors.Wrapf(err, "could not remove ownerReference %v from object %s/%s", owner.String(), obj.GetKind(), obj.GetName())
		}
		if (ownerGV.Group == clusterv1.GroupVersion.Group && owner.Kind == "MachineSet") ||
			(cpGVK != nil && ownerGV.Group == cpGVK.GroupVersion().Group && owner.Kind == cpGVK.Kind) {
			return true, nil
		}
	}
	return false, nil
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
