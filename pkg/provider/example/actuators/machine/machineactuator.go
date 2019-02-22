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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

// Actuator is responsible for performing machine reconciliation.
type Actuator struct {
	client   clusterv1alpha1.ClusterV1alpha1Interface
	recorder record.EventRecorder
}

// NewMachineActuator return a machine actuator
func NewMachineActuator(clusterV1alpha1 clusterv1alpha1.ClusterV1alpha1Interface, recorder record.EventRecorder) (*Actuator, error) {
	return &Actuator{
		client:   clusterV1alpha1,
		recorder: recorder,
	}, nil
}

// Create creates a machine
func (a *Actuator) Create(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	a.recorder.Event(machine, corev1.EventTypeWarning, "cluster-api", "machineactuator Create invoked")
	return nil
}

// Delete deletes a machine
func (a *Actuator) Delete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	a.recorder.Event(machine, corev1.EventTypeWarning, "cluster-api", "machineactuator Delete invoked")
	return nil
}

// Update updates a machine
func (a *Actuator) Update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	a.recorder.Event(machine, corev1.EventTypeWarning, "cluster-api", "machineactuator Update invoked")
	return nil
}

// Exists test for the existence of a machine
func (a *Actuator) Exists(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	a.recorder.Event(machine, corev1.EventTypeWarning, "cluster-api", "machineactuator Exists invoked")
	return false, nil
}
