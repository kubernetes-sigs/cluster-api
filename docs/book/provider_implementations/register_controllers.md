
# Register Controllers

`pkg/controller/add_cluster_controller.go`

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

package controller

import (
        "sigs.k8s.io/cluster-api-provider-solas/pkg/cloud/solas/actuators/cluster"
        capicluster "sigs.k8s.io/cluster-api/pkg/controller/cluster"
        "sigs.k8s.io/controller-runtime/pkg/manager"
)

//+kubebuilder:rbac:groups=solas.k8s.io,resources=solasclusterproviderspecs;solasclusterproviderstatuses,verbs=get;list;watch;create;update;patch;delete
func init() {
        // AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
        AddToManagerFuncs = append(AddToManagerFuncs, func(m manager.Manager) error {
                actuator, err := cluster.NewActuator(cluster.ActuatorParams{})
                if err != nil {
                        return err
                }
                return capicluster.AddWithActuator(m, actuator)
        })
}
```

`pkg/controller/add_machine_controller.go`

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

package controller

import (
        "sigs.k8s.io/cluster-api-provider-solas/pkg/cloud/solas/actuators/machine"
        capimachine "sigs.k8s.io/cluster-api/pkg/controller/machine"
        "sigs.k8s.io/controller-runtime/pkg/manager"
)

//+kubebuilder:rbac:groups=solas.k8s.io,resources=solasmachineproviderspecs;solasmachineproviderstatuses,verbs=get;list;watch;create;update;patch;delete
func init() {
        // AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
        AddToManagerFuncs = append(AddToManagerFuncs, func(m manager.Manager) error {
                return capimachine.AddWithActuator(m, &machine.Actuator{})
        })
}
```
