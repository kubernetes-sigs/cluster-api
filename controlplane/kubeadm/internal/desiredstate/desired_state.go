/*
Copyright 2025 The Kubernetes Authors.

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

// Package desiredstate contains utils to compute the desired state of a Machine.
package desiredstate

import (
	"context"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/contract"
	topologynames "sigs.k8s.io/cluster-api/internal/topology/names"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/labels/format"
	"sigs.k8s.io/cluster-api/util/version"
)

var (
	// minKubernetesVersionControlPlaneKubeletLocalMode is the min version from which
	// we will enable the ControlPlaneKubeletLocalMode kubeadm feature gate.
	// Note: We have to do this with Kubernetes 1.31. Because with that version we encountered
	// a case where it's not okay anymore to ignore the Kubernetes version skew (kubelet 1.31 uses
	// the spec.clusterIP field selector that is only implemented in kube-apiserver >= 1.31.0).
	minKubernetesVersionControlPlaneKubeletLocalMode = semver.MustParse("1.31.0")

	// droppedKubernetesVersionControlPlaneKubeletLocalMode is the version from which
	// we will drop the ControlPlaneKubeletLocalMode kubeadm feature gate.
	// Starting with Kubernetes 1.36, this feature graduated to GA and the feature gate
	// is no longer needed (and will be removed in future K8s versions).
	droppedKubernetesVersionControlPlaneKubeletLocalMode = semver.MustParse("1.36.0")

	// ControlPlaneKubeletLocalMode is a feature gate of kubeadm that ensures
	// kubelets only communicate with the local apiserver.
	ControlPlaneKubeletLocalMode = "ControlPlaneKubeletLocalMode"
)

// MandatoryMachineReadinessGates are readinessGates KCP enforces to be set on machine it owns.
var MandatoryMachineReadinessGates = []clusterv1.MachineReadinessGate{
	{ConditionType: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition},
	{ConditionType: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition},
	{ConditionType: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition},
}

// etcdMandatoryMachineReadinessGates are readinessGates KCP enforces to be set on machine it owns if etcd is managed.
var etcdMandatoryMachineReadinessGates = []clusterv1.MachineReadinessGate{
	{ConditionType: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition},
	{ConditionType: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition},
}

// kubeadmClusterConfigurationAnnotation is an annotation that was set in Cluster API <= v1.11.
// Starting with Cluster API v1.12 we remove it from existing Machines.
//
// Deprecated: This constant and corresponding cleanup code can be removed once we don't support upgrades from Cluster API v1.12 anymore.
const kubeadmClusterConfigurationAnnotation = "controlplane.cluster.x-k8s.io/kubeadm-cluster-configuration"

// ComputeDesiredMachine computes the desired Machine.
// This Machine will be used during reconciliation to:
// * create a new Machine
// * update an existing Machine
// Because we are using Server-Side-Apply we always have to calculate the full object.
// There are small differences in how we calculate the Machine depending on if it
// is a create or update. Example: for a new Machine we have to calculate a new name,
// while for an existing Machine we have to use the name of the existing Machine.
func ComputeDesiredMachine(kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster, failureDomain string, existingMachine *clusterv1.Machine) (*clusterv1.Machine, error) {
	var machineName string
	var machineUID types.UID
	var version string
	annotations := map[string]string{}
	if existingMachine == nil {
		// Creating a new machine
		nameTemplate := "{{ .kubeadmControlPlane.name }}-{{ .random }}"
		if kcp.Spec.MachineNaming.Template != "" {
			nameTemplate = kcp.Spec.MachineNaming.Template
			if !strings.Contains(nameTemplate, "{{ .random }}") {
				return nil, errors.New("failed to compute desired Machine: cannot generate Machine name: {{ .random }} is missing in machineNaming.template")
			}
		}
		generatedMachineName, err := topologynames.KCPMachineNameGenerator(nameTemplate, cluster.Name, kcp.Name).GenerateName()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute desired Machine: failed to generate Machine name")
		}
		machineName = generatedMachineName
		version = kcp.Spec.Version

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

		// Cleanup the KubeadmClusterConfigurationAnnotation annotation that was set in Cluster API <= v1.11.
		delete(annotations, kubeadmClusterConfigurationAnnotation)

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
				*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
			},
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
	desiredMachine.Labels = ControlPlaneMachineLabels(kcp, cluster.Name)

	// Set annotations
	desiredMachine.Annotations = ControlPlaneMachineAnnotations(kcp)
	for k, v := range annotations {
		desiredMachine.Annotations[k] = v
	}

	// Set other in-place mutable fields
	desiredMachine.Spec.Deletion.NodeDrainTimeoutSeconds = kcp.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds
	desiredMachine.Spec.Deletion.NodeDeletionTimeoutSeconds = kcp.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds
	desiredMachine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = kcp.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds
	desiredMachine.Spec.Taints = kcp.Spec.MachineTemplate.Spec.Taints

	// Note: We intentionally don't set "minReadySeconds" on Machines because we consider it enough to have machine availability driven by readiness of control plane components.
	if existingMachine != nil {
		desiredMachine.Spec.InfrastructureRef = existingMachine.Spec.InfrastructureRef
		desiredMachine.Spec.Bootstrap.ConfigRef = existingMachine.Spec.Bootstrap.ConfigRef
	}

	// Set machines readiness gates
	allReadinessGates := []clusterv1.MachineReadinessGate{}
	allReadinessGates = append(allReadinessGates, MandatoryMachineReadinessGates...)
	isEtcdManaged := !kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.External.IsDefined()
	if isEtcdManaged {
		allReadinessGates = append(allReadinessGates, etcdMandatoryMachineReadinessGates...)
	}
	allReadinessGates = append(allReadinessGates, kcp.Spec.MachineTemplate.Spec.ReadinessGates...)

	desiredMachine.Spec.ReadinessGates = []clusterv1.MachineReadinessGate{}
	knownGates := sets.Set[string]{}
	for _, gate := range allReadinessGates {
		if knownGates.Has(gate.ConditionType) {
			continue
		}
		desiredMachine.Spec.ReadinessGates = append(desiredMachine.Spec.ReadinessGates, gate)
		knownGates.Insert(gate.ConditionType)
	}

	return desiredMachine, nil
}

// ComputeDesiredKubeadmConfig computes the desired KubeadmConfig.
func ComputeDesiredKubeadmConfig(kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster, isJoin bool, name string, existingKubeadmConfig *bootstrapv1.KubeadmConfig) (*bootstrapv1.KubeadmConfig, error) {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	var ownerReferences []metav1.OwnerReference
	if existingKubeadmConfig == nil || !util.HasOwner(existingKubeadmConfig.OwnerReferences, clusterv1.GroupVersion.String(), []string{"Machine"}) {
		ownerReferences = append(ownerReferences, metav1.OwnerReference{
			APIVersion: controlplanev1.GroupVersion.String(),
			Kind:       "KubeadmControlPlane",
			Name:       kcp.Name,
			UID:        kcp.UID,
		})
	}

	spec := kcp.Spec.KubeadmConfigSpec.DeepCopy()
	if isJoin {
		// Note: When building a KubeadmConfig for a joining CP machine empty out the unnecessary InitConfiguration.
		spec.InitConfiguration = bootstrapv1.InitConfiguration{}
		// Note: For the joining we are preserving the ClusterConfiguration in order to determine if the
		// cluster is using an external etcd in the kubeadm bootstrap provider (even if this is not required by kubeadm Join).
		// Note: We are always setting JoinConfiguration.ControlPlane so we can later identify this KubeadmConfig as a
		// join KubeadmConfig.
		if spec.JoinConfiguration.ControlPlane == nil {
			spec.JoinConfiguration.ControlPlane = &bootstrapv1.JoinControlPlane{}
		}
	} else {
		// Note: When building a KubeadmConfig for the first CP machine empty out the unnecessary JoinConfiguration.
		spec.JoinConfiguration = bootstrapv1.JoinConfiguration{}
	}

	parsedVersion, err := semver.ParseTolerant(kcp.Spec.Version)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compute desired KubeadmConfig: failed to parse Kubernetes version %q", kcp.Spec.Version)
	}
	DefaultFeatureGates(spec, parsedVersion)

	kubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       kcp.Namespace,
			Labels:          ControlPlaneMachineLabels(kcp, cluster.Name),
			Annotations:     ControlPlaneMachineAnnotations(kcp),
			OwnerReferences: ownerReferences,
		},
		Spec: *spec,
	}
	if existingKubeadmConfig != nil {
		kubeadmConfig.SetName(existingKubeadmConfig.GetName())
		kubeadmConfig.SetUID(existingKubeadmConfig.GetUID())
	}
	return kubeadmConfig, nil
}

// ComputeDesiredInfraMachine computes the desired InfraMachine.
func ComputeDesiredInfraMachine(ctx context.Context, c client.Client, kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster, name string, existingInfraMachine *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	var ownerReference *metav1.OwnerReference
	if existingInfraMachine == nil || !util.HasOwner(existingInfraMachine.GetOwnerReferences(), clusterv1.GroupVersion.String(), []string{"Machine"}) {
		ownerReference = &metav1.OwnerReference{
			APIVersion: controlplanev1.GroupVersion.String(),
			Kind:       "KubeadmControlPlane",
			Name:       kcp.Name,
			UID:        kcp.UID,
		}
	}

	apiVersion, err := contract.GetAPIVersion(ctx, c, kcp.Spec.MachineTemplate.Spec.InfrastructureRef.GroupKind())
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired InfraMachine")
	}
	templateRef := &corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       kcp.Spec.MachineTemplate.Spec.InfrastructureRef.Kind,
		Namespace:  kcp.Namespace,
		Name:       kcp.Spec.MachineTemplate.Spec.InfrastructureRef.Name,
	}

	template, err := external.Get(ctx, c, templateRef)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired InfraMachine")
	}
	generateTemplateInput := &external.GenerateTemplateInput{
		Template:    template,
		TemplateRef: templateRef,
		Namespace:   kcp.Namespace,
		Name:        name,
		ClusterName: cluster.Name,
		OwnerRef:    ownerReference,
		Labels:      ControlPlaneMachineLabels(kcp, cluster.Name),
		Annotations: ControlPlaneMachineAnnotations(kcp),
	}
	infraMachine, err := external.GenerateTemplate(generateTemplateInput)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute desired InfraMachine")
	}
	if existingInfraMachine != nil {
		infraMachine.SetName(existingInfraMachine.GetName())
		infraMachine.SetUID(existingInfraMachine.GetUID())
	}
	return infraMachine, nil
}

// DefaultFeatureGates defaults the feature gates field.
func DefaultFeatureGates(kubeadmConfigSpec *bootstrapv1.KubeadmConfigSpec, kubernetesVersion semver.Version) {
	// Only set ControlPlaneKubeletLocalMode for Kubernetes versions 1.31 <= version < 1.36
	// For K8s < 1.31: feature gate doesn't exist
	// For K8s >= 1.36: feature graduated to GA and gate does not exist anymore
	if version.Compare(kubernetesVersion, minKubernetesVersionControlPlaneKubeletLocalMode, version.WithoutPreReleases()) < 0 {
		return
	}
	if version.Compare(kubernetesVersion, droppedKubernetesVersionControlPlaneKubeletLocalMode, version.WithoutPreReleases()) >= 0 {
		return
	}

	if kubeadmConfigSpec.ClusterConfiguration.FeatureGates == nil {
		kubeadmConfigSpec.ClusterConfiguration.FeatureGates = map[string]bool{}
	}

	if _, ok := kubeadmConfigSpec.ClusterConfiguration.FeatureGates[ControlPlaneKubeletLocalMode]; !ok {
		kubeadmConfigSpec.ClusterConfiguration.FeatureGates[ControlPlaneKubeletLocalMode] = true
	}
}

// ControlPlaneMachineLabels returns a set of labels to add to a control plane machine for this specific cluster.
func ControlPlaneMachineLabels(kcp *controlplanev1.KubeadmControlPlane, clusterName string) map[string]string {
	labels := map[string]string{}

	// Add the labels from the MachineTemplate.
	// Note: we intentionally don't use the map directly to ensure we don't modify the map in KCP.
	for k, v := range kcp.Spec.MachineTemplate.ObjectMeta.Labels {
		labels[k] = v
	}

	// Always force these labels over the ones coming from the spec.
	labels[clusterv1.ClusterNameLabel] = clusterName
	labels[clusterv1.MachineControlPlaneLabel] = ""
	// Note: MustFormatValue is used here as the label value can be a hash if the control plane name is longer than 63 characters.
	labels[clusterv1.MachineControlPlaneNameLabel] = format.MustFormatValue(kcp.Name)
	return labels
}

// ControlPlaneMachineAnnotations returns a set of annotations to add to a control plane machine for this specific cluster.
func ControlPlaneMachineAnnotations(kcp *controlplanev1.KubeadmControlPlane) map[string]string {
	annotations := map[string]string{}

	// Add the annotations from the MachineTemplate.
	// Note: we intentionally don't use the map directly to ensure we don't modify the map in KCP.
	for k, v := range kcp.Spec.MachineTemplate.ObjectMeta.Annotations {
		annotations[k] = v
	}

	return annotations
}
