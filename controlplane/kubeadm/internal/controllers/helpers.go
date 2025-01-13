/*
Copyright 2020 The Kubernetes Authors.

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
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/storage/names"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	topologynames "sigs.k8s.io/cluster-api/internal/topology/names"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/conditions"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
)

// mandatoryMachineReadinessGates are readinessGates KCP enforces to be set on machine it owns.
var mandatoryMachineReadinessGates = []clusterv1.MachineReadinessGate{
	{ConditionType: string(controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition)},
	{ConditionType: string(controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition)},
	{ConditionType: string(controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition)},
	{ConditionType: string(controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition)},
	{ConditionType: string(controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition)},
}

func (r *KubeadmControlPlaneReconciler) reconcileKubeconfig(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	endpoint := controlPlane.Cluster.Spec.ControlPlaneEndpoint
	if endpoint.IsZero() {
		return ctrl.Result{}, nil
	}

	controllerOwnerRef := *metav1.NewControllerRef(controlPlane.KCP, controlplanev1.GroupVersion.WithKind(kubeadmControlPlaneKind))
	clusterName := util.ObjectKey(controlPlane.Cluster)
	configSecret, err := secret.GetFromNamespacedName(ctx, r.SecretCachingClient, clusterName, secret.Kubeconfig)
	switch {
	case apierrors.IsNotFound(err):
		createErr := kubeconfig.CreateSecretWithOwner(
			ctx,
			r.SecretCachingClient,
			clusterName,
			endpoint.String(),
			controllerOwnerRef,
		)
		if errors.Is(createErr, kubeconfig.ErrDependentCertificateNotFound) {
			return ctrl.Result{RequeueAfter: dependentCertRequeueAfter}, nil
		}
		// always return if we have just created in order to skip rotation checks
		return ctrl.Result{}, createErr
	case err != nil:
		return ctrl.Result{}, errors.Wrap(err, "failed to retrieve kubeconfig Secret")
	}

	if err := r.adoptKubeconfigSecret(ctx, configSecret, controlPlane.KCP); err != nil {
		return ctrl.Result{}, err
	}

	// only do rotation on owned secrets
	if !util.IsControlledBy(configSecret, controlPlane.KCP) {
		return ctrl.Result{}, nil
	}

	needsRotation, err := kubeconfig.NeedsClientCertRotation(configSecret, certs.ClientCertificateRenewalDuration)
	if err != nil {
		return ctrl.Result{}, err
	}

	if needsRotation {
		log.Info("Rotating kubeconfig secret")
		if err := kubeconfig.RegenerateSecret(ctx, r.Client, configSecret); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to regenerate kubeconfig")
		}
	}

	return ctrl.Result{}, nil
}

// Ensure the KubeadmConfigSecret has an owner reference to the control plane if it is not a user-provided secret.
func (r *KubeadmControlPlaneReconciler) adoptKubeconfigSecret(ctx context.Context, configSecret *corev1.Secret, kcp *controlplanev1.KubeadmControlPlane) (reterr error) {
	patchHelper, err := patch.NewHelper(configSecret, r.Client)
	if err != nil {
		return err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, configSecret); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()
	controller := metav1.GetControllerOf(configSecret)

	// If the current controller is KCP, ensure the owner reference is up to date and return early.
	// Note: This ensures secrets created prior to v1alpha4 are updated to have the correct owner reference apiVersion.
	if controller != nil && controller.Kind == kubeadmControlPlaneKind {
		configSecret.SetOwnerReferences(util.EnsureOwnerRef(configSecret.GetOwnerReferences(), *metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind(kubeadmControlPlaneKind))))
		return nil
	}

	// If secret type is a CAPI-created secret ensure the owner reference is to KCP.
	if configSecret.Type == clusterv1.ClusterSecretType {
		// Remove the current controller if one exists and ensure KCP is the controller of the secret.
		if controller != nil {
			configSecret.SetOwnerReferences(util.RemoveOwnerRef(configSecret.GetOwnerReferences(), *controller))
		}
		configSecret.SetOwnerReferences(util.EnsureOwnerRef(configSecret.GetOwnerReferences(), *metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind(kubeadmControlPlaneKind))))
	}
	return nil
}

func (r *KubeadmControlPlaneReconciler) reconcileExternalReference(ctx context.Context, controlPlane *internal.ControlPlane) error {
	ref := &controlPlane.KCP.Spec.MachineTemplate.InfrastructureRef
	if !strings.HasSuffix(ref.Kind, clusterv1.TemplateSuffix) {
		return nil
	}

	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, ref); err != nil {
		return err
	}

	obj, err := external.Get(ctx, r.Client, ref)
	if err != nil {
		if apierrors.IsNotFound(err) {
			controlPlane.InfraMachineTemplateIsNotFound = true
		}
		return err
	}

	// Note: We intentionally do not handle checking for the paused label on an external template reference

	desiredOwnerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       controlPlane.Cluster.Name,
		UID:        controlPlane.Cluster.UID,
	}

	if util.HasExactOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef) {
		return nil
	}

	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return err
	}

	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), desiredOwnerRef))

	return patchHelper.Patch(ctx, obj)
}

func (r *KubeadmControlPlaneReconciler) cloneConfigsAndGenerateMachine(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, bootstrapSpec *bootstrapv1.KubeadmConfigSpec, failureDomain *string) error {
	var errs []error

	// Compute desired Machine
	machine, err := r.computeDesiredMachine(kcp, cluster, failureDomain, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create Machine: failed to compute desired Machine")
	}

	// Since the cloned resource should eventually have a controller ref for the Machine, we create an
	// OwnerReference here without the Controller field set
	infraCloneOwner := &metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       kubeadmControlPlaneKind,
		Name:       kcp.Name,
		UID:        kcp.UID,
	}

	infraMachineName := machine.Name
	if r.DeprecatedInfraMachineNaming {
		infraMachineName = names.SimpleNameGenerator.GenerateName(kcp.Spec.MachineTemplate.InfrastructureRef.Name + "-")
	}

	// Clone the infrastructure template
	infraRef, err := external.CreateFromTemplate(ctx, &external.CreateFromTemplateInput{
		Client:      r.Client,
		TemplateRef: &kcp.Spec.MachineTemplate.InfrastructureRef,
		Namespace:   kcp.Namespace,
		Name:        infraMachineName,
		OwnerRef:    infraCloneOwner,
		ClusterName: cluster.Name,
		Labels:      internal.ControlPlaneMachineLabelsForCluster(kcp, cluster.Name),
		Annotations: kcp.Spec.MachineTemplate.ObjectMeta.Annotations,
	})
	if err != nil {
		// Safe to return early here since no resources have been created yet.
		conditions.MarkFalse(kcp, controlplanev1.MachinesCreatedCondition, controlplanev1.InfrastructureTemplateCloningFailedReason,
			clusterv1.ConditionSeverityError, err.Error())
		return errors.Wrap(err, "failed to clone infrastructure template")
	}
	machine.Spec.InfrastructureRef = *infraRef

	// Clone the bootstrap configuration
	bootstrapRef, err := r.generateKubeadmConfig(ctx, kcp, cluster, bootstrapSpec, machine.Name)
	if err != nil {
		conditions.MarkFalse(kcp, controlplanev1.MachinesCreatedCondition, controlplanev1.BootstrapTemplateCloningFailedReason,
			clusterv1.ConditionSeverityError, err.Error())
		errs = append(errs, errors.Wrap(err, "failed to generate bootstrap config"))
	}

	// Only proceed to generating the Machine if we haven't encountered an error
	if len(errs) == 0 {
		machine.Spec.Bootstrap.ConfigRef = bootstrapRef

		if err := r.createMachine(ctx, kcp, machine); err != nil {
			conditions.MarkFalse(kcp, controlplanev1.MachinesCreatedCondition, controlplanev1.MachineGenerationFailedReason,
				clusterv1.ConditionSeverityError, err.Error())
			errs = append(errs, errors.Wrap(err, "failed to create Machine"))
		}
	}

	// If we encountered any errors, attempt to clean up any dangling resources
	if len(errs) > 0 {
		if err := r.cleanupFromGeneration(ctx, infraRef, bootstrapRef); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to cleanup generated resources"))
		}

		return kerrors.NewAggregate(errs)
	}

	return nil
}

func (r *KubeadmControlPlaneReconciler) cleanupFromGeneration(ctx context.Context, remoteRefs ...*corev1.ObjectReference) error {
	var errs []error

	for _, ref := range remoteRefs {
		if ref == nil {
			continue
		}
		config := &unstructured.Unstructured{}
		config.SetKind(ref.Kind)
		config.SetAPIVersion(ref.APIVersion)
		config.SetNamespace(ref.Namespace)
		config.SetName(ref.Name)

		if err := r.Client.Delete(ctx, config); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, errors.Wrap(err, "failed to cleanup generated resources after error"))
		}
	}

	return kerrors.NewAggregate(errs)
}

func (r *KubeadmControlPlaneReconciler) generateKubeadmConfig(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster, spec *bootstrapv1.KubeadmConfigSpec, name string) (*corev1.ObjectReference, error) {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	owner := metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       kubeadmControlPlaneKind,
		Name:       kcp.Name,
		UID:        kcp.UID,
	}

	bootstrapConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       kcp.Namespace,
			Labels:          internal.ControlPlaneMachineLabelsForCluster(kcp, cluster.Name),
			Annotations:     kcp.Spec.MachineTemplate.ObjectMeta.Annotations,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: *spec,
	}

	if err := r.Client.Create(ctx, bootstrapConfig); err != nil {
		return nil, errors.Wrap(err, "Failed to create bootstrap configuration")
	}

	bootstrapRef := &corev1.ObjectReference{
		APIVersion: bootstrapv1.GroupVersion.String(),
		Kind:       "KubeadmConfig",
		Name:       bootstrapConfig.GetName(),
		Namespace:  bootstrapConfig.GetNamespace(),
		UID:        bootstrapConfig.GetUID(),
	}

	return bootstrapRef, nil
}

// updateExternalObject updates the external object with the labels and annotations from KCP.
func (r *KubeadmControlPlaneReconciler) updateExternalObject(ctx context.Context, obj client.Object, kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster) error {
	updatedObject := &unstructured.Unstructured{}
	updatedObject.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	updatedObject.SetNamespace(obj.GetNamespace())
	updatedObject.SetName(obj.GetName())
	// Set the UID to ensure that Server-Side-Apply only performs an update
	// and does not perform an accidental create.
	updatedObject.SetUID(obj.GetUID())

	// Update labels
	updatedObject.SetLabels(internal.ControlPlaneMachineLabelsForCluster(kcp, cluster.Name))
	// Update annotations
	updatedObject.SetAnnotations(kcp.Spec.MachineTemplate.ObjectMeta.Annotations)

	if err := ssa.Patch(ctx, r.Client, kcpManagerName, updatedObject, ssa.WithCachingProxy{Cache: r.ssaCache, Original: obj}); err != nil {
		return errors.Wrapf(err, "failed to update %s", obj.GetObjectKind().GroupVersionKind().Kind)
	}
	return nil
}

func (r *KubeadmControlPlaneReconciler) createMachine(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, machine *clusterv1.Machine) error {
	if err := ssa.Patch(ctx, r.Client, kcpManagerName, machine); err != nil {
		return errors.Wrap(err, "failed to create Machine")
	}
	// Remove the annotation tracking that a remediation is in progress (the remediation completed when
	// the replacement machine has been created above).
	delete(kcp.Annotations, controlplanev1.RemediationInProgressAnnotation)
	return nil
}

func (r *KubeadmControlPlaneReconciler) updateMachine(ctx context.Context, machine *clusterv1.Machine, kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster) (*clusterv1.Machine, error) {
	updatedMachine, err := r.computeDesiredMachine(kcp, cluster, machine.Spec.FailureDomain, machine)
	if err != nil {
		return nil, errors.Wrap(err, "failed to update Machine: failed to compute desired Machine")
	}

	err = ssa.Patch(ctx, r.Client, kcpManagerName, updatedMachine, ssa.WithCachingProxy{Cache: r.ssaCache, Original: machine})
	if err != nil {
		return nil, errors.Wrap(err, "failed to update Machine")
	}
	return updatedMachine, nil
}

// computeDesiredMachine computes the desired Machine.
// This Machine will be used during reconciliation to:
// * create a new Machine
// * update an existing Machine
// Because we are using Server-Side-Apply we always have to calculate the full object.
// There are small differences in how we calculate the Machine depending on if it
// is a create or update. Example: for a new Machine we have to calculate a new name,
// while for an existing Machine we have to use the name of the existing Machine.
func (r *KubeadmControlPlaneReconciler) computeDesiredMachine(kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster, failureDomain *string, existingMachine *clusterv1.Machine) (*clusterv1.Machine, error) {
	var machineName string
	var machineUID types.UID
	var version *string
	annotations := map[string]string{}
	if existingMachine == nil {
		// Creating a new machine
		nameTemplate := "{{ .kubeadmControlPlane.name }}-{{ .random }}"
		if kcp.Spec.MachineNamingStrategy != nil && kcp.Spec.MachineNamingStrategy.Template != "" {
			nameTemplate = kcp.Spec.MachineNamingStrategy.Template
			if !strings.Contains(nameTemplate, "{{ .random }}") {
				return nil, errors.New("cannot generate KCP machine name: {{ .random }} is missing in machineNamingStrategy.template")
			}
		}
		generatedMachineName, err := topologynames.KCPMachineNameGenerator(nameTemplate, cluster.Name, kcp.Name).GenerateName()
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate name for KCP Machine")
		}
		machineName = generatedMachineName
		version = &kcp.Spec.Version

		// Machine's bootstrap config may be missing ClusterConfiguration if it is not the first machine in the control plane.
		// We store ClusterConfiguration as annotation here to detect any changes in KCP ClusterConfiguration and rollout the machine if any.
		// Nb. This annotation is read when comparing the KubeadmConfig to check if a machine needs to be rolled out.
		clusterConfig, err := json.Marshal(kcp.Spec.KubeadmConfigSpec.ClusterConfiguration)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal cluster configuration")
		}
		annotations[controlplanev1.KubeadmClusterConfigurationAnnotation] = string(clusterConfig)

		// In case this machine is being created as a consequence of a remediation, then add an annotation
		// tracking remediating data.
		// NOTE: This is required in order to track remediation retries.
		if remediationData, ok := kcp.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok {
			annotations[controlplanev1.RemediationForAnnotation] = remediationData
		}
	} else {
		// Updating an existing machine
		machineName = existingMachine.Name
		machineUID = existingMachine.UID
		version = existingMachine.Spec.Version

		// For existing machine only set the ClusterConfiguration annotation if the machine already has it.
		// We should not add the annotation if it was missing in the first place because we do not have enough
		// information.
		if clusterConfig, ok := existingMachine.Annotations[controlplanev1.KubeadmClusterConfigurationAnnotation]; ok {
			annotations[controlplanev1.KubeadmClusterConfigurationAnnotation] = clusterConfig
		}

		// If the machine already has remediation data then preserve it.
		// NOTE: This is required in order to track remediation retries.
		if remediationData, ok := existingMachine.Annotations[controlplanev1.RemediationForAnnotation]; ok {
			annotations[controlplanev1.RemediationForAnnotation] = remediationData
		}
	}
	// Setting pre-terminate hook so we can later remove the etcd member right before Machine termination
	// (i.e. before InfraMachine deletion).
	annotations[controlplanev1.PreTerminateHookCleanupAnnotation] = ""

	// Construct the basic Machine.
	desiredMachine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			UID:       machineUID,
			Name:      machineName,
			Namespace: kcp.Namespace,
			// Note: by setting the ownerRef on creation we signal to the Machine controller that this is not a stand-alone Machine.
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind(kubeadmControlPlaneKind)),
			},
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:   cluster.Name,
			Version:       version,
			FailureDomain: failureDomain,
		},
	}

	// Set the in-place mutable fields.
	// When we create a new Machine we will just create the Machine with those fields.
	// When we update an existing Machine will we update the fields on the existing Machine (in-place mutate).

	// Set labels
	desiredMachine.Labels = internal.ControlPlaneMachineLabelsForCluster(kcp, cluster.Name)

	// Set annotations
	// Add the annotations from the MachineTemplate.
	// Note: we intentionally don't use the map directly to ensure we don't modify the map in KCP.
	for k, v := range kcp.Spec.MachineTemplate.ObjectMeta.Annotations {
		desiredMachine.Annotations[k] = v
	}
	for k, v := range annotations {
		desiredMachine.Annotations[k] = v
	}

	// Set other in-place mutable fields
	desiredMachine.Spec.NodeDrainTimeout = kcp.Spec.MachineTemplate.NodeDrainTimeout
	desiredMachine.Spec.NodeDeletionTimeout = kcp.Spec.MachineTemplate.NodeDeletionTimeout
	desiredMachine.Spec.NodeVolumeDetachTimeout = kcp.Spec.MachineTemplate.NodeVolumeDetachTimeout

	if existingMachine != nil {
		desiredMachine.Spec.InfrastructureRef = existingMachine.Spec.InfrastructureRef
		desiredMachine.Spec.Bootstrap.ConfigRef = existingMachine.Spec.Bootstrap.ConfigRef
		desiredMachine.Spec.ReadinessGates = existingMachine.Spec.ReadinessGates
	}
	ensureMandatoryReadinessGates(desiredMachine)

	return desiredMachine, nil
}

func ensureMandatoryReadinessGates(m *clusterv1.Machine) {
	if m.Spec.ReadinessGates == nil {
		m.Spec.ReadinessGates = mandatoryMachineReadinessGates
		return
	}

	for _, want := range mandatoryMachineReadinessGates {
		found := false
		for _, got := range m.Spec.ReadinessGates {
			if got.ConditionType == want.ConditionType {
				found = true
				break
			}
		}
		if !found {
			m.Spec.ReadinessGates = append(m.Spec.ReadinessGates, want)
		}
	}
}
