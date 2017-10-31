package cloud

import (
	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
)

// Controls machines on a specific cloud.
type MachineActuator interface {
	Create(*machinesv1.Machine) error
	Delete(*machinesv1.Machine) error
	Get(string) (*machinesv1.Machine, error)
}