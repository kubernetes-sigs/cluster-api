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

/*
Package tree supports the generation of an "at glance" view of a Cluster API cluster designed to help the user in quickly
understanding if there are problems and where.

The "at glance" view is based on the idea that we should avoid to overload the user with information, but instead
surface problems, if any; in practice:

- The view assumes we are processing objects conforming with https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20200506-conditions.md.
  As a consequence each object should have a Ready condition summarizing the object state.

- The view organizes objects in a hierarchical tree, however it is not required that the
  tree reflects the ownerReference tree so it is possible to skip objects not relevant for triaging the cluster status
  e.g. secrets or templates.

- It is possible to add "meta names" to object, thus making hierarchical tree more consistent for the users,
  e.g. use MachineInfrastructure instead of using all the different infrastructure machine kinds (AWSMachine, VSphereMachine etc.).

- It is possible to add "virtual nodes", thus allowing to make the hierarchical tree more meaningful for the users,
  e.g. adding a Workers object to group all the MachineDeployments.

- It is possible to "group" siblings objects by ready condition e.g. group all the machines with Ready=true
  in a single node instead of listing each one of them.

- Given that the ready condition of the child object bubbles up to the parents, it is possible to avoid the "echo"
  (reporting the same condition at the parent/child) e.g. if a machine's Ready condition is already
  surface an error from the infrastructure machine, let's avoid to show the InfrastructureMachine
  given that representing its state is redundant in this case.

- In order to avoid long list of objects (think e.g. a cluster with 50 worker machines), sibling objects with the
  same value for the ready condition can be grouped together into a virtual node, e.g. 10 Machines ready

The ObjectTree object defined implements all the above behaviors of the "at glance" visualization, by generating
a tree of Kubernetes objects; each object gets a set of annotation, reflecting its own visualization specific attributes,
e.g is virtual node, is group node, meta name etc.

The Discovery object uses the ObjectTree to build the "at glance" view of a Cluster API.
*/
package tree
