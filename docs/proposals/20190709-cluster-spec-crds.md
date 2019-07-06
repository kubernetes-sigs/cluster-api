---
title: Cluster Provider Spec and Status as CRDs 
authors:
  - "@pablochacin"
reviewers:
  - "@ncdc"
  - "@vincepri"
creation-date: 2019-07-09
last-updated: 2019-07-09
status: provisional
see-also:
  - "/docs/proposals/20190610-machine-states-preboot-bootstrapping.md"
---

# Cluster Spec & Status CRDs


## Table of Contents

- [Title](#title)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
    * [Data model changes](#data-model-changes)
    * [Controller collaboration[#controller-collaboration)
    - [User Stories [optional]](#user-stories-optional)
    - [Implementation Details/Notes/Constraints [optional]](#implementation-detailsnotesconstraints-optional)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Design Details](#design-details)
    - [Test Plan](#test-plan)
    - [Graduation Criteria](#graduation-criteria)
    - [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)
    - [Version Skew Strategy](#version-skew-strategy)
  - [Implementation History](#implementation-history)
  - [Drawbacks [optional]](#drawbacks-optional)
  - [Alternatives [optional]](#alternatives-optional)
  - [Infrastructure Needed [optional]](#infrastructure-needed-optional)


## Summary

In Cluster API (CAPI) v1alpha1 Cluster object contains the `ClusterSpec` and `ClusterStatus` structs. Both structs contain provider-specific fields, `ProviderSpec` and `ProviderStatus` respectively, embedded as opaque [RawExtensions](https://godoc.org/k8s.io/apimachinery/pkg/runtime#RawExtension).

The Cluster controller does not handle these fields directly. Instead, the provider-specific actuator receives the `ProviderSpec` as part of the Cluster object and sets the `ProviderStatus` to reflect the status of the cluster infrastructure. 

This proposal outlines the replacement of these provider-specific embedded fields by references to objects managed by a provider controller, effectively eliminating the actuator interface. The new objects are introduced and the cooperation between the Cluster controller and the Cluster Infrastructure Controller is outlined. This process introduces a new field `InfrastructureState` which reflects the state of the provisioning process.

## Motivation

Embedding opaque provider-specific information has some disadvantages: 
- Using an embedded provider spec as a `RawExtension` means its content is not validated as part of the Cluster object validation
- Embedding the provider spec makes difficult implementing the logic of the provider as an independent controller watching for changes in the specs. Instead, providers implement cluster controllers which embed the generic logic and a provider-specific actuator which implements the logic for handling events in the Cluster object.  
- Embedding the provider status as part of the Cluster requires that either the generic cluster controller be responsible for pulling this state from the provider (e.g. by invoking a provider actuator) or the provider updating this field, incurring in overlapping responsibilities. 

### Goals

1. To use Kubernetes Controllers to manage the lifecycle of provider specific cluster infrastructure object.
1. To validate provider-specific object as early as possible (create, update).
1. Allow cluster providers to expose the status via a status subresource, eliminating the need of status updates across controllers.

### Non-Goals/Future Work
1. To modify the generic Cluster object beyond the elements related to the provider-specific specs and status.
1. Splitting the provider-specific infrastructure into more granular elements, such as network infrastructure and control plane specs. Revisiting the definition of the provider specific infrastructure is left for further proposals.
1. To replace the cluster status by alternative representations, such as a list of conditions.
1. To propose cluster state lifecycle hooks. This must be part of a future proposal that builds on these proposed changes

## Proposal

This proposal introduces changes in the Cluster data model and describes a collaboration model between the Cluster controller and the provider infrastructure controller, including a model for managing the state transition during the provisioning process. 

### Data Model changes

```go
type ProviderSpec struct
```
- **To remove**
    - **Value** [optional] _Superseded by InfrastructureRef_
        - Type: `*runtime.RawExtension`
        - Description: Value is an inlined, serialized representation of the resource configuration. It is recommended that providers maintain their own versioned API types that should be serialized/deserialized from this field, akin to component config.

- **To add**
    - **InfrastructureRef** [optional]
        - Type: `*corev1.ObjectReference`
        - Description: InfrastructureRef is a reference to a provider-specific resource that holds the details for provisioning a cluster in said provider. This reference is optional and allows the provider to implement its own cluster specification details.

```go
type ClusterStatus struct
```
- **To remove**
    - **ProviderStatus** [optional]
        - Type: `*runtime.RawExtension`
        - Description:  Provider-specific status. It is recommended that providers maintain their own versioned API types that should be serialized/deserialized from this field.

- **To add**
    - **InfrastructureStatusRef** [optional]
        - Type: `*corev1.ObjectReference`
        - Description: InfrastructureStatusRef is a reference to a cluster provider-specific resource that holds the status of the cluster in said provider. This reference is optional and allows the provider to implement its own cluster status details.
    - **InfrastructureState** [optional]
       - Type: `InfrastructureState`
       - Description: InfrastructureState indicates the state of the infrastructure provisioning process.

### Controller collaboration

Moving the provider-specific infrastructure specs to a separate object makes a clear separation of responsibilities between the generic Cluster controller and the provider's cluster controller, but also introduces the need for coordination as the provider controller will likely require information from the Cluster object during the cluster provisioning process.

It is also necessary to model the state of the cluster along the provisioning process to be able to keep track of its progress. In order to do so, a new `ClusterInfrastructureState` field is introduced as part of the `ClusterStatus` struct. The state and their transitions conditions are explained in detail in section [States and Transition](#states-and-transitions).

The sequence diagram below describes the hight-level process, collaborations and state transitions.  

```
    O          +------------+                +------------+               +--------------+
  --|--        |     API    |                |   Cluster  |               |Infrastructure|
   / \         |   Server   |                | Controller |               | Controller   |
    |          +------------+                +------------+               +--------------+
    | Create          |                             |                             |
    | Cluster         |                             |                             |
    |---------------->|  New Cluster                |                             |
    |                 |---------------------------> |                             |
    |                 |                            +-+                            |
    |                 |  +-----------------------------------------+              |    
    |                 |  | IF InfrastructureRef is | |             |              |    
    |                 |  |    not set              | |             |              |    
    |                 |  |                         | |             |              |    
    |                 |  | Set InfrastructureState | |             |              |    
    |                 |  | to "Pending"            | |             |              |    
    |                 |<-+-------------------------| |             |              |    
    |                 |  +-----------------------------------------+              |
    |                 |                            +-+                            |
    |                 |                             |                             |
    |                 |  New Provider Infrastructure|                             |
    |                 |-----------------------------+---------------------------->|
    |                 |                             |                            +-+
    |                 |                             |  +-----------------------------------------+
    |                 |                             |  | IF Infrastructure has   | |             |
    |                 |                             |  |    no owner ref         | |--+          |
    |                 |                             |  |                         | |  | Do       |
    |                 |                             |  |                         | |<-+ nothing  |
    |                 |                             |  |                         | |             |
    |                 |                             |  +-----------------------------------------+
    |                 |                             |                            +-+
    |                 |                             |                             |
    |                 |  Cluster updated           +-+                            |
    |                 |--------------------------->| |                            |
    |                 |  +---------------------------------------------+          |    
    |                 |  | IF InfrastructureRef    | |                 |          |    
    |                 |  |    is set               | |                 |          |    
    |                 |  |                         | |                 |          |    
    |                 |  | Get Infrastructure      | |                 |          |
    |                 |<---------------------------| |                 |          |
    |                 |  | +-----------------------------------------+ |          |    
    |                 |  | | IF Not seen before    | |               | |          |    
    |                 |  | |                       | |--+            | |          |    
    |                 |  | |                       | |  | Watch      | |          |    
    |                 |  | |                       | |  | Infra-     | |          |    
    |                 |  | |                       | |  | structure  | |          |    
    |                 |  | |                       | |<-+            | |          |    
    |                 |  | +-----------------------------------------+ |          |
    |                 |  |                         | |                 |          |
    |                 |  | +-----------------------------------------+ |          |    
    |                 |  | | IF Infrastructure has | |               | |          |    
    |                 |  | |    no owner           | |               | |          |    
    |                 |  | |                       | |               | |          |    
    |                 |  | | Set Infrastructure's  | |               | |          |    
    |                 |  | | owner to Cluster      | |               | |          |    
    |                 |<-+++-----------------------| |               | |          |    
    |                 |  | |                       | |               | |          |    
    |                 |  | | Set InfrastructureStat| |               | |          |    
    |                 |  | | to "Provisioning"     | |               | |          |    
    |                 |<-+++-----------------------| |               | |          |    
    |                 |  | |                       | |               | |          |    
    |                 |  | +-----------------------------------------+ |          |
    |                 |  +++-------------------------------------------+          |
    |                 |                            +-+                            |
    |                 |                             |                             |
    |                 |  Provider Infrastructure update                           |
    |                 |-----------------------------+---------------------------->|
    |                 |                             |                            +-+
    |                 |                             |  +-----------------------------------------+
    |                 |                             |  | IF Infrastructure has   | |             |
    |                 |  Get Cluster                |  |    owner ref            | |             |
    |                 |<----------------------------+--+-------------------------| |             |
    |                 |                             |  |                         | |--+          |
    |                 |                             |  |                         | |  | Provision|
    |                 |                             |  |                         | |  | infra-   |
    |                 | Set InfrastructureStatusRef |  |                         | |<-+ structure|
    |                 |<----------------------------+--+-------------------------| |             |
    |                 |                             |  |                         | |             |
    |                 |                             |  +-----------------------------------------+
    |                 |                             |                            +-+
    |                 |  Cluster update             |                             |
    |                 |---------------------------> |                             |
    |                 |                            +-+                            |
    |                 |  +-----------------------------------------+              |    
    |                 |  | IF InfrastructureStatusRef              |              |    
    |                 |  |    is set               | |             |              |    
    |                 |  |                         | |             |              |    
    |                 |  | Set InfrastructureStage | |             |              |    
    |                 |  | to "Provisioned"        | |             |              |    
    |                 |<-+-------------------------| |             |              |    
    |                 |  |                         | |             |              |    
    |                 |  +-----------------------------------------+              |
    |                 |                            +-+                            |
    |                 |                             |                             |
```
Figure 1: Cluster provisioning process

Figure 1 presents the sequence of actions involved in provisioning a cluster, highlighting the coordination required between the CAPI cluster controller and the provider infrastructure controller. 

The creation of the Cluster and provider infrastructure objects are independent events. When a Cluster object is created, if the `InfrastructureRef` is not set, the cluster controller will set the `InfrastructureState` to "Pending". When a provider infrastructure object is created, the provider's controller will do nothing unless its owner reference is set to a cluster. 

When the `InfrastructureRef` is set in the cluster, the cluster controller will retrieve the infrastructure object. If the object has not been seen before, it will start watching it. Also, if the object's owner is not set, it will set to the Cluster object and will set the cluster's `InfrastructureState` to "Provisioning". 

When an infrastructure object is updated, the provider controller will check the owner reference. If it is set, it will retrieve the cluster object to obtain the required cluster specification and starts the provisioning process. When the process finishes, it sets the `InfrastructureStatusRef` in the cluster object. When the cluster controller detects the `InfrastructureStatusRef` is set, it sets the status to "Provisioned". 

### States and Transitions

The Cluster Controller has the responsibility of managing the state transitions during cluster lifecycle. In order to do so, it requires collaboration with the provider's controller to clearly signal the state transitions without requiring knowledge of the internals of the provider. 

#### Pending

The initial state when the Cluster object has been created but there is no provider's infrastructure associated with it. 

##### Conditions
- `Cluster.ProviderSpec.InsfrastructureRef` is an empty string

#### Provisioning

The cluster has a provider infrastructure object associated. The provider can start provisioning the infrastructure.

##### Transition Conditions

- The Cluster object has the `InfrastructureRef` field set 
- The Infrastructure object has its owner set to the Cluster

#### Provisioned

The provider has finished provisioning the infrastructure.

##### Conditions
- The Cluster object has the `InfrastructureStatusRef` set 

```
               +----------------+
               |                |   Cluster object is created
               |    Pending     |   but has no associated 
               |                |   Infrastructure
               +----------------+
                       |
                       V
               +----------------+
               |                |   Cluster has an associated
               |  Provisioning  |   infrastructure. Provider is 
               |                |   provisioning it.
               +----------------+
                       |
                       V
               +----------------+
               |                |   Provider has signaled the
               |  Provisioned   |   end of the provisioning 
               |                |   
               +----------------+
```
Figure 2. Cluster State transitions

### User Stories [optional]

#### As an infrastructure provider author, I would like to take advantage of the Kubernetes API to provide validation for provider-specific data needed to provision a machine.

#### As an infrastructure provider author, I would like to build a controller to manage provisioning machines using tools of my own choosing.

#### As an infrastructure provider author, I would like to build a controller to manage provisioning clusters without being restricted to a CRUD API.


### Implementation Details/Notes/Constraints [optional]

#### Role of Cluster Controller
The Cluster Controller should be the only controller having write permissions to the Cluster objects. Its main responsibility is to 
- Manage cluster finalizers
- Set provider's infrastructure object's OwnerRef to Cluster
- Update the state of the infrastructure provisioning

#### Cluster Controller dynamic watchers

As the provider spec data is no longer inlined in the Cluster object, the Cluster Controller needs to watch for updates to provider specific resources so it can detect events. To achieve this, when the Cluster Controller reconciles a Cluster and detects a reference to a provider-specific infrastructure object, it starts watching this resource using the [dynamic informer](https://godoc.org/k8s.io/client-go/dynamic/dynamicinformer) and a [`handler.EnqueueRequestForOwner`](https://godoc.org/sigs.k8s.io/controller-runtime/pkg/handler#EnqueueRequestForOwner) configured with Cluster as the OwnerType, to ensure it only watches provider objects related to a CAPI cluster.

#### Handling optionality

As both `InfrastructureRef` and `InfrastructureStatusRef` are optional, implementation must be able of differentiating the case when these fields are used by a provider, but have not yet been set, from the case when they are not in use. 
### Risks and Mitigations

## Design Details

### Test Plan

TODO

### Graduation Criteria

TODO

### Upgrade / Downgrade Strategy

TODO

### Version Skew Strategy

TODO

## Implementation History

- [x] 03/20/2019 [Issue Opened](https://github.com/kubernetes-sigs/cluster-api/issues/833) proposing the externalization of embeded provider objects as CRDs
- [x] 06/19/2009 [Discussed](https://github.com/kubernetes-sigs/cluster-api/issues/833#issuecomment-501380522) the inclusion in the scope of v1alpha2.
- [x] 07/09/2019 initial version of the proposal created
- [ ] Presentation of proposal to the community
- [ ] Feedback

## Drawbacks [optional]

TODO

## Alternatives [optional]

