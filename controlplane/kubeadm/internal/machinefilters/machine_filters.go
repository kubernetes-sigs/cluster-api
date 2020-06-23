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

package machinefilters

import (
	"context"
	"encoding/json"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Func func(machine *clusterv1.Machine) bool

// And returns a filter that returns true if all of the given filters returns true.
func And(filters ...Func) Func {
	return func(machine *clusterv1.Machine) bool {
		for _, f := range filters {
			if !f(machine) {
				return false
			}
		}
		return true
	}
}

// Or returns a filter that returns true if any of the given filters returns true.
func Or(filters ...Func) Func {
	return func(machine *clusterv1.Machine) bool {
		for _, f := range filters {
			if f(machine) {
				return true
			}
		}
		return false
	}
}

// Not returns a filter that returns the opposite of the given filter.
func Not(mf Func) Func {
	return func(machine *clusterv1.Machine) bool {
		return !mf(machine)
	}
}

// HasControllerRef is a filter that returns true if the machine has a controller ref
func HasControllerRef(machine *clusterv1.Machine) bool {
	if machine == nil {
		return false
	}
	return metav1.GetControllerOf(machine) != nil
}

// InFailureDomains returns a filter to find all machines
// in any of the given failure domains
func InFailureDomains(failureDomains ...*string) Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		for i := range failureDomains {
			fd := failureDomains[i]
			if fd == nil {
				if fd == machine.Spec.FailureDomain {
					return true
				}
				continue
			}
			if machine.Spec.FailureDomain == nil {
				continue
			}
			if *fd == *machine.Spec.FailureDomain {
				return true
			}
		}
		return false
	}
}

// OwnedMachines returns a filter to find all owned control plane machines.
// Usage: managementCluster.GetMachinesForCluster(ctx, cluster, machinefilters.OwnedMachines(controlPlane))
func OwnedMachines(owner controllerutil.Object) func(machine *clusterv1.Machine) bool {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		return util.IsOwnedByObject(machine, owner)
	}
}

// ControlPlaneMachines returns a filter to find all control plane machines for a cluster, regardless of ownership.
// Usage: managementCluster.GetMachinesForCluster(ctx, cluster, machinefilters.ControlPlaneMachines(cluster.Name))
func ControlPlaneMachines(clusterName string) func(machine *clusterv1.Machine) bool {
	selector := ControlPlaneSelectorForCluster(clusterName)
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		return selector.Matches(labels.Set(machine.Labels))
	}
}

// AdoptableControlPlaneMachines returns a filter to find all un-controlled control plane machines.
// Usage: managementCluster.GetMachinesForCluster(ctx, cluster, AdoptableControlPlaneMachines(cluster.Name, controlPlane))
func AdoptableControlPlaneMachines(clusterName string) func(machine *clusterv1.Machine) bool {
	return And(
		ControlPlaneMachines(clusterName),
		Not(HasControllerRef),
	)
}

// HasDeletionTimestamp returns a filter to find all machines that have a deletion timestamp.
func HasDeletionTimestamp(machine *clusterv1.Machine) bool {
	if machine == nil {
		return false
	}
	return !machine.DeletionTimestamp.IsZero()
}

// MatchesConfigurationHash returns a filter to find all machines
// that match a given KubeadmControlPlane configuration hash.
func MatchesConfigurationHash(configHash string) Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		if hash, ok := machine.Labels[controlplanev1.KubeadmControlPlaneHashLabelKey]; ok {
			return hash == configHash
		}
		return false
	}
}

// IsReady returns a filter to find all machines with the ReadyCondition equals to True.
func IsReady() Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		return conditions.IsTrue(machine, clusterv1.ReadyCondition)
	}
}

// OlderThan returns a filter to find all machines
// that have a CreationTimestamp earlier than the given time.
func OlderThan(t *metav1.Time) Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		return machine.CreationTimestamp.Before(t)
	}
}

// HasAnnotationKey returns a filter to find all machines that have the
// specified Annotation key present
func HasAnnotationKey(key string) Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil || machine.Annotations == nil {
			return false
		}
		if _, ok := machine.Annotations[key]; ok {
			return true
		}
		return false
	}
}

// ControlPlaneSelectorForCluster returns the label selector necessary to get control plane machines for a given cluster.
func ControlPlaneSelectorForCluster(clusterName string) labels.Selector {
	must := func(r *labels.Requirement, err error) labels.Requirement {
		if err != nil {
			panic(err)
		}
		return *r
	}
	return labels.NewSelector().Add(
		must(labels.NewRequirement(clusterv1.ClusterLabelName, selection.Equals, []string{clusterName})),
		must(labels.NewRequirement(clusterv1.MachineControlPlaneLabelName, selection.Exists, []string{})),
	)
}

// MatchesKCPConfiguration returns a filter to find all machines that matches with KCP config and do not require any rollout.
// Kubernetes version, infrastructure template, and KubeadmConfig field need to be equivalent.
func MatchesKCPConfiguration(ctx context.Context, c client.Client, kcp controlplanev1.KubeadmControlPlane, cluster clusterv1.Cluster) func(machine *clusterv1.Machine) bool {
	return And(
		MatchesKubernetesVersion(kcp.Spec.Version),
		MatchesKubeadmBootstrapConfig(ctx, c, kcp, cluster),
		MatchesTemplateClonedFrom(ctx, c, kcp),
	)
}

// MatchesTemplateClonedFrom returns a filter to find all machines that match a given KCP infra template.
func MatchesTemplateClonedFrom(ctx context.Context, c client.Client, kcp controlplanev1.KubeadmControlPlane) Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		infraObj, err := external.Get(ctx, c, &machine.Spec.InfrastructureRef, kcp.Namespace)
		// Return true here because failing to get infrastructure machine should not be considered as unmatching.
		if err != nil {
			return true
		}

		clonedFromName, ok1 := infraObj.GetAnnotations()[clusterv1.TemplateClonedFromNameAnnotation]
		clonedFromGroupKind, ok2 := infraObj.GetAnnotations()[clusterv1.TemplateClonedFromGroupKindAnnotation]
		if !ok1 || !ok2 {
			// All kcp cloned infra machines should have this annotation.
			// Missing the annotation may be due to older version machines or adopted machines.
			// Should not be considered as mismatch.
			return true
		}

		// Check if the machine's infrastructure reference has been created from the current KCP infrastructure template.
		if clonedFromName != kcp.Spec.InfrastructureTemplate.Name ||
			clonedFromGroupKind != kcp.Spec.InfrastructureTemplate.GroupVersionKind().GroupKind().String() {
			return false
		}
		return true
	}
}

// MatchesKubernetesVersion returns a filter to find all machines that match a given Kubernetes version.
func MatchesKubernetesVersion(kubernetesVersion string) Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		if machine.Spec.Version == nil {
			return false
		}
		return *machine.Spec.Version == kubernetesVersion
	}
}

// MatchesKubeadmBootstrapConfig checks if machine's KubeadmConfigSpec is equivalent with KCP's KubeadmConfigSpec.
func MatchesKubeadmBootstrapConfig(ctx context.Context, c client.Client, kcp controlplanev1.KubeadmControlPlane, cluster clusterv1.Cluster) Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		bootstrapRef := machine.Spec.Bootstrap.ConfigRef
		if bootstrapRef == nil {
			// Missing bootstrap reference should not be considered as unmatching.
			// This is a safety precaution to avoid selecting machines that are broken, which in the future should be remediated separately.
			return true
		}

		bootstrapObj := &bootstrapv1.KubeadmConfig{}
		if err := c.Get(ctx, client.ObjectKey{Name: bootstrapRef.Name, Namespace: machine.Namespace}, bootstrapObj); err != nil {
			// Return true here because failing to get KubeadmConfig should not be considered as unmatching.
			// This is a safety precaution to avoid rolling out machines if the client or the api-server is misbehaving.
			return true
		}

		kcpConfigSpecLocal := kcp.Spec.KubeadmConfigSpec.DeepCopy()

		// Machine's init configuration is nil when machine is the control plane is already initialized.
		if bootstrapObj.Spec.InitConfiguration == nil {
			kcpConfigSpecLocal.InitConfiguration = nil
		}

		// Machine's join configuration is nil when a machine is the first machine in the control plane.
		if bootstrapObj.Spec.JoinConfiguration == nil {
			kcpConfigSpecLocal.JoinConfiguration = nil
		} else if kcpConfigSpecLocal.JoinConfiguration == nil {
			// If KCP join configuration is not present, set machine join configuration to nil (nothing can trigger rollout here).
			bootstrapObj.Spec.JoinConfiguration = nil
		}

		// Clear up the TypeMeta information from the comparison. Today, the kubeadm types are embedded in CABPK and KCP types don't carry this information.
		if bootstrapObj.Spec.InitConfiguration != nil && kcpConfigSpecLocal.InitConfiguration != nil {
			bootstrapObj.Spec.InitConfiguration.TypeMeta = kcpConfigSpecLocal.InitConfiguration.TypeMeta
		}
		if bootstrapObj.Spec.JoinConfiguration != nil && kcpConfigSpecLocal.JoinConfiguration != nil {
			bootstrapObj.Spec.JoinConfiguration.TypeMeta = kcpConfigSpecLocal.JoinConfiguration.TypeMeta
		}

		// Machines that have KubeadmClusterConfigurationAnnotation will have to match with KCP ClusterConfiguration.
		// If the annotation is not present (machine is either old or adopted), we won't roll out on any possible changes
		// made in KCP's ClusterConfiguration given that we don't have enough information to make a decision.
		// Users should use KCP.Spec.UpgradeAfter field to force a rollout in this case.
		machineClusterConfigStr, ok := machine.GetAnnotations()[controlplanev1.KubeadmClusterConfigurationAnnotation]
		if ok {
			machineClusterConfig := &kubeadmv1.ClusterConfiguration{}
			// ClusterConfiguration annotation is not correct, only solution is to rollout.
			if err := json.Unmarshal([]byte(machineClusterConfigStr), machineClusterConfig); err != nil {
				return false
			}
			if !reflect.DeepEqual(machineClusterConfig, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration) {
				return false
			}
		}

		// KCP ClusterConfiguration will only be compared with a machine's ClusterConfiguration annotation.
		// To skip ClusterConfiguration merge during SemanticMerge(), set both Machine's and KCP's ClusterConfigurations to nil here.
		kcpConfigSpecLocal.ClusterConfiguration = nil
		bootstrapObj.Spec.ClusterConfiguration = nil

		// Machine's JoinConfiguration Discovery is set to an empty object by KubeadmConfig controller if KCP is Discovery is nil, also
		// adopted machines may have an unrelated discovery setting so ignore discovery completely when comparing JoinConfigurations.
		emptyDiscovery := kubeadmv1.Discovery{}
		if kcpConfigSpecLocal.JoinConfiguration != nil {
			kcpConfigSpecLocal.JoinConfiguration.Discovery = emptyDiscovery
		}
		if bootstrapObj.Spec.JoinConfiguration != nil {
			bootstrapObj.Spec.JoinConfiguration.Discovery = emptyDiscovery
		}

		// Machine's JoinConfiguration ControlPlane is set to an empty object by KubeadmConfig controller if KCP is ControlPlane is nil.
		// Set Machine's ControlPlane to nil to avoid
		if kcpConfigSpecLocal.JoinConfiguration != nil && kcpConfigSpecLocal.JoinConfiguration.ControlPlane == nil {
			bootstrapObj.Spec.JoinConfiguration.ControlPlane = nil
		}

		// If KCP's join NodeRegistration is empty, set machine's node registration to empty as no changes should trigger rollout.
		emptyNodeRegistration := kubeadmv1.NodeRegistrationOptions{}
		if kcpConfigSpecLocal.JoinConfiguration != nil && reflect.DeepEqual(kcpConfigSpecLocal.JoinConfiguration.NodeRegistration, emptyNodeRegistration) {
			bootstrapObj.Spec.JoinConfiguration.NodeRegistration = emptyNodeRegistration
		}

		return reflect.DeepEqual(bootstrapObj.Spec, *kcpConfigSpecLocal)
	}
}
