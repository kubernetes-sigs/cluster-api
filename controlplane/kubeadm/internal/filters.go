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
	"context"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/defaulting"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/desiredstate"
	"sigs.k8s.io/cluster-api/internal/util/compare"
	"sigs.k8s.io/cluster-api/util/collections"
)

// UpToDateResult is the result of calling the UpToDate func for a Machine.
type UpToDateResult struct {
	LogMessages              []string
	ConditionMessages        []string
	EligibleForInPlaceUpdate bool
	DesiredMachine           *clusterv1.Machine
	CurrentInfraMachine      *unstructured.Unstructured
	DesiredInfraMachine      *unstructured.Unstructured
	CurrentKubeadmConfig     *bootstrapv1.KubeadmConfig
	DesiredKubeadmConfig     *bootstrapv1.KubeadmConfig
}

// UpToDate checks if a Machine is up to date with the control plane's configuration.
// If not, messages explaining why are provided with different level of detail for logs and conditions.
func UpToDate(
	ctx context.Context,
	c client.Client,
	cluster *clusterv1.Cluster,
	machine *clusterv1.Machine,
	kcp *controlplanev1.KubeadmControlPlane,
	reconciliationTime *metav1.Time,
	infraMachines map[string]*unstructured.Unstructured,
	kubeadmConfigs map[string]*bootstrapv1.KubeadmConfig,
) (bool, *UpToDateResult, error) {
	res := &UpToDateResult{
		// A Machine is eligible for in-place update except if we find a reason why it shouldn't be,
		// e.g. rollout.before, rollout.after or the Machine is already up-to-date.
		EligibleForInPlaceUpdate: true,
	}

	// If a Machine is marked for deletion it is not eligible for in-place update.
	if _, ok := machine.Annotations[clusterv1.DeleteMachineAnnotation]; ok {
		res.EligibleForInPlaceUpdate = false
	}

	// If a Machine is marked for remediation it is not eligible for in-place update.
	if _, ok := machine.Annotations[clusterv1.RemediateMachineAnnotation]; ok {
		res.EligibleForInPlaceUpdate = false
	}

	// Machines whose certificates are about to expire.
	if collections.ShouldRolloutBefore(reconciliationTime, kcp.Spec.Rollout.Before)(machine) {
		res.LogMessages = append(res.LogMessages, "certificates will expire soon, rolloutBefore expired")
		res.ConditionMessages = append(res.ConditionMessages, "Certificates will expire soon")
		res.EligibleForInPlaceUpdate = false
	}

	// Machines that are scheduled for rollout (KCP.Spec.RolloutAfter set,
	// the RolloutAfter deadline is expired, and the machine was created before the deadline).
	if collections.ShouldRolloutAfter(reconciliationTime, kcp.Spec.Rollout.After)(machine) {
		res.LogMessages = append(res.LogMessages, "rolloutAfter expired")
		res.ConditionMessages = append(res.ConditionMessages, "KubeadmControlPlane spec.rolloutAfter expired")
		res.EligibleForInPlaceUpdate = false
	}

	// Machines that do not match with KCP config.
	// Note: matchesMachineSpec will update res with desired and current objects if necessary.
	matches, specLogMessages, specConditionMessages, err := matchesMachineSpec(ctx, c, infraMachines, kubeadmConfigs, kcp, cluster, machine, res)
	if err != nil {
		return false, nil, errors.Wrapf(err, "failed to determine if Machine %s is up-to-date", machine.Name)
	}
	if !matches {
		res.LogMessages = append(res.LogMessages, specLogMessages...)
		res.ConditionMessages = append(res.ConditionMessages, specConditionMessages...)
	}

	if len(res.LogMessages) > 0 || len(res.ConditionMessages) > 0 {
		return false, res, nil
	}

	// If everything matches no need for an update.
	res.EligibleForInPlaceUpdate = false
	return true, res, nil
}

// matchesMachineSpec checks if a Machine matches any of a set of KubeadmConfigs and a set of infra machine configs.
// If it doesn't, it returns the reasons why.
// Kubernetes version, infrastructure template, and KubeadmConfig field need to be equivalent.
// Note: We don't need to compare the entire MachineSpec to determine if a Machine needs to be rolled out,
// because all the fields in the MachineSpec, except for version, the infrastructureRef and bootstrap.ConfigRef, are either:
// - mutated in-place (ex: NodeDrainTimeoutSeconds)
// - are not dictated by KCP (ex: ProviderID)
// - are not relevant for the rollout decision (ex: failureDomain).
func matchesMachineSpec(
	ctx context.Context,
	c client.Client,
	infraMachines map[string]*unstructured.Unstructured,
	kubeadmConfigs map[string]*bootstrapv1.KubeadmConfig,
	kcp *controlplanev1.KubeadmControlPlane,
	cluster *clusterv1.Cluster,
	machine *clusterv1.Machine,
	res *UpToDateResult,
) (bool, []string, []string, error) {
	logMessages := []string{}
	conditionMessages := []string{}

	desiredMachine, err := desiredstate.ComputeDesiredMachine(kcp, cluster, machine.Spec.FailureDomain, machine)
	if err != nil {
		return false, nil, nil, errors.Wrapf(err, "failed to match Machine")
	}
	// Note: spec.version is not mutated in-place by syncMachines and accordingly
	// not updated by desiredstate.ComputeDesiredMachine, so we have to update it here.
	desiredMachine.Spec.Version = kcp.Spec.Version
	// Note: spec.failureDomain is in general only changed on delete/create, so we don't have to update it here for in-place.
	res.DesiredMachine = desiredMachine
	// Note: Intentionally not storing currentMachine as it can change later, e.g. through syncMachines.
	if desiredMachine.Spec.Version != machine.Spec.Version {
		logMessages = append(logMessages, fmt.Sprintf("Machine version %q is not equal to KCP version %q", machine.Spec.Version, desiredMachine.Spec.Version))
		// Note: the code computing the message for KCP's RolloutOut condition is making assumptions on the format/content of this message.
		conditionMessages = append(conditionMessages, fmt.Sprintf("Version %s, %s required", machine.Spec.Version, desiredMachine.Spec.Version))
	}

	reason, currentKubeadmConfig, desiredKubeadmConfig, matches, err := matchesKubeadmConfig(kubeadmConfigs, kcp, cluster, machine)
	if err != nil {
		return false, nil, nil, errors.Wrapf(err, "failed to match Machine")
	}
	res.CurrentKubeadmConfig = currentKubeadmConfig
	res.DesiredKubeadmConfig = desiredKubeadmConfig
	if !matches {
		logMessages = append(logMessages, reason)
		conditionMessages = append(conditionMessages, "KubeadmConfig is not up-to-date")
	}

	reason, currentInfraMachine, desiredInfraMachine, matches, err := matchesInfraMachine(ctx, c, infraMachines, kcp, cluster, machine)
	if err != nil {
		return false, nil, nil, errors.Wrapf(err, "failed to match Machine")
	}
	res.CurrentInfraMachine = currentInfraMachine
	res.DesiredInfraMachine = desiredInfraMachine
	if !matches {
		logMessages = append(logMessages, reason)
		conditionMessages = append(conditionMessages, fmt.Sprintf("%s is not up-to-date", machine.Spec.InfrastructureRef.Kind))
	}

	if len(logMessages) > 0 || len(conditionMessages) > 0 {
		return false, logMessages, conditionMessages, nil
	}

	return true, nil, nil, nil
}

// matchesInfraMachine checks if a Machine has a corresponding infrastructure machine that
// matches a given KCP infra template and if it doesn't match returns the reason why.
// Note: Differences to the labels and annotations on the infrastructure machine are not considered for matching
// criteria, because changes to labels and annotations are propagated in-place to the infrastructure machines.
func matchesInfraMachine(
	ctx context.Context,
	c client.Client,
	infraMachines map[string]*unstructured.Unstructured,
	kcp *controlplanev1.KubeadmControlPlane,
	cluster *clusterv1.Cluster,
	machine *clusterv1.Machine,
) (string, *unstructured.Unstructured, *unstructured.Unstructured, bool, error) {
	currentInfraMachine, found := infraMachines[machine.Name]
	if !found {
		// Return true here because failing to get infrastructure machine should not be considered as unmatching.
		return "", nil, nil, true, nil
	}

	clonedFromName, ok1 := currentInfraMachine.GetAnnotations()[clusterv1.TemplateClonedFromNameAnnotation]
	clonedFromGroupKind, ok2 := currentInfraMachine.GetAnnotations()[clusterv1.TemplateClonedFromGroupKindAnnotation]
	if !ok1 || !ok2 {
		// All kcp cloned infra machines should have this annotation.
		// Missing the annotation may be due to older version machines or adopted machines.
		// Should not be considered as mismatch.
		return "", nil, nil, true, nil
	}

	desiredInfraMachine, err := desiredstate.ComputeDesiredInfraMachine(ctx, c, kcp, cluster, machine.Name, currentInfraMachine)
	if err != nil {
		return "", nil, nil, false, errors.Wrapf(err, "failed to match %s", currentInfraMachine.GetKind())
	}

	// Check if the machine's infrastructure reference has been created from the current KCP infrastructure template.
	if clonedFromName != kcp.Spec.MachineTemplate.Spec.InfrastructureRef.Name ||
		clonedFromGroupKind != kcp.Spec.MachineTemplate.Spec.InfrastructureRef.GroupKind().String() {
		return fmt.Sprintf("Infrastructure template on KCP rotated from %s %s to %s %s",
			clonedFromGroupKind, clonedFromName,
			kcp.Spec.MachineTemplate.Spec.InfrastructureRef.GroupKind().String(), kcp.Spec.MachineTemplate.Spec.InfrastructureRef.Name), currentInfraMachine, desiredInfraMachine, false, nil
	}

	return "", currentInfraMachine, desiredInfraMachine, true, nil
}

// matchesKubeadmConfig checks if machine's KubeadmConfigSpec is equivalent with KCP's KubeadmConfigSpec.
// Note: Differences to the labels and annotations on the KubeadmConfig are not considered for matching
// criteria, because changes to labels and annotations are propagated in-place to KubeadmConfig.
func matchesKubeadmConfig(
	kubeadmConfigs map[string]*bootstrapv1.KubeadmConfig,
	kcp *controlplanev1.KubeadmControlPlane,
	cluster *clusterv1.Cluster,
	machine *clusterv1.Machine,
) (string, *bootstrapv1.KubeadmConfig, *bootstrapv1.KubeadmConfig, bool, error) {
	bootstrapRef := machine.Spec.Bootstrap.ConfigRef
	if !bootstrapRef.IsDefined() {
		// Missing bootstrap reference should not be considered as unmatching.
		// This is a safety precaution to avoid selecting machines that are broken, which in the future should be remediated separately.
		return "", nil, nil, true, nil
	}

	currentKubeadmConfig, found := kubeadmConfigs[machine.Name]
	if !found {
		// Return true here because failing to get KubeadmConfig should not be considered as unmatching.
		// This is a safety precaution to avoid rolling out machines if the client or the api-server is misbehaving.
		return "", nil, nil, true, nil
	}

	// Note: Compute a KubeadmConfig assuming we are dealing with a joining machine, which is the most common scenario.
	// When dealing with the KubeadmConfig for the init machine, the code will make a first tentative comparison under
	// the assumption that KCP initConfiguration and joinConfiguration should be configured identically.
	// In order to do so, PrepareKubeadmConfigsForDiff will attempt to convert initConfiguration to
	// joinConfiguration in currentKubeadmConfig.
	desiredKubeadmConfigWithJoin, err := desiredstate.ComputeDesiredKubeadmConfig(kcp, cluster, true, machine.Name, currentKubeadmConfig)
	if err != nil {
		return "", nil, nil, false, errors.Wrapf(err, "failed to match KubeadmConfig")
	}
	desiredKubeadmConfigWithJoinForDiff, currentKubeadmConfigWithJoinForDiff := PrepareKubeadmConfigsForDiff(desiredKubeadmConfigWithJoin, currentKubeadmConfig, true)

	// Check if current and desired KubeadmConfigs match.
	// Note: desiredKubeadmConfigWithJoinForDiff has been computed for a kubeadm join.
	// Note: currentKubeadmConfigWithJoinForDiff has been migrated from init to join, if currentKubeadmConfig was for a kubeadm init.
	match, diff, err := compare.Diff(&currentKubeadmConfigWithJoinForDiff.Spec, &desiredKubeadmConfigWithJoinForDiff.Spec)
	if err != nil {
		return "", nil, nil, false, errors.Wrapf(err, "failed to match KubeadmConfig")
	}
	if !match {
		// Note: KCP initConfiguration and joinConfiguration should be configured identically.
		// But if they are not configured identically and the currentKubeadmConfig is for init we still
		// want to avoid unnecessary rollouts.
		// Accordingly, we also have to compare the currentKubeadmConfig against a desiredKubeadmConfig
		// computed for init to avoid unnecessary rollouts.
		// Note: In any case we are going to use desiredKubeadmConfigWithJoin for in-place updates and return it accordingly.
		if isKubeadmConfigForInit(currentKubeadmConfig) {
			desiredKubeadmConfigWithInit, err := desiredstate.ComputeDesiredKubeadmConfig(kcp, cluster, false, machine.Name, currentKubeadmConfig)
			if err != nil {
				return "", nil, nil, false, errors.Wrapf(err, "failed to match KubeadmConfig")
			}
			desiredKubeadmConfigWithInitForDiff, currentKubeadmConfigWithInitForDiff := PrepareKubeadmConfigsForDiff(desiredKubeadmConfigWithInit, currentKubeadmConfig, false)

			// Check if current and desired KubeadmConfigs match.
			// Note: desiredKubeadmConfigWithInitForDiff has been computed for a kubeadm init.
			// Note: currentKubeadmConfigWithInitForDiff is for a kubeadm init.
			match, diff, err := compare.Diff(&currentKubeadmConfigWithInitForDiff.Spec, &desiredKubeadmConfigWithInitForDiff.Spec)
			if err != nil {
				return "", nil, nil, false, errors.Wrapf(err, "failed to match KubeadmConfig")
			}
			// Always return desiredKubeadmConfigWithJoin (not desiredKubeadmConfigWithInit) as it should always be used for in-place updates.
			if !match {
				return fmt.Sprintf("Machine KubeadmConfig is outdated: diff: %s", diff), currentKubeadmConfig, desiredKubeadmConfigWithJoin, false, nil
			}
			return "", currentKubeadmConfig, desiredKubeadmConfigWithJoin, true, nil
		}

		return fmt.Sprintf("Machine KubeadmConfig is outdated: diff: %s", diff), currentKubeadmConfig, desiredKubeadmConfigWithJoin, false, nil
	}

	return "", currentKubeadmConfig, desiredKubeadmConfigWithJoin, true, nil
}

// PrepareKubeadmConfigsForDiff cleans up all fields that are not relevant for the comparison.
func PrepareKubeadmConfigsForDiff(desiredKubeadmConfig, currentKubeadmConfig *bootstrapv1.KubeadmConfig, convertCurrentInitConfigurationToJoinConfiguration bool) (desired, current *bootstrapv1.KubeadmConfig) {
	// DeepCopy to ensure the passed in KubeadmConfigs are not modified.
	// This has to be done because we eventually want to be able to apply the desiredKubeadmConfig
	// (without the modifications that we make here).
	desiredKubeadmConfig = desiredKubeadmConfig.DeepCopy()
	currentKubeadmConfig = currentKubeadmConfig.DeepCopy()

	if convertCurrentInitConfigurationToJoinConfiguration && isKubeadmConfigForInit(currentKubeadmConfig) {
		// Convert InitConfiguration to JoinConfiguration
		currentKubeadmConfig.Spec.JoinConfiguration.Timeouts = currentKubeadmConfig.Spec.InitConfiguration.Timeouts
		currentKubeadmConfig.Spec.JoinConfiguration.Patches = currentKubeadmConfig.Spec.InitConfiguration.Patches
		currentKubeadmConfig.Spec.JoinConfiguration.SkipPhases = currentKubeadmConfig.Spec.InitConfiguration.SkipPhases
		currentKubeadmConfig.Spec.JoinConfiguration.NodeRegistration = currentKubeadmConfig.Spec.InitConfiguration.NodeRegistration
		if currentKubeadmConfig.Spec.InitConfiguration.LocalAPIEndpoint.IsDefined() {
			currentKubeadmConfig.Spec.JoinConfiguration.ControlPlane = &bootstrapv1.JoinControlPlane{
				LocalAPIEndpoint: currentKubeadmConfig.Spec.InitConfiguration.LocalAPIEndpoint,
			}
		}
		currentKubeadmConfig.Spec.InitConfiguration = bootstrapv1.InitConfiguration{}

		// CACertPath can only be configured for join.
		// The CACertPath should never trigger a rollout of Machines created via kubeadm init.
		currentKubeadmConfig.Spec.JoinConfiguration.CACertPath = desiredKubeadmConfig.Spec.JoinConfiguration.CACertPath
	}

	// Ignore ControlPlaneEndpoint which is added on the Machine KubeadmConfig by CABPK.
	// Note: ControlPlaneEndpoint should also never change for a Cluster, so no reason to trigger a rollout because of that.
	currentKubeadmConfig.Spec.ClusterConfiguration.ControlPlaneEndpoint = desiredKubeadmConfig.Spec.ClusterConfiguration.ControlPlaneEndpoint

	// Skip checking DNS fields because we can update the configuration of the working cluster in place.
	currentKubeadmConfig.Spec.ClusterConfiguration.DNS = desiredKubeadmConfig.Spec.ClusterConfiguration.DNS

	// Default both KubeadmConfigSpecs before comparison.
	// *Note* This assumes that newly added default values never
	// introduce a semantic difference to the unset value.
	// But that is something that is ensured by our API guarantees.
	defaulting.ApplyPreviousKubeadmConfigDefaults(&desiredKubeadmConfig.Spec)
	defaulting.ApplyPreviousKubeadmConfigDefaults(&currentKubeadmConfig.Spec)

	// Cleanup JoinConfiguration.Discovery from desiredKubeadmConfig and currentKubeadmConfig, because those info are relevant only for
	// the join process and not for comparing the configuration of the machine.
	// Note: Changes to Discovery will apply for the next join, but they will not lead to a rollout.
	// Note: We should also not send Discovery.BootstrapToken.Token to a RuntimeExtension for security reasons.
	desiredKubeadmConfig.Spec.JoinConfiguration.Discovery = bootstrapv1.Discovery{}
	currentKubeadmConfig.Spec.JoinConfiguration.Discovery = bootstrapv1.Discovery{}

	// Cleanup ControlPlaneComponentHealthCheckSeconds from desiredKubeadmConfig and currentKubeadmConfig,
	// because through conversion apiServer.timeoutForControlPlane in v1beta1 is converted to
	// initConfiguration/joinConfiguration.timeouts.controlPlaneComponentHealthCheckSeconds in v1beta2 and
	// this can lead to a diff here that would lead to a rollout.
	// Note: Changes to ControlPlaneComponentHealthCheckSeconds will apply for the next join, but they will not lead to a rollout.
	desiredKubeadmConfig.Spec.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = nil
	desiredKubeadmConfig.Spec.JoinConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = nil
	currentKubeadmConfig.Spec.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = nil
	currentKubeadmConfig.Spec.JoinConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = nil

	// If KCP JoinConfiguration.ControlPlane is nil and the Machine JoinConfiguration.ControlPlane is empty,
	// set Machine JoinConfiguration.ControlPlane to nil.
	// NOTE: This is required because CABPK applies an empty JoinConfiguration.ControlPlane in case it is nil.
	if desiredKubeadmConfig.Spec.JoinConfiguration.ControlPlane == nil &&
		reflect.DeepEqual(currentKubeadmConfig.Spec.JoinConfiguration.ControlPlane, &bootstrapv1.JoinControlPlane{}) {
		currentKubeadmConfig.Spec.JoinConfiguration.ControlPlane = nil
	}

	// Drop differences that do not lead to changes to Machines, but that might exist due
	// to changes in how we serialize objects or how webhooks work.
	dropOmittableFields(&desiredKubeadmConfig.Spec)
	dropOmittableFields(&currentKubeadmConfig.Spec)

	return desiredKubeadmConfig, currentKubeadmConfig
}

// dropOmittableFields makes the comparison tolerant to omittable fields being set in the go struct. It applies to:
// - empty array vs nil
// - empty map vs nil
// - empty struct vs nil (when struct is pointer and there are only omittable fields in the struct).
// Note: for the part of the KubeadmConfigSpec that is rendered using go templates, consideration might be a little bit different.
func dropOmittableFields(spec *bootstrapv1.KubeadmConfigSpec) {
	// When rendered to kubeadm config files there is no diff between nil and empty array or map.

	if len(spec.ClusterConfiguration.Etcd.Local.ExtraArgs) == 0 {
		spec.ClusterConfiguration.Etcd.Local.ExtraArgs = nil
	}
	if spec.ClusterConfiguration.Etcd.Local.ExtraEnvs != nil &&
		len(*spec.ClusterConfiguration.Etcd.Local.ExtraEnvs) == 0 {
		spec.ClusterConfiguration.Etcd.Local.ExtraEnvs = nil
	}
	if len(spec.ClusterConfiguration.Etcd.Local.ServerCertSANs) == 0 {
		spec.ClusterConfiguration.Etcd.Local.ServerCertSANs = nil
	}
	if len(spec.ClusterConfiguration.Etcd.Local.PeerCertSANs) == 0 {
		spec.ClusterConfiguration.Etcd.Local.PeerCertSANs = nil
	}
	// NOTE: we are not dropping spec.ClusterConfiguration.Etcd.ExternalEtcd.Endpoints because this field
	// doesn't have omitempty, so [] array is different from nil when serialized.
	// But this field is also required and has MinItems=1, so it will
	// never actually be nil or an empty array so that difference also won't trigger any rollouts.
	if len(spec.ClusterConfiguration.APIServer.ExtraArgs) == 0 {
		spec.ClusterConfiguration.APIServer.ExtraArgs = nil
	}
	if spec.ClusterConfiguration.APIServer.ExtraEnvs != nil &&
		len(*spec.ClusterConfiguration.APIServer.ExtraEnvs) == 0 {
		spec.ClusterConfiguration.APIServer.ExtraEnvs = nil
	}
	if len(spec.ClusterConfiguration.APIServer.ExtraVolumes) == 0 {
		spec.ClusterConfiguration.APIServer.ExtraVolumes = nil
	}
	if len(spec.ClusterConfiguration.APIServer.CertSANs) == 0 {
		spec.ClusterConfiguration.APIServer.CertSANs = nil
	}
	if len(spec.ClusterConfiguration.ControllerManager.ExtraArgs) == 0 {
		spec.ClusterConfiguration.ControllerManager.ExtraArgs = nil
	}
	if spec.ClusterConfiguration.ControllerManager.ExtraEnvs != nil &&
		len(*spec.ClusterConfiguration.ControllerManager.ExtraEnvs) == 0 {
		spec.ClusterConfiguration.ControllerManager.ExtraEnvs = nil
	}
	if len(spec.ClusterConfiguration.ControllerManager.ExtraVolumes) == 0 {
		spec.ClusterConfiguration.ControllerManager.ExtraVolumes = nil
	}
	if len(spec.ClusterConfiguration.Scheduler.ExtraArgs) == 0 {
		spec.ClusterConfiguration.Scheduler.ExtraArgs = nil
	}
	if spec.ClusterConfiguration.Scheduler.ExtraEnvs != nil &&
		len(*spec.ClusterConfiguration.Scheduler.ExtraEnvs) == 0 {
		spec.ClusterConfiguration.Scheduler.ExtraEnvs = nil
	}
	if len(spec.ClusterConfiguration.Scheduler.ExtraVolumes) == 0 {
		spec.ClusterConfiguration.Scheduler.ExtraVolumes = nil
	}
	if len(spec.ClusterConfiguration.FeatureGates) == 0 {
		spec.ClusterConfiguration.FeatureGates = nil
	}

	if len(spec.InitConfiguration.BootstrapTokens) == 0 {
		spec.InitConfiguration.BootstrapTokens = nil
	}
	for i, token := range spec.InitConfiguration.BootstrapTokens {
		if len(token.Usages) == 0 {
			token.Usages = nil
		}
		if len(token.Groups) == 0 {
			token.Groups = nil
		}
		spec.InitConfiguration.BootstrapTokens[i] = token
	}
	if len(spec.InitConfiguration.NodeRegistration.KubeletExtraArgs) == 0 {
		spec.InitConfiguration.NodeRegistration.KubeletExtraArgs = nil
	}
	if len(spec.InitConfiguration.NodeRegistration.IgnorePreflightErrors) == 0 {
		spec.InitConfiguration.NodeRegistration.IgnorePreflightErrors = nil
	}
	if len(spec.InitConfiguration.SkipPhases) == 0 {
		spec.InitConfiguration.SkipPhases = nil
	}
	// NOTE: we are not dropping spec.InitConfiguration.Taints because for this field there
	// is a difference between not set (use kubeadm defaults) and empty (do not apply any taint).

	if len(spec.JoinConfiguration.Discovery.BootstrapToken.CACertHashes) == 0 {
		spec.JoinConfiguration.Discovery.BootstrapToken.CACertHashes = nil
	}
	if len(spec.JoinConfiguration.Discovery.File.KubeConfig.Cluster.CertificateAuthorityData) == 0 {
		spec.JoinConfiguration.Discovery.File.KubeConfig.Cluster.CertificateAuthorityData = nil
	}
	if len(spec.JoinConfiguration.Discovery.File.KubeConfig.User.AuthProvider.Config) == 0 {
		spec.JoinConfiguration.Discovery.File.KubeConfig.User.AuthProvider.Config = nil
	}
	if len(spec.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.Args) == 0 {
		spec.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.Args = nil
	}
	if len(spec.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.Env) == 0 {
		spec.JoinConfiguration.Discovery.File.KubeConfig.User.Exec.Env = nil
	}
	if len(spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs) == 0 {
		spec.JoinConfiguration.NodeRegistration.KubeletExtraArgs = nil
	}
	if len(spec.JoinConfiguration.NodeRegistration.IgnorePreflightErrors) == 0 {
		spec.JoinConfiguration.NodeRegistration.IgnorePreflightErrors = nil
	}
	// NOTE: we are not dropping spec.JoinConfiguration.Taints because for this field there
	// is a difference between not set (use kubeadm defaults) and empty (do not apply any taint).
	if len(spec.JoinConfiguration.SkipPhases) == 0 {
		spec.JoinConfiguration.SkipPhases = nil
	}

	// When rendered to cloud init, there is no diff between nil and empty files.
	if len(spec.Files) == 0 {
		spec.Files = nil
	}

	// When rendered to cloud init, there is no diff between nil and empty diskSetup.filesystems.
	// When rendered to cloud init, there is no diff between nil and empty diskSetup.filesystems[].extraOpts.
	// When rendered to cloud init, there is no diff between nil and empty diskSetup.partitions.
	if len(spec.DiskSetup.Filesystems) == 0 {
		spec.DiskSetup.Filesystems = nil
	}
	for i, fs := range spec.DiskSetup.Filesystems {
		if len(fs.ExtraOpts) == 0 {
			fs.ExtraOpts = nil
		}
		spec.DiskSetup.Filesystems[i] = fs
	}
	if len(spec.DiskSetup.Partitions) == 0 {
		spec.DiskSetup.Partitions = nil
	}

	// When rendered to cloud init, there is no diff between nil and empty Mounts.
	if len(spec.Mounts) == 0 {
		spec.Mounts = nil
	}

	// When rendered to cloud init, there is no diff between nil and empty BootCommands.
	if len(spec.BootCommands) == 0 {
		spec.BootCommands = nil
	}

	// When rendered to cloud init, there is no diff between nil and empty PreKubeadmCommands.
	if len(spec.PreKubeadmCommands) == 0 {
		spec.PreKubeadmCommands = nil
	}

	// When rendered to cloud init, there is no diff between nil and empty PostKubeadmCommands.
	if len(spec.PostKubeadmCommands) == 0 {
		spec.PostKubeadmCommands = nil
	}

	// When rendered to cloud init, there is no diff between nil and empty Users.
	// When rendered to cloud init, there is no diff between nil and empty Users[].SSHAuthorizedKeys.
	if len(spec.Users) == 0 {
		spec.Users = nil
	}
	for i, user := range spec.Users {
		if len(user.SSHAuthorizedKeys) == 0 {
			user.SSHAuthorizedKeys = nil
		}
		spec.Users[i] = user
	}

	// When rendered to cloud init, there is no diff between nil and empty ntp.servers.
	if len(spec.NTP.Servers) == 0 {
		spec.NTP.Servers = nil
	}
}

// isKubeadmConfigForJoin returns true if the KubeadmConfig is for a control plane
// or a worker machine that joined an existing cluster.
// Note: This check is based on the assumption that KubeadmConfig for joining
// control plane and workers nodes always have a non-empty JoinConfiguration.Discovery, while
// instead the JoinConfiguration for the first control plane machine in the
// cluster is emptied out by KCP.
// Note: Previously we checked if the entire JoinConfiguration is defined, but that
// is not safe because apiServer.timeoutForControlPlane in v1beta1 is also converted to
// joinConfiguration.timeouts.controlPlaneComponentHealthCheckSeconds in v1beta2 and
// accordingly we would also detect init KubeadmConfigs as join.
func isKubeadmConfigForJoin(c *bootstrapv1.KubeadmConfig) bool {
	return c.Spec.JoinConfiguration.Discovery.IsDefined()
}

// isKubeadmConfigForInit returns true if the KubeadmConfig is for the first control plane
// machine in the cluster.
func isKubeadmConfigForInit(c *bootstrapv1.KubeadmConfig) bool {
	return !isKubeadmConfigForJoin(c)
}
