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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
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

// OwnedControlPlaneMachines rerturns a filter to find all owned control plane machines.
// Usage: managementCluster.GetMachinesForCluster(ctx, cluster, machinefilters.OwnedControlPlaneMachines(controlPlane.Name))
func OwnedControlPlaneMachines(controlPlaneName string) Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		controllerRef := metav1.GetControllerOf(machine)
		if controllerRef == nil {
			return false
		}
		return controllerRef.Kind == "KubeadmControlPlane" && controllerRef.Name == controlPlaneName
	}
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

// IsProvisioning returns whether the machine is missing its NodeRef or does
// not have InfrastructureReady set to true.
func IsProvisioning(m *clusterv1.Machine) bool {
	return m.Status.NodeRef == nil || !m.Status.InfrastructureReady
}

// IsFailed returns whether the machine has a FailureMessage or a FailureReason.
func IsFailed(m *clusterv1.Machine) bool {
	return m.Status.FailureMessage != nil || m.Status.FailureReason != nil
}

// IsUnhealthy returns whether the machine has the MachineHealthCheck unhealthy
// annotation.
func IsUnhealthy(m *clusterv1.Machine) bool {
	return HasAnnotationKey(clusterv1.MachineUnhealthy)(m)
}
