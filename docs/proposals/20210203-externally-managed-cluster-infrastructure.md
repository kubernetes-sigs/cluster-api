---
title: Externally Managed cluster infrastructure
authors:
  - "@enxebre"
  - "@joelspeed"
  - "@alexander-demichev"
reviewers:
  - "@vincepri"
  - "@randomvariable"
  - "@CecileRobertMichon"
  - "@yastij"
  - "@fabriziopandini"
creation-date: 2021-02-03
last-updated: 2021-02-12
status: implementable
see-also:
replaces:
superseded-by:
---

# Externally Managed cluster infrastructure

## Table of Contents
   * [Externally Managed cluster infrastructure](#externally-managed-cluster-infrastructure)
      * [Table of Contents](#table-of-contents)
      * [Glossary](#glossary)
         * [Managed cluster infrastructure](#managed-cluster-infrastructure)
         * [Externally managed cluster infrastructure](#externally-managed-cluster-infrastructure-1)
      * [Summary](#summary)
      * [Motivation](#motivation)
         * [Goals](#goals)
         * [Non-Goals/Future Work](#non-goalsfuture-work)
      * [Proposal](#proposal)
         * [User Stories](#user-stories)
            * [Story 1 - Alternate control plane provisioning with user managed infrastructure](#story-1---alternate-control-plane-provisioning-with-user-managed-infrastructure)
            * [Story 2 - Restricted access to cloud provider APIs](#story-2---restricted-access-to-cloud-provider-apis)
            * [Story 3 - Consuming existing cloud infrastructure](#story-3---consuming-existing-cloud-infrastructure)
         * [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
            * [Provider implementation changes](#provider-implementation-changes)
         * [Security Model](#security-model)
         * [Risks and Mitigations](#risks-and-mitigations)
            * [What happens when a user converts an externally managed InfraCluster to a managed InfraCluster?](#what-happens-when-a-user-converts-an-externally-managed-infracluster-to-a-managed-infracluster)
         * [Future Work](#future-work)
            * [Marking InfraCluster ready manually](#marking-infracluster-ready-manually)
      * [Alternatives](#alternatives)
         * [ExternalInfra CRD](#externalinfra-crd)
         * [ManagementPolicy field](#managementpolicy-field)
      * [Upgrade Strategy](#upgrade-strategy)
      * [Additional Details](#additional-details)
      * [Implementation History](#implementation-history)

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

### Managed cluster infrastructure

Cluster infrastructure whose lifecycle is managed by a provider InfraCluster controller.
E.g. in AWS:
- Network
    - VPC
    - Subnets
    - Internet gateways
    - Nat gateways
    - Route tables
- Security groups
- Load balancers

### Externally managed cluster infrastructure

An InfraCluster resource (usually part of an infrastructure provider) whose lifecycle is managed by an external controller.

## Summary

This proposal introduces support to allow infrastructure cluster resources (e.g. AzureCluster, AWSCluster, vSphereCluster, etc.) to be managed by an external controller or tool.

## Motivation

Currently, Cluster API infrastructure providers support an opinionated happy path to create and manage cluster infrastructure lifecycle.
The fundamental use case we want to support is out of tree controllers or tools that can manage these resources.

For example, users could create clusters using tools such as Terraform, Crossplane, or Kops and run CAPI on top of installed infrastructure.

The proposal might also ease adoption of Cluster API in heavily restricted environments where the provider infrastructure for the cluster needs to be managed out of band.

### Goals

- Introduce support for "externally managed" cluster infrastructure consistently across Cluster API providers.
- Any machine controller or machine infrastructure controllers must be able to keep operating like they do today.
- Reuse existing InfraCluster CRDs in "externally managed" clusters to minimise differences between the two topologies.

### Non-Goals/Future Work

- Modify existing managed behaviour.
- Automatically mark InfraCluster resources as ready (this will be up to the external management component initially).
- Support anything other than cluster infrastructure (e.g. machines).

## Proposal

A new annotation `cluster.x-k8s.io/managed-by: "<name-of-system>"` is going to be defined in Cluster API core repository, which helps define and identify resources managed by external controllers. The value of the annotation will not be checked by Cluster API and is considered free form text.

Infrastructure providers SHOULD respect the annotation and its contract.

When this annotation is present on an InfraCluster resource, the InfraCluster controller is expected to ignore the resource and not perform any reconciliation.
Importantly, it will not modify the resource or its status in any way.
A predicate will be provided in the Cluster API repository to aid provider implementations in filtering resources that are externally managed.

Additionally, the external management system must provide all required fields within the spec of the InfraCluster and must adhere to the CAPI provider contract and set the InfraCluster status to be ready when it is appropriate to do so.

While an "externally managed" InfraCluster won't reconcile or manage the lifecycle of the cluster infrastructure, CAPI will still be able to create compute nodes within it.

The machine controller must be able to operate without hard dependencies regardless of the cluster infrastructure being managed or externally managed.
![](https://i.imgur.com/nA61XJt.png)

### User Stories

#### Story 1 - Alternate control plane provisioning with user managed infrastructure
As a cluster provider I want to use CAPI in my service offering to orchestrate Kubernetes bootstrapping while letting workload cluster operators own their infrastructure lifecycle.

For example, Cluster API Provider AWS only supports a single architecture for delivery of network resources for cluster infrastructure, but given the possible variations in network architecture in AWS, the majority of organisations are going to want to provision VPCs, security groups and load balancers themselves, and then have Cluster API Provider AWS provision machines as normal. Currently CAPA supports "bring your own infrastructure" when users fill in the `AWSCluster` spec, and then CAPA reconciles any missing resources. This has been done in an ad hoc fashion, and has proven to be a frequently brittle mechanism with many bugs. The AWSMachine controller only requires a subset of the AWSCluster resource in order to reconcile machines, in particular - subnet, load balancer (for control plane instances) and security groups. Having a formal contract for externally managed infrastructure would improve the user experience for those getting started with Cluster API and have non-trivial networking requirements.

#### Story 2 - Restricted access to cloud provider APIs
As a cluster operator I want to use CAPI to orchestrate kubernetes bootstrapping while restricting the privileges I need to grant for my cloud provider because of organisational cloud security constraints.

#### Story 3 - Consuming existing cloud infrastructure
As a cluster operator I want to use CAPI to orchestrate Kubernetes bootstrapping while reusing infrastructure that has already been created in the organisation either by me or another team.

Following from the example in Story 1, many AWS environments are tightly governed by an organisation's cloud security operations unit, and provisioning of security groups in particular is often prohibited.

### Implementation Details/Notes/Constraints

**Managed**

- It will be default and will preserve existing behaviour. An InfraCluster CR without the `cluster.x-k8s.io/managed-by: "<name-of-system>"` annotation.


**Externally Managed**

An InfraCluster CR with the `cluster.x-k8s.io/managed-by: "<name-of-system>"` annotation.

The provider InfraCluster controller must:
- Skip any reconciliation of the resource.

- Not update the resource or its status in any way

The external management system must:

- Populate all required fields within the InfraCluster spec to allow other CAPI components to continue as normal.

- Adhere to all Cluster API contracts for infrastructure providers.

- When the infrastructure is ready, set the appropriate status as is done by the provider controller today.

#### Provider implementation changes

To enable providers to implement the changes required by this contract, Cluster API is going to provide a new `predicates.ResourceExternallyManaged` predicate as part of its utils.

This predicate filters out any resource that has been marked as "externally managed" and prevents the controller from reconciling the resource.

### Security Model

When externally managed, the required cloud provider privileges required by CAPI might be significantly reduced when compared with a traditionally managed cluster.
The only privileges required by CAPI are those that are required to manage machines.

For example, when an AWS cluster is managed by CAPI, permissions are required to be able to create VPCs and other networking components that are managed by the AWSCluster controller. When externally managed, these permissions are not required as the external entity is responsible for creating such components.

Support for minimising permissions in Cluster API Provider AWS will be added to its IAM provisioning tool, `clusterawsadm`.

### Risks and Mitigations

#### What happens when a user converts an externally managed InfraCluster to a managed InfraCluster?

There currently is no immutability support for CRD annotations within the Kubernetes API.

This means that, once a user has created their externally managed InfraCluster, they could at some point, update the annotation to make the InfraCluster appear to be managed.

There is no way to predict what would happen in this scenario.
The InfraCluster controller would start attempting to reconcile infrastructure that it did not create, and therefore, there may be assumptions it makes that mean it cannot manage this infrastructure.

To prevent this, we will have to implement (in the InfraCluster webhook) a means to prevent users converting externally managed InfraClusters into managed InfraClusters.

Note however, converting from managed to externally managed should cause no issues and should be allowed.
It will be documented as part of the externally managed contract that this is a one way operation.

### Future Work

#### Marking InfraCluster ready manually

The content of this proposal assumes that the management of the external infrastructure is done by some controller which has the ability to set the spec and status of the InfraCluster resource.

In reality, this may not be the case. For example, if the infrastructure was created by an admin using Terraform.

When using a system such as this, a user can copy the details from the infrastructure into an InfraCluster resource and create this manually.
However, they will not be able to set the InfraCluster to ready as this requires updating the resource status which is difficult when not using a controller.

To allow users to adopt this external management pattern without the need for writing their own controllers or tooling, we will provide a longer term solution that allows a user to indicate that the infrastructure is ready and have the status set appropriately.

The exact mechanism for how this will work is undecided, though the following ideas have been suggested:

- Reuse of future kubectl subresource flag capabilities https://github.com/kubernetes/kubernetes/pull/99556.

- Add a secondary annotation to this contract that causes the provider InfraCluster controller to mark resources as ready

## Alternatives

### ExternalInfra CRD

We could have an adhoc CRD https://github.com/kubernetes-sigs/cluster-api/issues/4095

This would introduce complexity for the CAPI ecosystem with yet an additional CRD and it wouldn't scale well across providers as it would need to contain provider specific information.

### ManagementPolicy field

As an alternative to the proposed annotation, a `ManagementPolicy` field on Infrastructure Cluster spec could be required as part of this contract.
The field would be an enum that initially has 2 possible values: managed and unmanaged.
That would require a new provider contract and modification of existing infrastructure CRDs, so this option is not preferred.

## Upgrade Strategy

Support is introduced by adding a new annotation for the provider infraCluster.

This makes any transition towards an externally managed cluster backward compatible and leave the current managed behaviour untouched.

## Additional Details

## Implementation History

- [x] 11/25/2020: Proposed idea in an issue or [community meeting] https://github.com/kubernetes-sigs/cluster-api-provider-aws/pull/2124
- [x] 02/03/2021: Compile a Google Doc following the CAEP template https://hackmd.io/FqsAdOP6S7SFn5s-akEPkg?both
- [x] 02/03/2021: First round of feedback from community
- [x] 03/10/2021: Present proposal at a [community meeting]
- [x] 02/03/2021: Open proposal PR

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1Ys-DOR5UsgbMEeciuG0HOgDQc8kZsaWIWJeKJ1-UfbY
