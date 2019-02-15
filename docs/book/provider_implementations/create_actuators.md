# Creating Actuators

In this section we will create the actuator stubs needed to create the
Cluster API controllers.

## Cluster Actuator

The following actuator stub code should be copied to
`cluster-api-provider-solas/pkg/cloud/solas/actuators/cluster/actuator.go`.

```go
/*
Copyright 2018 The Kubernetes authors.

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

package cluster

import (
        "fmt"
        "log"

        clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
        client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

// Add RBAC rules to access cluster-api resources
//+kubebuilder:rbac:groups=cluster.k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

// Actuator is responsible for performing cluster reconciliation
type Actuator struct {
        clustersGetter client.ClustersGetter
}

// ActuatorParams holds parameter information for Actuator
type ActuatorParams struct {
        ClustersGetter client.ClustersGetter
}

// NewActuator creates a new Actuator
func NewActuator(params ActuatorParams) (*Actuator, error) {
        return &Actuator{
                clustersGetter: params.ClustersGetter,
        }, nil
}

// Reconcile reconciles a cluster and is invoked by the Cluster Controller
func (a *Actuator) Reconcile(cluster *clusterv1.Cluster) error {
        log.Printf("Reconciling cluster %v.", cluster.Name)
        return fmt.Errorf("TODO: Not yet implemented")
}

// Delete deletes a cluster and is invoked by the Cluster Controller
func (a *Actuator) Delete(cluster *clusterv1.Cluster) error {
        log.Printf("Deleting cluster %v.", cluster.Name)
        return fmt.Errorf("TODO: Not yet implemented")
}
```

## Machine Actuator

The following actuator stub code should be copied to
`cluster-api-provider-solas/pkg/cloud/solas/actuators/machine/actuator.go`.

```go
/*
Copyright 2018 The Kubernetes authors.

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
        "log"

        clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
        client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

const (
        ProviderName = "solas"
)

// Add RBAC rules to access cluster-api resources
//+kubebuilder:rbac:groups=cluster.k8s.io,resources=machines;machines/status;machinedeployments;machinedeployments/status;machinesets;machinesets/status;machineclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=nodes;events,verbs=get;list;watch;create;update;patch;delete

// Actuator is responsible for performing machine reconciliation
type Actuator struct {
        machinesGetter client.MachinesGetter
}

// ActuatorParams holds parameter information for Actuator
type ActuatorParams struct {
        MachinesGetter client.MachinesGetter
}

// NewActuator creates a new Actuator
func NewActuator(params ActuatorParams) (*Actuator, error) {
        return &Actuator{
                machinesGetter: params.MachinesGetter,
        }, nil
}

// Create creates a machine and is invoked by the Machine Controller
func (a *Actuator) Create(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
        log.Printf("Creating machine %v for cluster %v.", machine.Name, cluster.Name)
        return fmt.Errorf("TODO: Not yet implemented")
}

// Delete deletes a machine and is invoked by the Machine Controller
func (a *Actuator) Delete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
        log.Printf("Deleting machine %v for cluster %v.", machine.Name, cluster.Name)
        return fmt.Errorf("TODO: Not yet implemented")
}

// Update updates a machine and is invoked by the Machine Controller
func (a *Actuator) Update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
        log.Printf("Updating machine %v for cluster %v.", machine.Name, cluster.Name)
        return fmt.Errorf("TODO: Not yet implemented")
}

// Exists tests for the existence of a machine and is invoked by the Machine Controller
func (a *Actuator) Exists(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
        log.Printf("Checking if machine %v for cluster %v exists.", machine.Name, cluster.Name)
        return false, fmt.Errorf("TODO: Not yet implemented")
}

// The Machine Actuator interface must implement GetIP and GetKubeConfig functions as a workaround for issues
// cluster-api#158 (https://github.com/kubernetes-sigs/cluster-api/issues/158) and cluster-api#160
// (https://github.com/kubernetes-sigs/cluster-api/issues/160).

// GetIP returns IP address of the machine in the cluster.
func (a *Actuator) GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {
        log.Printf("Getting IP of machine %v for cluster %v.", machine.Name, cluster.Name)
        return "", fmt.Errorf("TODO: Not yet implemented")
}

// GetKubeConfig gets a kubeconfig from the running control plane.
func (a *Actuator) GetKubeConfig(cluster *clusterv1.Cluster, controlPlaneMachine *clusterv1.Machine) (string, error) {
        log.Printf("Getting IP of machine %v for cluster %v.", controlPlaneMachine.Name, cluster.Name)
        return "", fmt.Errorf("TODO: Not yet implemented")
}
```
