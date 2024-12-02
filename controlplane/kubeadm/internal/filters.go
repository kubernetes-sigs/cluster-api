/*
Copyright 2021 The Kubernetes Authors.

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

package internal

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/util/compare"
	"sigs.k8s.io/cluster-api/util/collections"
)

// matchesMachineSpec checks if a Machine matches any of a set of KubeadmConfigs and a set of infra machine configs.
// If it doesn't, it returns the reasons why.
// Kubernetes version, infrastructure template, and KubeadmConfig field need to be equivalent.
// Note: We don't need to compare the entire MachineSpec to determine if a Machine needs to be rolled out,
// because all the fields in the MachineSpec, except for version, the infrastructureRef and bootstrap.ConfigRef, are either:
// - mutated in-place (ex: NodeDrainTimeout)
// - are not dictated by KCP (ex: ProviderID)
// - are not relevant for the rollout decision (ex: failureDomain).
func matchesMachineSpec(infraConfigs map[string]*unstructured.Unstructured, machineConfigs map[string]*bootstrapv1.KubeadmConfig, kcp *controlplanev1.KubeadmControlPlane, machine *clusterv1.Machine) (bool, []string, []string, error) {
	logMessages := []string{}
	conditionMessages := []string{}

	if !collections.MatchesKubernetesVersion(kcp.Spec.Version)(machine) {
		machineVersion := ""
		if machine != nil && machine.Spec.Version != nil {
			machineVersion = *machine.Spec.Version
		}
		logMessages = append(logMessages, fmt.Sprintf("Machine version %q is not equal to KCP version %q", machineVersion, kcp.Spec.Version))
		// Note: the code computing the message for KCP's RolloutOut condition is making assumptions on the format/content of this message.
		conditionMessages = append(conditionMessages, fmt.Sprintf("Version %s, %s required", machineVersion, kcp.Spec.Version))
	}

	reason, matches, err := matchesKubeadmBootstrapConfig(machineConfigs, kcp, machine)
	if err != nil {
		return false, nil, nil, errors.Wrapf(err, "failed to match Machine spec")
	}
	if !matches {
		logMessages = append(logMessages, reason)
		conditionMessages = append(conditionMessages, "KubeadmConfig is not up-to-date")
	}

	if reason, matches := matchesTemplateClonedFrom(infraConfigs, kcp, machine); !matches {
		logMessages = append(logMessages, reason)
		conditionMessages = append(conditionMessages, fmt.Sprintf("%s is not up-to-date", machine.Spec.InfrastructureRef.Kind))
	}

	if len(logMessages) > 0 || len(conditionMessages) > 0 {
		return false, logMessages, conditionMessages, nil
	}

	return true, nil, nil, nil
}

// UpToDate checks if a Machine is up to date with the control plane's configuration.
// If not, messages explaining why are provided with different level of detail for logs and conditions.
func UpToDate(machine *clusterv1.Machine, kcp *controlplanev1.KubeadmControlPlane, reconciliationTime *metav1.Time, infraConfigs map[string]*unstructured.Unstructured, machineConfigs map[string]*bootstrapv1.KubeadmConfig) (bool, []string, []string, error) {
	logMessages := []string{}
	conditionMessages := []string{}

	// Machines whose certificates are about to expire.
	if collections.ShouldRolloutBefore(reconciliationTime, kcp.Spec.RolloutBefore)(machine) {
		logMessages = append(logMessages, "certificates will expire soon, rolloutBefore expired")
		conditionMessages = append(conditionMessages, "Certificates will expire soon")
	}

	// Machines that are scheduled for rollout (KCP.Spec.RolloutAfter set,
	// the RolloutAfter deadline is expired, and the machine was created before the deadline).
	if collections.ShouldRolloutAfter(reconciliationTime, kcp.Spec.RolloutAfter)(machine) {
		logMessages = append(logMessages, "rolloutAfter expired")
		conditionMessages = append(conditionMessages, "KubeadmControlPlane spec.rolloutAfter expired")
	}

	// Machines that do not match with KCP config.
	matches, specLogMessages, specConditionMessages, err := matchesMachineSpec(infraConfigs, machineConfigs, kcp, machine)
	if err != nil {
		return false, nil, nil, errors.Wrapf(err, "failed to determine if Machine %s is up-to-date", machine.Name)
	}
	if !matches {
		logMessages = append(logMessages, specLogMessages...)
		conditionMessages = append(conditionMessages, specConditionMessages...)
	}

	if len(logMessages) > 0 || len(conditionMessages) > 0 {
		return false, logMessages, conditionMessages, nil
	}

	return true, nil, nil, nil
}

// matchesTemplateClonedFrom checks if a Machine has a corresponding infrastructure machine that
// matches a given KCP infra template and if it doesn't match returns the reason why.
// Note: Differences to the labels and annotations on the infrastructure machine are not considered for matching
// criteria, because changes to labels and annotations are propagated in-place to the infrastructure machines.
// TODO: This function will be renamed in a follow-up PR to something better. (ex: MatchesInfraMachine).
func matchesTemplateClonedFrom(infraConfigs map[string]*unstructured.Unstructured, kcp *controlplanev1.KubeadmControlPlane, machine *clusterv1.Machine) (string, bool) {
	if machine == nil {
		return "Machine cannot be compared with KCP.spec.machineTemplate.infrastructureRef: Machine is nil", false
	}
	infraObj, found := infraConfigs[machine.Name]
	if !found {
		// Return true here because failing to get infrastructure machine should not be considered as unmatching.
		return "", true
	}

	clonedFromName, ok1 := infraObj.GetAnnotations()[clusterv1.TemplateClonedFromNameAnnotation]
	clonedFromGroupKind, ok2 := infraObj.GetAnnotations()[clusterv1.TemplateClonedFromGroupKindAnnotation]
	if !ok1 || !ok2 {
		// All kcp cloned infra machines should have this annotation.
		// Missing the annotation may be due to older version machines or adopted machines.
		// Should not be considered as mismatch.
		return "", true
	}

	// Check if the machine's infrastructure reference has been created from the current KCP infrastructure template.
	if clonedFromName != kcp.Spec.MachineTemplate.InfrastructureRef.Name ||
		clonedFromGroupKind != kcp.Spec.MachineTemplate.InfrastructureRef.GroupVersionKind().GroupKind().String() {
		return fmt.Sprintf("Infrastructure template on KCP rotated from %s %s to %s %s",
			clonedFromGroupKind, clonedFromName,
			kcp.Spec.MachineTemplate.InfrastructureRef.GroupVersionKind().GroupKind().String(), kcp.Spec.MachineTemplate.InfrastructureRef.Name), false
	}

	return "", true
}

// matchesKubeadmBootstrapConfig checks if machine's KubeadmConfigSpec is equivalent with KCP's KubeadmConfigSpec.
// Note: Differences to the labels and annotations on the KubeadmConfig are not considered for matching
// criteria, because changes to labels and annotations are propagated in-place to KubeadmConfig.
func matchesKubeadmBootstrapConfig(machineConfigs map[string]*bootstrapv1.KubeadmConfig, kcp *controlplanev1.KubeadmControlPlane, machine *clusterv1.Machine) (string, bool, error) {
	if machine == nil {
		return "Machine KubeadmConfig cannot be compared: Machine is nil", false, nil
	}

	// Check if KCP and machine ClusterConfiguration matches, if not return
	match, diff, err := matchClusterConfiguration(kcp, machine)
	if err != nil {
		return "", false, errors.Wrapf(err, "failed to match KubeadmConfig")
	}
	if !match {
		return fmt.Sprintf("Machine KubeadmConfig ClusterConfiguration is outdated: diff: %s", diff), false, nil
	}

	bootstrapRef := machine.Spec.Bootstrap.ConfigRef
	if bootstrapRef == nil {
		// Missing bootstrap reference should not be considered as unmatching.
		// This is a safety precaution to avoid selecting machines that are broken, which in the future should be remediated separately.
		return "", true, nil
	}

	machineConfig, found := machineConfigs[machine.Name]
	if !found {
		// Return true here because failing to get KubeadmConfig should not be considered as unmatching.
		// This is a safety precaution to avoid rolling out machines if the client or the api-server is misbehaving.
		return "", true, nil
	}

	// Check if KCP and machine InitConfiguration or JoinConfiguration matches
	// NOTE: only one between init configuration and join configuration is set on a machine, depending
	// on the fact that the machine was the initial control plane node or a joining control plane node.
	match, diff, err = matchInitOrJoinConfiguration(machineConfig, kcp)
	if err != nil {
		return "", false, errors.Wrapf(err, "failed to match KubeadmConfig")
	}
	if !match {
		return fmt.Sprintf("Machine KubeadmConfig InitConfiguration or JoinConfiguration are outdated: diff: %s", diff), false, nil
	}

	return "", true, nil
}

// matchClusterConfiguration verifies if KCP and machine ClusterConfiguration matches.
// NOTE: Machines that have KubeadmClusterConfigurationAnnotation will have to match with KCP ClusterConfiguration.
// If the annotation is not present (machine is either old or adopted), we won't roll out on any possible changes
// made in KCP's ClusterConfiguration given that we don't have enough information to make a decision.
// Users should use KCP.Spec.RolloutAfter field to force a rollout in this case.
func matchClusterConfiguration(kcp *controlplanev1.KubeadmControlPlane, machine *clusterv1.Machine) (bool, string, error) {
	machineClusterConfigStr, ok := machine.GetAnnotations()[controlplanev1.KubeadmClusterConfigurationAnnotation]
	if !ok {
		// We don't have enough information to make a decision; don't' trigger a roll out.
		return true, "", nil
	}

	machineClusterConfig := &bootstrapv1.ClusterConfiguration{}
	// ClusterConfiguration annotation is not correct, only solution is to rollout.
	// The call to json.Unmarshal has to take a pointer to the pointer struct defined above,
	// otherwise we won't be able to handle a nil ClusterConfiguration (that is serialized into "null").
	// See https://github.com/kubernetes-sigs/cluster-api/issues/3353.
	if err := json.Unmarshal([]byte(machineClusterConfigStr), &machineClusterConfig); err != nil {
		return false, "", nil //nolint:nilerr // Intentionally not returning the error here
	}

	// If any of the compared values are nil, treat them the same as an empty ClusterConfiguration.
	if machineClusterConfig == nil {
		machineClusterConfig = &bootstrapv1.ClusterConfiguration{}
	}

	kcpLocalClusterConfiguration := kcp.Spec.KubeadmConfigSpec.ClusterConfiguration
	if kcpLocalClusterConfiguration == nil {
		kcpLocalClusterConfiguration = &bootstrapv1.ClusterConfiguration{}
	}

	// Skip checking DNS fields because we can update the configuration of the working cluster in place.
	machineClusterConfig.DNS = kcpLocalClusterConfiguration.DNS

	// Compare and return.
	match, diff, err := compare.Diff(machineClusterConfig, kcpLocalClusterConfiguration)
	if err != nil {
		return false, "", errors.Wrapf(err, "failed to match ClusterConfiguration")
	}
	return match, diff, nil
}

// matchInitOrJoinConfiguration verifies if KCP and machine InitConfiguration or JoinConfiguration matches.
// NOTE: By extension this method takes care of detecting changes in other fields of the KubeadmConfig configuration (e.g. Files, Mounts etc.)
func matchInitOrJoinConfiguration(machineConfig *bootstrapv1.KubeadmConfig, kcp *controlplanev1.KubeadmControlPlane) (bool, string, error) {
	if machineConfig == nil {
		// Return true here because failing to get KubeadmConfig should not be considered as unmatching.
		// This is a safety precaution to avoid rolling out machines if the client or the api-server is misbehaving.
		return true, "", nil
	}

	// takes the KubeadmConfigSpec from KCP and applies the transformations required
	// to allow a comparison with the KubeadmConfig referenced from the machine.
	kcpConfig := getAdjustedKcpConfig(kcp, machineConfig)

	// Default both KubeadmConfigSpecs before comparison.
	// *Note* This assumes that newly added default values never
	// introduce a semantic difference to the unset value.
	// But that is something that is ensured by our API guarantees.
	kcpConfig.Default()
	machineConfig.Spec.Default()

	// cleanups all the fields that are not relevant for the comparison.
	cleanupConfigFields(kcpConfig, machineConfig)

	match, diff, err := compare.Diff(&machineConfig.Spec, kcpConfig)
	if err != nil {
		return false, "", errors.Wrapf(err, "failed to match InitConfiguration or JoinConfiguration")
	}
	return match, diff, nil
}

// getAdjustedKcpConfig takes the KubeadmConfigSpec from KCP and applies the transformations required
// to allow a comparison with the KubeadmConfig referenced from the machine.
// NOTE: The KCP controller applies a set of transformations when creating a KubeadmConfig referenced from the machine,
// mostly depending on the fact that the machine was the initial control plane node or a joining control plane node.
// In this function we don't have such information, so we are making the KubeadmConfigSpec similar to the KubeadmConfig.
func getAdjustedKcpConfig(kcp *controlplanev1.KubeadmControlPlane, machineConfig *bootstrapv1.KubeadmConfig) *bootstrapv1.KubeadmConfigSpec {
	kcpConfig := kcp.Spec.KubeadmConfigSpec.DeepCopy()

	// Machine's join configuration is nil when it is the first machine in the control plane.
	if machineConfig.Spec.JoinConfiguration == nil {
		kcpConfig.JoinConfiguration = nil
	}

	// Machine's init configuration is nil when the control plane is already initialized.
	if machineConfig.Spec.InitConfiguration == nil {
		kcpConfig.InitConfiguration = nil
	}

	return kcpConfig
}

// cleanupConfigFields cleanups all the fields that are not relevant for the comparison.
func cleanupConfigFields(kcpConfig *bootstrapv1.KubeadmConfigSpec, machineConfig *bootstrapv1.KubeadmConfig) {
	// KCP ClusterConfiguration will only be compared with a machine's ClusterConfiguration annotation, so
	// we are cleaning up from the reflect.DeepEqual comparison.
	kcpConfig.ClusterConfiguration = nil
	machineConfig.Spec.ClusterConfiguration = nil

	// If KCP JoinConfiguration is not present, set machine JoinConfiguration to nil (nothing can trigger rollout here).
	// NOTE: this is required because CABPK applies an empty joinConfiguration in case no one is provided.
	if kcpConfig.JoinConfiguration == nil {
		machineConfig.Spec.JoinConfiguration = nil
	}

	// Cleanup JoinConfiguration.Discovery from kcpConfig and machineConfig, because those info are relevant only for
	// the join process and not for comparing the configuration of the machine.
	emptyDiscovery := bootstrapv1.Discovery{}
	if kcpConfig.JoinConfiguration != nil {
		kcpConfig.JoinConfiguration.Discovery = emptyDiscovery
	}
	if machineConfig.Spec.JoinConfiguration != nil {
		machineConfig.Spec.JoinConfiguration.Discovery = emptyDiscovery
	}

	// If KCP JoinConfiguration.ControlPlane is not present, set machine join configuration to nil (nothing can trigger rollout here).
	// NOTE: this is required because CABPK applies an empty joinConfiguration.ControlPlane in case no one is provided.
	if kcpConfig.JoinConfiguration != nil && kcpConfig.JoinConfiguration.ControlPlane == nil &&
		machineConfig.Spec.JoinConfiguration != nil {
		machineConfig.Spec.JoinConfiguration.ControlPlane = nil
	}

	// If KCP's join NodeRegistration is empty, set machine's node registration to empty as no changes should trigger rollout.
	emptyNodeRegistration := bootstrapv1.NodeRegistrationOptions{}
	if kcpConfig.JoinConfiguration != nil && reflect.DeepEqual(kcpConfig.JoinConfiguration.NodeRegistration, emptyNodeRegistration) &&
		machineConfig.Spec.JoinConfiguration != nil {
		machineConfig.Spec.JoinConfiguration.NodeRegistration = emptyNodeRegistration
	}

	// Clear up the TypeMeta information from the comparison.
	// NOTE: KCP types don't carry this information.
	if machineConfig.Spec.InitConfiguration != nil && kcpConfig.InitConfiguration != nil {
		machineConfig.Spec.InitConfiguration.TypeMeta = kcpConfig.InitConfiguration.TypeMeta
	}
	if machineConfig.Spec.JoinConfiguration != nil && kcpConfig.JoinConfiguration != nil {
		machineConfig.Spec.JoinConfiguration.TypeMeta = kcpConfig.JoinConfiguration.TypeMeta
	}
}
