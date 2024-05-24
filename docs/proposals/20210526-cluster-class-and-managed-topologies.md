---
title: ClusterClass and managed topologies
authors:
  - "@srm09"
  - "@vincepri"
  - "@fabriziopandini"
  - "@CecileRobertMichon"
  - "@sbueringer"
reviewers:
  - "@vincepri"
  - "@fabriziopandini"
  - "@CecileRobertMichon"
  - "@enxebre"
  - "@schrej"
  - "@randomvariable"
creation-date: 2021-05-26
replaces: https://docs.google.com/document/d/1lwxgBK3Q7zmNkOSFqzTGmrSys_vinkwubwgoyqSRAbI
status: provisional
---

# ClusterClass and Managed Topologies

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Glossary](#glossary)
  - [ClusterClass](#clusterclass)
  - [Topology](#topology)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Prospective future Work](#prospective-future-work)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
    - [Story 1 - Use ClusterClass to easily stamp Clusters](#story-1---use-clusterclass-to-easily-stamp-clusters)
    - [Story 2 - Easier UX for Kubernetes version upgrades](#story-2---easier-ux-for-kubernetes-version-upgrades)
    - [Story 3 - Easier UX for scaling workers nodes](#story-3---easier-ux-for-scaling-workers-nodes)
    - [Story 4 - Use ClusterClass to easily modify Clusters in bulk](#story-4---use-clusterclass-to-easily-modify-clusters-in-bulk)
    - [Story 5 - Ability to define ClusterClass customizations](#story-5---ability-to-define-clusterclass-customizations)
    - [Story 6 - Ability to customize individual Clusters via variables](#story-6---ability-to-customize-individual-clusters-via-variables)
    - [Story 7 - Ability to mutate variables](#story-7---ability-to-mutate-variables)
    - [Story 8 - Easy UX for MachineHealthChecks](#story-8---easy-ux-for-machinehealthchecks)
  - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [New API types](#new-api-types)
      - [ClusterClass](#clusterclass-1)
    - [Modification to existing API Types](#modification-to-existing-api-types)
      - [Cluster](#cluster)
    - [Validation and Defaulting](#validation-and-defaulting)
    - [Basic behaviors](#basic-behaviors)
      - [Create a new Cluster using ClusterClass object](#create-a-new-cluster-using-clusterclass-object)
      - [Update an existing Cluster using ClusterClass](#update-an-existing-cluster-using-clusterclass)
    - [Behavior with patches](#behavior-with-patches)
      - [Create a new ClusterClass with patches](#create-a-new-clusterclass-with-patches)
      - [Create a new Cluster with patches](#create-a-new-cluster-with-patches)
    - [Provider implementation](#provider-implementation)
    - [Conventions for template types implementation](#conventions-for-template-types-implementation)
    - [Notes on template <-> object reconciliation](#notes-on-template---object-reconciliation)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
- [Upgrade Strategy](#upgrade-strategy)
- [Additional Details](#additional-details)
  - [Test Plan [optional]](#test-plan-optional)
  - [Graduation Criteria [optional]](#graduation-criteria-optional)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Glossary

### ClusterClass
A collection of templates that define a topology (control plane, machine deployments and machine pools) to be used to continuously reconcile one or more Clusters.

### Topology
A topology refers to a Cluster that provides a single control point to manage its own topology; the topology is defined by a ClusterClass.

## Summary

This proposal introduces a new ClusterClass object which will be used to provide easy stamping of clusters of similar shapes. It serves as a collection of template resources which are used to generate one or more clusters of the same flavor.

We're enhancing the Cluster CRD and controller to use a ClusterClass resource to provision the underlying objects that compose a cluster. Additionally, when using a ClusterClass, the Cluster provides a single control point to manage the Kubernetes version, worker pools, labels, replicas, and so on.

## Motivation

Currently, Cluster API does not expose a native way to provision multiple clusters of the same configuration. The ClusterClass object is supposed to act as a collection of template references which can be used to create managed topologies.

Today, the Cluster object is a logical grouping of components which describe an underlying cluster. The user experience to create a cluster requires the user to create a bunch of underlying resources such as KCP (control plane provider), MachineDeployments, MachinePools and infrastructure or bootstrap templates for those resources which logically end up representing the cluster. Since the cluster configuration is spread around multiple components, upgrading the cluster version is hard as it requires changes to different fields in different resources to perform an upgrade. The ClusterClass object aims at reducing this complexity by delegating the responsibility of lifecycle managing these underlying resources to the Cluster controller.

This method of provisioning the cluster would act as a single control point for the entire cluster. Scaling the nodes, adding/removing sets of worker nodes and upgrading cluster kubernetes versions would be achievable by editing the topology. This would facilitate the maintenance of existing clusters as well as ease the creation of newer clusters.

### Goals

- Create the new ClusterClass CRD which can serve as a collection of templates to create clusters.
- Extend the Cluster object to use ClusterClass for creating managed topologies.
- Enhance the Cluster object to act as a single point of control for the topology.
- Extend the Cluster controller to create/update/delete managed topologies (this includes continuous reconciliation of the topology managed resources).
- Introduce mechanisms to allow Cluster-specific customizations of a ClusterClass 

### Prospective future Work

⚠️ The following points are mostly ideas and can change at any given time  ⚠️

We are fully aware that in order to exploit the potential of ClusterClass and managed topologies, the following class of problems still needs to be addressed:
- **Upgrade/rollback strategy**: Implement a strategy to upgrade and rollback the managed topologies.
- **Adoption**: Providing a way to convert existing clusters into managed topologies.
- **Observability**: Build an SDK and enhance the Cluster object status to surface a summary of the status of the topology.
- **Lifecycle integrations**: Extend ClusterClass to include lifecycle management integrations such as Cluster Autoscaler to manage the state of the managed topologies.

However we are intentionally leaving them out from this initial iteration for the following reasons:
- We want the community to reach a consensus on cornerstone elements of the design before iterating on additional features.
- We want to enable starting the implementation of the required scaffolding and the initial support for managed topologies as soon as possible, so we can surface problems which are not easy to identify at this stage of the proposal.
- We would like the community to rally in defining use cases for the advanced features, help in prioritizing them, so we can chart a more effective roadmap for the next steps.

## Proposal

This proposal enhances the `Cluster` object to create topologies using the `ClusterClass` object.

### User Stories

#### Story 1 - Use ClusterClass to easily stamp Clusters
As a cluster operator, I want to use one `ClusterClass` to create multiple topologies of similar flavor.
- Rather than recreating the KCP and MD objects for every cluster that needs to be provisioned, the cluster operator can create a template once and reuse it to create multiple Clusters with similar configurations.

#### Story 2 - Easier UX for Kubernetes version upgrades
For a cluster operator, the UX to update the Kubernetes version of the control plane and worker nodes in the cluster should be easy.
- Instead of individually modifying the KCP and each MachineDeployment or MachinePool, updating a single option should result in k8s version updates for all the CP and worker nodes.

**Note**: In order to complete the user story for all the providers, some of the advanced features (such as Extensibility/Transformation) are required. However, getting this in place even only for a subset of providers allows us to build and test a big chunk of the entire machinery.

#### Story 3 - Easier UX for scaling workers nodes
As a cluster operator, I want to be able to easily scale up/down the number of replicas for each set of worker nodes in the cluster.
- Currently, (for a cluster with 3 machine deployments) this is possible by updating these three different objects representing the sets of worker nodes in the pool. An easier user experience would be to update a single object to enable the scaling of multiple sets of worker nodes.

#### Story 4 - Use ClusterClass to easily modify Clusters in bulk
As a cluster operator, I want to be able to easily change the configuration of all Clusters of a ClusterClass. For example, I want to be able to change the kube-apiserver 
command line flags (e.g. via `KubeadmControlPlane`) in the ClusterClass and this change should be rolled out to all Clusters of the ClusterClass. The same should be possible 
for all fields of all templates referenced in a ClusterClass.

**Notes**:
- Only compatible changes (as specified in [ClusterClass compatibility](#clusterclass-compatibility)) should be allowed.
- Changes to InfrastructureMachineTemplates and BootstrapTemplates should be rolled out according to the established operational practices documented in 
[Updating Machine Infrastructure and Bootstrap Templates](https://cluster-api.sigs.k8s.io/tasks/updating-machine-templates.html), i.e. "template rotation".
- There are provider-specific incompatible changes which cannot be validated in a "core" webhook, e.g. changing an immutable field of `KubeadmControlPlane`. Those changes 
  will inevitably lead to errors during topology reconciliation. Those errors should be surfaced on the Cluster resource.

#### Story 5 - Ability to define ClusterClass customizations
As a ClusterClass author (e.g. an infrastructure provider author), I want to be able to write a ClusterClass which covers a wide range of use cases. To make this possible, 
I want to make the ClusterClass customizable, i.e. depending on configuration provided during Cluster creation, the managed topology should have a different shape.

**Note**: Without this feature all Clusters of the same ClusterClass would be the same apart from the properties that are already configure via the topology,
like Kubernetes version, labels and annotations. This would limit the number of variants a single ClusterClass could address, i.e. separate ClusterClasses would be 
required for deviations which cannot be achieved via the `Cluster.spec.topology` fields. 

**Example**: The ClusterAPI provider AWS project wants to provide a ClusterClass which cluster operators can then use to deploy a Cluster in a specific AWS region, 
which they can configure on the Cluster resource.

#### Story 6 - Ability to customize individual Clusters via variables
As a cluster operator, I want to customize individual Clusters simply by providing variables in the Cluster resource.

**Example**: A cluster operator wants to deploy CAPA Clusters using ClusterClass in different AWS regions. One option to achieve this is to duplicate the ClusterClass and its referenced templates.
The better option is to introduce a variable and a corresponding patch in the ClusterClass. Now, a user can simply set the AWS region via a variable in the Cluster spec, instead of 
having to duplicate the entire ClusterClass just to set a different region in the AWSCluster resource.

#### Story 7 - Ability to mutate variables
As a cluster operator, I want to be able to mutate variables in a Cluster, which should lead to a rollout of affected resources of the managed topology.

**Example**: Given a ClusterClass which exposes the `controlPlaneMachineType` variable to make the control plane machine type configurable, i.e. different Clusters using the same ClusterClass 
can use different machine types. A cluster operator initially chooses a `controlPlaneMachineType` on Cluster creation. Over time the Cluster grows and thus also the resource requirements 
of the control plane machines as the Kubernetes control plane components require more CPU and memory. The cluster operator now scales the control plane machines vertically by mutating 
the `controlPlaneMachineType` variable accordingly.

**Notes**: Same notes as in Story 4 apply.

#### Story 8 - Easy UX for MachineHealthChecks
As a cluster operator I want a simple way to define checks to manage the health of the machines in my cluster. 

Instead of defining MachineHealthChecks each time a Cluster is created, there should be a mechanism for creating the same type of health check for each Cluster stamped by a ClusterClass.
 
### Implementation Details/Notes/Constraints

The following section provides details about the introduction of new types and modifications to existing types to implement the ClusterClass functionality.
If instead you are eager to see an example of ClusterClass and how the Cluster object will look, you can jump to the Behavior paragraph.

#### New API types

##### ClusterClass

The ClusterClass CRD allows to define a collection of templates that describe the topology for one or more clusters.

The detailed definition of this type can be found at [ClusterClass CRD reference](https://doc.crds.dev/github.com/kubernetes-sigs/cluster-api/cluster.x-k8s.io/ClusterClass/v1beta1);
at high level the new CRD contains:

- The reference to the InfrastructureCluster template (e.g. AWSClusterTemplate) to be used when creating a Cluster using this ClusterClass
- The reference to the ControlPlane template (e.g. KubeadmControlPlaneTemplate) to be used when creating a Cluster using this ClusterClass along with:
  - The reference to infrastructureMachine template (e.g. AWSMachineTemplate) to be used when creating machines for the cluster's control plane.
  - Additional attributes to be set when creating the control plane object, like metadata, nodeDrainTimeout, etc.
  - The definition of a MachineHealthCheck to be created for monitoring control plane's machines.
- The definition of how workers machines should look like in a Cluster using this ClusterClass, being composed of:
  - A set of MachineDeploymentClasses, each one with: 
    - The reference to the bootstrap template (e.g. KubeadmConfigTemplate) to be used when creating machine deployment machines.
    - The reference to the infrastructureMachine template (e.g. AWSMachineTemplate) to be used when creating machine deployment machines.
    - Additional attributes to be set when creating the machine deployment object, like metadata, nodeDrainTimeout, rolloutStrategy etc.
    - The definition of a MachineHealthCheck to be created for monitoring machine deployment machines.
  - And/or a set of MachinePoolClasses, each one with:
    - The reference to the bootstrap template (e.g. KubeadmConfigTemplate) to be used when creating machine pools.
    - The reference to the infrastructureMachinePool template (e.g. DockerMachinePoolTemplate) to be used when creating machine pools.
    - Additional attributes to be set when creating the machine pool object, like metadata, nodeDrainTimeout, etc.
- A list of patches, allowing to change above templates for each specific Cluster.
- A list of variable definitions, defining a set of additional values the users can provide on each specific cluster;
  those values can be used in patches. 

The following paragraph provides some additional context on some of the above values; more info can
be found in [writing a ClusterClass](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/write-clusterclass.html).

**ClusterClass variable definitions**

Each variable can be defined by providing its own OpenAPI schema definition. The OpenAPI schema used is inspired from the [schema](https://github.com/kubernetes/apiextensions-apiserver/blob/master/pkg/apis/apiextensions/types_jsonschema.go) used in Custom Resource Definitions in Kubernetes.

To keep the implementation as easy and user-friendly as possible variable definition in ClusterClass is restricted to:
- Basic types: boolean, integer, number, string
- Complex types: objects, maps and arrays
- Basic validation, e.g. format, minimum, maximum, pattern, required, etc.
- Defaulting
    - Defaulting will be implemented based on the CRD structural schema library and thus will have the same feature set 
      as CRD defaulting. I.e., it will only be possible to use constant values as defaults.
  
Note: if you are using clusterctl templating for creating ClusterClass, it will be possible to to inject default values
from environment variables at creation time.

**ClusterClass Patches**

There are two ways to define patches, by providing inline JSON patches in the ClusterClass or by referencing external patches as defined in
 [Topology Mutation Hook proposal](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220330-topology-mutation-hook.md).

However, it's important to notice that  while defining patches, the author can reference both variable values
provided in the Cluster spec (see next paragraph for more details) as well as a set of [built in variables](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/write-clusterclass.html#builtin-variables)
providing generic information about the cluster or the template being patched. 

#### Modification to existing API Types

##### Cluster

The [Cluster CRD](https://doc.crds.dev/github.com/kubernetes-sigs/cluster-api/cluster.x-k8s.io/Cluster/v1beta1) has been extended 
with a new field allowing to define and control the cluster topology from a single point.

At high level the cluster topology is defined by:

- A link to a Cluster Class
- The Kubernetes version to be used for the Cluster (both CP and workers).
- The definition of the Cluster's control plane attributes, including the number of replicas as
  well as overrides/additional values for control plane metadata, nodeDrainTimeout etc. 
  Additionally it is also possible to override the control plane's MachineHealthCheck.
- The list of machine deployments to be created, each one defined by:
  - The link to the MachineDeployment class defining the templates to use for this MachineDeployment
  - The number of replicas for this MachineDeployment as well as overrides/additional values for metadata, nodeDrainTimeout etc.
    Additionally it is also possible to override the control plane's MachineHealthCheck.
- The above also applies for machine pools.
- A set of variables allowing to customize the cluster topology through patches. Please note that it is also possible
  to define variable overrides for each MachineDeployment or MachinePool.

More info in [writing a ClusterClass](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/write-clusterclass.html).

#### Validation and Defaulting

Both the new field in the Cluster CRD and the new ClusterClass CRD type will be validated according to what is specified in the
API definitions; additionally please consider the following:

**ClusterClass**

- It is not allowed to change apiGroup or Kind for the referenced templates (with the only exception of the bootstrap templates).
- MachineDeploymentClass and MachinePoolClass cannot be removed as long as they are used in Clusters.
- It’s the responsibility of the ClusterClass author to ensure the patches are semantically valid, in the sense they
  generate valid templates for all the combinations of the corresponding variables in input.
- Variables cannot be removed as long as they are used in Clusters.
- When changing variable definitions, the system validates schema changes against existing clusters and blocks in case the changes are  
  not compatible (the variable value is not compatible with the new variable definition).

Note: we are considering adding a field to allow smoother deprecations of MachineDeploymentClass, MachinePoolClass and/or variables, but this
is not yet implemented as of today.

**Cluster**

- Variables are defaulted according to the corresponding variable definitions in the ClusterClass. After defaulting is applied, values
  can be changed by the user only (they are not affected by change of the default value in the ClusterClass).
- All required variables must exist and match the schema defined in the corresponding variable definition in the ClusterClass.
- When changing the cluster class in use by a cluster, the validation ensures that the  new ClusterClass is compatible, i.e. the operation cannot change apiGroup or Kind
  for the referenced templates (with the only exception of the bootstrap templates).

#### Basic behaviors

This section lists out the basic behavior for Cluster objects using a ClusterClass in case of creates and updates. The following examples 
intentionally use resources without patches and variables to focus on the simplest case.

More info in [writing a ClusterClass](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/write-clusterclass.html)
as well as in
- [changing a ClusterClass](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/change-clusterclass.html)
- [operating a managed Cluster](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/operate-cluster.html)

##### Create a new Cluster using ClusterClass object

1. User creates a ClusterClass object.

   ```yaml
    apiVersion: cluster.x-k8s.io/v1beta1
    kind: ClusterClass
    metadata:
      name: mixed
      namespace: bar
    spec:
      controlPlane:
        ref:
          apiVersion: controlplane.cluster.x-k8s.io/v1beta1
          kind: KubeadmControlPlaneTemplate
          name: vsphere-prod-cluster-template-kcp
        machineInfrastructure:
          ref:
            apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
            kind: VSphereMachineTemplate
            name: linux-vsphere-template
        # This will create a MachineHealthCheck for ControlPlane machines.
        machineHealthCheck:
          nodeStartupTimeout: 3m
          maxUnhealthy: 33%
          unhealthyConditions:
            - type: Ready
              status: Unknown
              timeout: 300s
            - type: Ready
              status: "False"
              timeout: 300s
      workers:
        machineDeployments:
        - class: linux-worker
          template:
            bootstrap:
              ref:
                apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
                kind: KubeadmConfigTemplate
                name: existing-boot-ref
            infrastructure:
              ref:
                apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
                kind: VSphereMachineTemplate
                name: linux-vsphere-template
          # This will create a health check for each deployment created with the "linux-worker" MachineDeploymentClass
          machineHealthCheck:
            unhealthyConditions:
              - type: Ready
                status: Unknown
                timeout: 300s
              - type: Ready
                status: "False"
                timeout: 300s
        - class: windows-worker
          template:
            bootstrap:
              ref:
                apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
                kind: KubeadmConfigTemplate
                name: existing-boot-ref-windows
            infrastructure:
              ref:
                apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
                kind: VSphereMachineTemplate
                name: windows-vsphere-template
          # This will create a health check for each deployment created with the "windows-worker" MachineDeploymentClass
          machineHealthCheck:
            unhealthyConditions:
              - type: Ready
                status: Unknown
                timeout: 300s
              - type: Ready
                status: "False"
                timeout: 300s
      infrastructure:
        ref:
          apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
          kind: VSphereClusterTemplate
          name: vsphere-prod-cluster-template
   ```

2. User creates a cluster using the class name and defining the topology.
   ```yaml
    apiVersion: cluster.x-k8s.io/v1beta1
    kind: Cluster
    metadata:
      name: foo
      namespace: bar
    spec:
      topology:
        class: mixed
        version: v1.19.1
        controlPlane:
          replicas: 3
          labels: {}
          annotations: {}
        workers:
          machineDeployments:
          - class: linux-worker
            name: big-pool-of-machines-1
            replicas: 5
            labels:
              # This label is additive to the class' labels,
              # or if the same label exists, it overwrites it.
              custom-label: "production"
          - class: linux-worker
            name: small-pool-of-machines-1
            replicas: 1
          - class: windows-worker
            name: microsoft-1
            replicas: 3
   ```
3. The system creates Cluster's control plane object according to the ControlPlane specification defined in ClusterClass 
   and the control plane attributes defined in the Cluster topology (the latter overriding the first in case of conflicts).
4. The system creates MachineDeployments listed in the Cluster topology using MachineDeployment class as a starting point
   and the MachineDeployment attributes also defined in the Cluster topology (the latter overriding the first in case of conflicts).
5. The system creates MachineHealthChecks objects for control plane and MachineDeployments.
   
![Creation of cluster with ClusterClass](./images/cluster-class/create.png)

##### Update an existing Cluster using ClusterClass

This section talks about updating a Cluster which was created using a `ClusterClass` object.
1. User updates the `cluster.spec.topology`.
2. System compares and updates InfrastructureCluster object, if the computed object after the change is different than the current one.
3. System compares and updates ControlPlane object, if necessary. This includes also comparing and rotating the InfrastructureMachineTemplate, if necessary.
4. System compares and updates MachineDeployment and/or MachinePool object, if necessary. This includes also
    1. Adding/Removing MachineDeployment/MachinePool, if necessary.
    2. Comparing and rotating the InfrastructureMachineTemplate and BootstrapTemplate for the existing MachineDeployments/MachinePools, if necessary.
    3. Comparing and updating the replicas, labels, annotations and version of the existing MachineDeployments/MachinePools, if necessary.
5. System compares and updates MachineHealthCheck objects corresponding to ControlPlane or MachineDeployments, if necessary.

![Update cluster with ClusterClass](./images/cluster-class/update.png)

#### Behavior with patches

This section highlights how the basic behavior discussed above changes when patches are used. This is an important use case because without 
patches all the Cluster derived from a ClusterClass would be almost the same, thus limiting the use cases a single ClusterClass can target. 
Patches are used to customize individual Clusters, to avoid creating separate ClusterClasses for every small variation, 
like e.g. a different HTTP proxy configuration, a different image to be used for the machines etc.

##### Create a new ClusterClass with patches

1. User creates a ClusterClass object with variables and patches (other fields are omitted for brevity).
   ```yaml
   apiVersion: cluster.x-k8s.io/v1beta1
   kind: ClusterClass
   metadata:
     name: my-cluster-class
   spec:
     [...]
     variables:
     - name: region
       required: true
       schema:
         openAPIV3Schema:
           type: string
     - name: controlPlaneMachineType
       schema:
         openAPIV3Schema:
           type: string
           default: t3.large
     patches:
     - name: region
       definitions:
       - selector:
           apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
           kind: AWSClusterTemplate
         jsonPatches:
         - op: replace
           path: “/spec/template/spec/region”
           valueFrom:
             variable: region
     - name: controlPlaneMachineType
       definitions:
       - selector:
           apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
           kind: AWSMachineTemplate
           matchResources:
             controlPlane: true
         jsonPatches:
         - op: replace
           path: “/spec/template/spec/instanceType”
           valueFrom:
             variable: machineType
   ```

##### Create a new Cluster with patches

1. User creates a Cluster referencing the ClusterClass created above and defining variables (other fields are omitted for brevity).
   ```yaml
   apiVersion: cluster.x-k8s.io/v1beta1
   kind: Cluster
   metadata:
     name: my-cluster
   spec:
     topology:
       class: my-cluster-class
       [...]
       variables:
       - name: region
         value: us-east-1
   ```
   **Note**: `controlPlaneMachineType` will be defaulted to `t3.large` through a mutating webhook based on the default value
   specified in the corresponding schema in the ClusterClass.

During reconciliation the cluster topology controller uses the templates referenced in the ClusterClass. 
However, in order to compute the desired state of the InfrastructureCluster, ControlPlane, BootstrapTemplates and 
InfrastructureMachineTemplates the patches will be considered. 
Most specifically patches are applied in the order in which they are defined in the ClusterClass; the resulting
templates are used as input for creating or updating the Cluster as described in previous paragraphs.

#### Provider implementation

**Impact on the bootstrap providers**:
- None.

**Impact on the controlPlane providers**:
- the provider implementers are required to implement the ControlPlaneTemplate type (e.g. `KubeadmControlPlaneTemplate` etc.).
- it is also important to notice that:
    - ClusterClass and managed topologies can work **only** with control plane providers implementing support for the `spec.version` field;
      Additionally, it is required to provide support for the `status.version` field reporting the minimum
      API server version in the cluster as required by the control plane contract.
    - ClusterClass and managed topologies can work both with control plane providers implementing support for
      machine infrastructures and with control plane providers not supporting this feature.
      Please refer to the control plane for the list of well known fields where the machine template
      should be defined (in case this feature is supported).
    - ClusterClass and managed topologies can work both with control plane providers implementing support for
      `spec.replicas` and with control plane provider not supporting this feature.

**Impact on the infrastructure providers**:

- the provider implementers are required to implement the InfrastructureClusterTemplate type (e.g. `AWSClusterTemplate`, `AzureClusterTemplate` etc.).

#### Conventions for template types implementation

Given that it is required to implement new templates, let's remind the conventions used for
defining templates and the corresponding objects:

Templates:

- Template fields must match or be a subset of the corresponding generated object.
- A template can't accept values which are not valid for the corresponding generated object,
  otherwise creating an object derived from a template will fail.
  
Objects generated from the template:

- For the fields existing both in the object and in the corresponding template:
    - The object can't have additional validation rules than the template,
      otherwise creating an object derived from a template could fail.
    - It is recommended to use the same defaulting rules implemented in the template,
      thus avoiding confusion in the users.
- For the fields existing only in the object but not in the corresponding template:
    - Fields must be optional or a default value must be automatically assigned,
      otherwise creating an object derived from a template will fail.

**Note:** The existing InfrastructureMachineTemplate and BootstrapMachineTemplate objects already
comply those conventions via explicit rules implemented in the code or via operational practices
(otherwise creating machines would not be working already today).

**Note:** As per this proposal, the definition of ClusterClass is immutable. The CC definition consists 
of infrastructure object references, say AWSMachineTemplate, which could be immutable. For such immutable
infrastructure objects, hard-coding the image identifiers leads to those templates being tied to a particular
Kubernetes version, thus making Kubernetes version upgrades impossible. Hence, when using CC, infrastructure
objects MUST NOT have mandatory static fields whose values prohibit version upgrades.

#### Notes on template <-> object reconciliation

One of the key points of this proposal is that cluster topologies are continuously
reconciled with the original templates to ensure consistency over time and to support changing the generated
topology when necessary.

More specifically, the topology controller uses [Server Side Apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/) to write/patch topology owned objects;
using SSA allows other controllers to co-author the generated objects.

However, this requires providers to pay attention on lists that are co-owned by multiple controller, for example lists that are expected to contain values from  ClusterClass/Variables, 
and thus managed by the CAPI topology controller, and values from the infrastructure provider itself, like e.g. subnets in CAPA.

In this cases for ServerSideApply to work properly it is required to ensure the proper annotation exists on the CRD
type definitions, like +MapType or +MapTypeKey, see [merge strategy](https://kubernetes.io/docs/reference/using-api/server-side-apply/#merge-strategy) for more details.

Note: in order to allow the topology controller to execute templates rotation only when strictly necessary, it is necessary
to implement specific handling of dry run operations in the templates webhooks as described in [Required Changes on providers from 1.1 to 1.2](https://cluster-api.sigs.k8s.io/developer/providers/migrations/v1.1-to-v1.2#required-api-changes-for-providers).

### Risks and Mitigations

This proposal tries to model the API design for ClusterClass with a narrow set of use cases. This initial implementation provides a baseline on which incremental changes can be introduced in the future. Instead of encompassing of all use cases under a single proposal, this proposal mitigates the risk of waiting too long to consider all required use cases under this topic.

## Alternatives

## Upgrade Strategy

Existing clusters created without ClusterClass cannot switch over to using ClusterClass for a topology.

## Additional Details

### Test Plan [optional]

TBD

### Graduation Criteria [optional]

The initial plan is to rollout Cluster Class and support for managed topologies under a feature flag which would be unset by default.

## Implementation History

- 04/05/2021: Proposed idea in an [issue](https://github.com/kubernetes-sigs/cluster-api/issues/4430)
- 05/05/2021: Compile a [Google Doc](https://docs.google.com/document/d/1lwxgBK3Q7zmNkOSFqzTGmrSys_vinkwubwgoyqSRAbI/edit#) following the CAEP template
- 05/19/2021: Present proposal at a community meeting
- 05/26/2021: Open proposal PR
- 07/21/2021: First version of the proposal merged
- 10/04/2021: Added support for patches and variables
- 01/10/2022: Added support for MachineHealthChecks
- 12/20/2022: Cleaned up outdated implementation details by linking the book's pages instead. This will make it easier to keep the proposal up to date.

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1Ys-DOR5UsgbMEeciuG0HOgDQc8kZsaWIWJeKJ1-UfbY
[Kubernetes API conventions]: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#lists-of-named-subobjects-preferred-over-maps
