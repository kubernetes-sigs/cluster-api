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

package collections

import (
	"time"

	"github.com/blang/semver/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// Func is the functon definition for a filter.
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

// HasControllerRef is a filter that returns true if the machine has a controller ref.
func HasControllerRef(machine *clusterv1.Machine) bool {
	if machine == nil {
		return false
	}
	return metav1.GetControllerOf(machine) != nil
}

// InFailureDomains returns a filter to find all machines
// in any of the given failure domains.
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

// OwnedMachines returns a filter to find all machines owned by specified owner.
// Usage: GetFilteredMachinesForCluster(ctx, client, cluster, OwnedMachines(controlPlane)).
func OwnedMachines(owner client.Object) func(machine *clusterv1.Machine) bool {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		return util.IsOwnedByObject(machine, owner)
	}
}

// ControlPlaneMachines returns a filter to find all control plane machines for a cluster, regardless of ownership.
// Usage: GetFilteredMachinesForCluster(ctx, client, cluster, ControlPlaneMachines(cluster.Name)).
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
// Usage: GetFilteredMachinesForCluster(ctx, client, cluster, AdoptableControlPlaneMachines(cluster.Name, controlPlane)).
func AdoptableControlPlaneMachines(clusterName string) func(machine *clusterv1.Machine) bool {
	return And(
		ControlPlaneMachines(clusterName),
		Not(HasControllerRef),
	)
}

// ActiveMachines returns a filter to find all active machines.
// Usage: GetFilteredMachinesForCluster(ctx, client, cluster, ActiveMachines).
func ActiveMachines(machine *clusterv1.Machine) bool {
	if machine == nil {
		return false
	}
	return machine.DeletionTimestamp.IsZero()
}

// HasDeletionTimestamp returns a filter to find all machines that have a deletion timestamp.
func HasDeletionTimestamp(machine *clusterv1.Machine) bool {
	if machine == nil {
		return false
	}
	return !machine.DeletionTimestamp.IsZero()
}

// HasUnhealthyCondition returns a filter to find all machines that have a MachineHealthCheckSucceeded condition set to False,
// indicating a problem was detected on the machine, and the MachineOwnerRemediated condition set, indicating that KCP is
// responsible of performing remediation as owner of the machine.
func HasUnhealthyCondition(machine *clusterv1.Machine) bool {
	if machine == nil {
		return false
	}
	return conditions.IsFalse(machine, clusterv1.MachineHealthCheckSucceededCondition) && conditions.IsFalse(machine, clusterv1.MachineOwnerRemediatedCondition)
}

// HasUnhealthyControlPlaneComponents returns a filter to find all unhealthy control plane machines that
// have any of the following control plane component conditions set to False:
// APIServerPodHealthy, ControllerManagerPodHealthy, SchedulerPodHealthy, EtcdPodHealthy & EtcdMemberHealthy (if using managed etcd).
// It is different from the HasUnhealthyCondition func which checks MachineHealthCheck conditions.
func HasUnhealthyControlPlaneComponents(isEtcdManaged bool) Func {
	controlPlaneMachineHealthConditions := []clusterv1.ConditionType{
		controlplanev1.MachineAPIServerPodHealthyCondition,
		controlplanev1.MachineControllerManagerPodHealthyCondition,
		controlplanev1.MachineSchedulerPodHealthyCondition,
	}
	if isEtcdManaged {
		controlPlaneMachineHealthConditions = append(controlPlaneMachineHealthConditions,
			controlplanev1.MachineEtcdPodHealthyCondition,
			controlplanev1.MachineEtcdMemberHealthyCondition,
		)
	}
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}

		// The machine without a node could be in failure status due to the kubelet config error, or still provisioning components (including etcd).
		// So do not treat it as unhealthy.

		for _, condition := range controlPlaneMachineHealthConditions {
			// Do not return true when the condition is not set or is set to Unknown because
			// it means a transient state and can not be considered as unhealthy.
			// preflightCheckCondition() can cover these two cases and skip the scaling up/down.
			if conditions.IsFalse(machine, condition) {
				return true
			}
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

// ShouldRolloutAfter returns a filter to find all machines where
// CreationTimestamp < rolloutAfter < reconciliationTIme.
func ShouldRolloutAfter(reconciliationTime, rolloutAfter *metav1.Time) Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		if reconciliationTime == nil || rolloutAfter == nil {
			return false
		}
		return machine.CreationTimestamp.Before(rolloutAfter) && rolloutAfter.Before(reconciliationTime)
	}
}

// ShouldRolloutBefore returns a filter to find all machine whose
// certificates will expire within the specified days.
func ShouldRolloutBefore(reconciliationTime *metav1.Time, rolloutBefore *controlplanev1.RolloutBefore) Func {
	return func(machine *clusterv1.Machine) bool {
		if rolloutBefore == nil || rolloutBefore.CertificatesExpiryDays == nil {
			return false
		}
		if machine == nil || machine.Status.CertificatesExpiryDate == nil {
			return false
		}
		certsExpiryTime := machine.Status.CertificatesExpiryDate.Time
		return reconciliationTime.Add(time.Duration(*rolloutBefore.CertificatesExpiryDays) * 24 * time.Hour).After(certsExpiryTime)
	}
}

// HasAnnotationKey returns a filter to find all machines that have the
// specified Annotation key present.
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
		must(labels.NewRequirement(clusterv1.ClusterNameLabel, selection.Equals, []string{clusterName})),
		must(labels.NewRequirement(clusterv1.MachineControlPlaneLabel, selection.Exists, []string{})),
	)
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

// WithVersion returns a filter to find machine that have a non empty and valid version.
func WithVersion() Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		if machine.Spec.Version == nil {
			return false
		}
		if _, err := semver.ParseTolerant(*machine.Spec.Version); err != nil {
			return false
		}
		return true
	}
}

// HealthyAPIServer returns a filter to find all machines that have a MachineAPIServerPodHealthyCondition
// set to true.
func HealthyAPIServer() Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		return conditions.IsTrue(machine, controlplanev1.MachineAPIServerPodHealthyCondition)
	}
}

// HasNode returns a filter to find all machines that have a corresponding Kubernetes node.
func HasNode() Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		return machine.Status.NodeRef != nil
	}
}
