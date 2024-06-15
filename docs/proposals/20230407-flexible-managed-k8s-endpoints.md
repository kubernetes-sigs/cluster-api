<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Flexible Managed Kubernetes Endpoints](#flexible-managed-kubernetes-endpoints)
  - [Glossary](#glossary)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
    - [Future Work](#future-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
      - [Story 3](#story-3)
      - [Story 4](#story-4)
      - [Story 5](#story-5)
      - [Story 6](#story-6)
      - [Story 7](#story-7)
    - [Design](#design)
      - [Core Cluster API changes](#core-cluster-api-changes)
      - [Infra Providers API changes](#infra-providers-api-changes)
      - [Core Cluster API Controllers changes](#core-cluster-api-controllers-changes)
      - [Provider controller changes](#provider-controller-changes)
    - [Guidelines for infra providers implementation](#guidelines-for-infra-providers-implementation)
  - [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

---
title: Flexible Managed Kubernetes Endpoints
authors:
  - "@jackfrancis"
reviewers:
  - "@richardcase"
  - "@pydctw"
  - "@mtougeron"
  - "@CecileRobertMichon"
  - "@fabriziopandini"
  - "@sbueringer"
  - "@killianmuldoon"
  - "@mboersma"
  - "@nojnhuh"
creation-date: 2023-04-07
last-updated: 2023-04-07
status: provisional
see-also:
  - "/docs/proposals/20220725-managed-kubernetes.md"
---

# Flexible Managed Kubernetes Endpoints

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

The following terms will be used in this document.

- Managed Kubernetes
  - Managed Kubernetes refers to any Kubernetes Cluster provisioning and maintenance abstraction, usually exposed as an API, that is natively available in a Cloud provider. For example: [EKS](https://aws.amazon.com/eks/), [OKE](https://www.oracle.com/cloud/cloud-native/container-engine-kubernetes/), [AKS](https://azure.microsoft.com/en-us/products/kubernetes-service), [GKE](https://cloud.google.com/kubernetes-engine), [IBM Cloud Kubernetes Service](https://www.ibm.com/cloud/kubernetes-service), [DOKS](https://www.digitalocean.com/products/kubernetes), and many more throughout the Kubernetes Cloud Native ecosystem.
- `ControlPlane Provider`
  - When we say `ControlPlane Provider` we refer to a solution that implements a solution for the management of a Kubernetes [control plane](https://kubernetes.io/docs/concepts/#kubernetes-control-plane) according to the Cluster API contract. Please note that in the context of managed Kubernetes, the `ControlPlane Provider` usually wraps the corresponding abstraction for a specific Cloud provider. Concrete example for Microsoft Azure is the `AzureManagedControlPlane`, for AWS the `AWSManagedControlPlane`, for Google the `GCPManagedControlPlane` etc.
- _Kubernetes Cluster Infrastructure_
  - When we refer to _Kubernetes Cluster Infrastructure_ (abbr. _Cluster Infrastructure_) we refer to the **infrastructure that supports a Kubernetes cluster**, like e.g. VPC, security groups, load balancers etc. Please note that in the context of Managed Kubernetes some of those components are going to be provided by the corresponding abstraction for a specific Cloud provider (EKS, OKE, AKS etc), and thus Cluster API should not take care of managing a subset or all those components.
- `<Infra>Cluster`
  - When we say `<Infra>Cluster` we refer to any provider that provides Kubernetes Cluster Infrastructure for a specific Cloud provider. Concrete example for Microsoft Azure is the `AzureCluster` and the `AzureManagedCluster`, for AWS the `AWSCluster` and the `AWSManagedCluster`, for Google Cloud the `GCPCluster` and the `GCPManagedCluster`).
- e.g.
  - This just means "For example:"!

## Summary

This proposal aims to address the lesson learned by running Managed Kubernetes solution on top of Cluster API, and make this use case simpler and more straight forward both for Cluster API users and for the maintainers of the Cluster API providers.

More specifically we would like to introduce first class support for two scenarios:

- Permit omitting the `<Infra>Cluster` entirely, thus making it simpler to use with Cluster API all the Managed Kubernetes implementations which do not require any additional Kubernetes Cluster Infrastructure (network settings, security groups, etc) on top of what is provided out of the box by the managed Kubernetes primitive offered by a Cloud provider.
- Allow the `ControlPlane Provider` component to take ownership of the responsibility of creating the control plane endpoint, thus making it simpler to use with Cluster API all the Managed Kubernetes implementations which are taking care out of the box of this piece of Cluster Infrastructure.
  - Note: In May 2024 [this pull request](https://github.com/kubernetes-sigs/cluster-api/pull/10667) added the ability for the control plane provider to provide the endpoint the same way the infrastructure cluster would.

The above capabilities can be used alone or in combination depending on the requirements of a specific Managed Kubernetes or on the specific architecture/set of Cloud components being implemented.

## Motivation

The implementation of Managed Kubernetes scenarios by Cluster API providers occurred after the architectural design of Cluster API, and thus that design process did not consider these Managed Kubernetes scenarios as a user story. In practice, Cluster API's specification has allowed Managed Kubernetes solutions to emerge that aid running fleets of clusters at scale, with CAPA's `AWSManagedCluster` and `AzureManagedCluster` being notable examples. However, because these Managed Kubernetes solutions arrived after the Cluster API contract was defined, providers have not settled on a consistent rendering of how a "Service-Managed Kubernetes" specification fits into a "Cluster API-Managed Kubernetes" surface area.

One particular part of the existing Cluster API surface area that is inconsistent with most Managed Kubernetes user experiences is the accounting of the [Kubernetes API server](https://kubernetes.io/docs/concepts/overview/components/#kube-apiserver). In the canonical "self-managed" user story that Cluster API addresses, it is the provider implementation of Cluster API (e.g., CAPA) that is responsible for scaffolding the necessary _Kubernetes Cluster Infrastructure_ that is required in order to create the Kubernetes API server (e.g., a Load Balancer and a public IP address). This provider responsibility is declared in the `<Infra>Cluster` resource, and carried out via its controllers; and then finally this reconciliation is synchronized with the parent `Cluster` Cluster API resource.

Because there exist Managed Kubernetes scenarios that handle a subset or all _Kubernetes Cluster Infrastructure_ responsibilities themselves, Cluster API's requirement of a `<Infra>Cluster` resource leads to undesirable implementation decisions, because in these scenarios there is no actual work for a Cluster API provider to do to scaffold _Kubernetes Cluster Infrastructure_.

Finally, for Managed Kubernetes scenarios that _do_ include additional, user-exposed infra (e.g., GKE and EKS as of this writing), we want to make it easier to account for the representation of the Managed Kubernetes API server endpoint, which is not always best owned by a `<Infra>Cluster` resource.

### Goals

- Build upon [the existing Cluster API Managed Kubernetes proposal](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220725-managed-kubernetes.md). Any net new recommendations and/or proposals will be a continuation of the existing proposal, and consistent with its original conclusions.
- Identify and document API changes and controllers changes required to omit the `<Infra>Cluster` entirely, where this is applicable.
- Identify and document API changes and controllers changes required to allow the `ControlPlane Provider` component to take ownership of the responsibility of creating the control plane endpoint.
- Ensure any changes to the current behavioral contract are backwards-compatible.

### Non-Goals

- Introduce new "Managed Kubernetes" data types in Cluster API.
- Invalidate [the existing Cluster API Managed Kubernetes proposal and concluding recommendations](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220725-managed-kubernetes.md).

### Future Work

- Detailed documentation that references the flavors of Managed Kubernetes scenarios and how they can be implemented in Cluster API, with provider examples.

## Proposal

### User Stories

#### Story 1

As a cluster operator, I want to use Cluster API to provision and manage the lifecycle of a control plane that utilizes my service provider's managed Kubernetes control plane (i.e. EKS, AKS, GKE), so that I don’t have to worry about the management/provisioning of control plane nodes, and so I can take advantage of any value add services offered by my cloud provider.

#### Story 2

As a cluster operator, I want to be able to provision both "unmanaged" and "managed" Kubernetes clusters from the same management cluster, so that I can support different requirements and use cases as needed whilst using a single operating model.

#### Story 3

As a Cluster API provider implementor, I want to be able to return the control plane endpoint created by the `ControlPlane Provider`, so that it fits naturally with how most of the native Managed Kubernetes implementations works.

#### Story 4

As a Cluster API provider developer, I want guidance on how to incorporate a managed Kubernetes service into my provider, so that its usage is compatible with Cluster API architecture/features and its usage is consistant with other providers.

#### Story 5

As a Cluster API provider developer, I want to enable the ClusterClass feature for a Managed Kubernetes service, so that users can take advantage of an improved UX with ClusterClass-based clusters.

#### Story 6

As a cluster operator, I want to use Cluster API to provision and manage the lifecycle of worker nodes that utilizes my cloud providers' managed instances (if they support them), so that I don't have to worry about the management of these instances.

#### Story 7

As a service provider I want to be able to offer Managed Kubernetes clusters by using CAPI referencing my own managed control plane implementation that satisfies Cluster API contracts.

### Design

Below we are documenting API changes and controllers changes required to omit the `<Infra>Cluster` entirely and to allow the `ControlPlane Provider` component  to take ownership of the responsibility of creating the control plane endpoint.

#### Core Cluster API changes

This proposal does not introduce any breaking changes for the existing "core" API. More specifically:

The existing Cluster API types are already able to omit the `<Infra>Cluster`:

- The `infrastructureRef` field on the Cluster object is already a pointer and thus it could be set to nil, and in fact we are already creating Clusters without `infrastructureRef` when we use a cluster class).
- The `infrastructure.Ref` field on the ClusterClass objects already a pointer and thus it could be set to nil, but in this case it is required to change the validation webhook to allow the user to not specify it; on top of that, when validating inline patches, we should reject patches targeting the infrastructure template objects if not specified.

In order to allow the `ControlPlane Provider` component to take ownership of the responsibility of creating the control plane endpoint we are going to introduce a new `ClusterEndpoint` CRD, below some example:

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterEndpoint
metadata:
   labels:
      cluster.x-k8s.io/cluster-name: my-cluster
spec:
  cluster: my-cluster
  host: "my-cluster-1234567890.region.elb.amazonaws.com"
  port: 1234
  type: ExternalControlPlaneEndpoint
```

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterEndpoint
metadata:
   labels:
      cluster.x-k8s.io/cluster-name: my-cluster-2
spec:
  cluster: my-cluster-2
  host: "10.40.85.102"
  port: 1234
  type: ExternalControlPlaneEndpoint
```

This is how the type specification would look:

```go
// ClusterEndpointType describes the type of cluster endpoint.
type ClusterEndpointType string

// ClusterEndpoint represents a reachable Kubernetes API endpoint serving a particular cluster function.
type ClusterEndpoint struct {
  metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

  Spec ClusterEndpointSpec   `json:"spec,omitempty"`
}

// ClusterEndpointSpec defines the desired state of the Cluster endpoint.
type ClusterEndpointSpec struct {
	// The Host is the DNS record or the IP address that the endpoint is reachable on.
	Host string `json:"host"`

	// The port on which the endpoint is serving.
	Port int32 `json:"port"`

  // Cluster is a reference to the cluster name that this endpoint is reachable on.
  Cluster string `json:"cluster"`

  // Type describes the function that this cluster endpoint serves.
  // +kubebuilder:validation:Enum=apiserver
  Type ClusterEndpointType `json:"type"`
}
```

The `<Infra>Cluster` object which is currently using the `spec.controlPlaneEndpoint` for the same scope will continue to work because "core" Cluster API controllers will continue to recognize when this field is set and take care of generating the `ClusterEndpoint` automatically; however this mechanism should be considered as a temporary machinery to migrate to the new CRD, and it will be removed in future versions of Cluster API. In addition, once the legacy behavior is removed, we will deprecate and eventually remove the `spec.controlPlaneEndpoint` field from the `Cluster` CustomResourceDefinition, and recommend that providers do the same for their `<Infra>Cluster` CustomResourceDefinitions as well.

Future Notes:

- A future `type` field can be introduced to enable CAPI to extend the usage of this CRD to address https://github.com/kubernetes-sigs/cluster-api/issues/5295 in a future iteration
- The current implementation originates from the `Cluster.spec.ControlPlaneEndpoint` field, which defines the info we need for this proposal; but in future iterations we might consider to support more addressed or more ports for each ClusterEndpoint, similarly what is implemented in the core v1 Endpoint type.

#### Infra Providers API changes

This proposal does not introduce any breaking changes for the provider's API.

However, Infra providers will be made aware that `spec.controlPlaneEndpoint` will be scheduled for deprecation in `<Infra>Cluster` resources in a future CAPI API version, with corresponding warning messages in controller logs. We will recommend that they remove it in a future API version of their provider.

#### Core Cluster API Controllers changes

- All the controllers working with ClusterClass objects must take into account that the `infrastructure.Ref` field could be omitted; most notably:
  - The ClusterClass controller must ignore nil `infrastructure.Ref` fields while adding owner references to all the objects referenced by a ClusterClass.
  - The Topology controller must skip the generation of the `<Infra>Cluster` objects when the `infrastructure.Ref` field in a ClusterClass is empty.

- All the controllers working with Cluster objects must take into account that the `infrastructureRef` field could be omitted; most notably:
  - The Cluster controller must use skip reconciling this external reference when the `infrastructureRef` is missing; also, the `status.InfrastructureReady` field must be automatically set to true in this case.

- A controller (details TBD) will reconcile the new `ClusterEndpoint` CR. Please note that:
  - The value from the `ClusterEndpoint` CRD must surface on the `spec.ControlPlaneEndpoint` field on the `Cluster` object.
  - If both are present, the value from the `ClusterEndpoint` CRD must take precedence on the value from `<Infra>Cluster` objects still using the `spec.controlPlaneEndpoint`.

- The Cluster controller must implement the temporary machinery to migrate to the new CRD existing Clusters and to deal with `<Infra>Cluster` objects still using the `spec.controlPlaneEndpoint` field as a way to communicate the ClusterAddress to "core" Cluster API controllers:
  - If there is the `spec.ControlPlaneEndpoint` on the `Cluster` object but not a corresponding `ClusterEndpoint` CR, the CR must be created.

#### Provider controller changes

- All the `<Infra>Cluster` controllers who are responsible for creating a control plane endpoint
  - As soon as the `spec.controlPlaneEndpoint` field in the `<Infra>Cluster` object will removed, the `<Infra>Cluster` controller must instead create a `ClusterEndpoint` CR to communicate the control plane endpoint to the Cluster API core controllers
    - NOTE: technically it is possible to start creating the `ClusterEndpoint` CR *before* the removal of the `spec.controlPlaneEndpoint` field, because the new CR will take precedence on the value read from the field, but this is up to the infra provider maintainers.
  - The `ClusterEndpoint` CR must have an owner reference to the `<Infra>Cluster` object from which it is originated.

- All the `ControlPlane Provider` controllers who are responsible for creating a control plane endpoint
  - Must no longer wait for the `spec.ControlPlaneEndpoint` field on the `Cluster` object to be set before starting to provision the control plane.
  - As soon as the Managed Kubernetes Service-provided control plane endpoint is available, the controller must create a `ClusterEndpoint` CR to communicate this to the control plane endpoint to the Cluster API core controllers
  - The `ClusterEndpoint` CR must have an owner reference to the `ControlPlane` object from which is originated.

### Guidelines for infra providers implementation

Let's consider following scenarios for an hypothetical `cluster-api-provider-foo` infra provider:

_Scenario 1._

If the `Foo` cloud provider has a `FKS` managed Kubernetes offering that is taking care of _the entire Kubernetes Cluster infrastructure_, the maintainers of the `cluster-api-provider-foo` provider:
- Must not implement a `FKSCluster` CRD and the corresponding `FKSClusterTemplate` CRD (nor the related controllers)
- Must implement a `FKRControlControlplane provider`, a `FKRControlControlplane` CRD, the corresponding `FKRControlControlplane` and related controllers
- The `FKRControlControlplane` controller:
  - Must not wait for `spec.ControlPlaneEndpoint` field on the `Cluster` object to be set before starting to provision the `FKS` managed Kubernetes instance.
  - As soon as the control plane endpoint is available, Must create a `ClusterEndpoint` CR to communicate the control plane endpoint to the Cluster API core controllers; the `ClusterEndpoint` CR must have an owner reference to the `FKRControlControlplane` object from which is originated.
  - Must set the `status.Ready` field on the `FKRControlControlplane` object when the provisioning is complete

_Scenario 2._

If the `Foo` cloud provider has a `FKS` managed Kubernetes offering that is taking care of _only of a subset of the Kubernetes Cluster infrastructure_, or it is required to provision some additional pieces of infrastructure on top of what provisioned out of the box, e.g. a SSH bastion host, the maintainers of the `cluster-api-provider-foo` provider:
- Must implement a `FKSCluster` CRD and the corresponding `FKSClusterTemplate` CRD and the related controllers
  - The `FKSCluster` controller
    - Must create only the additional piece of the _Kubernetes Cluster infrastructure_ not provisioned by the `FKS` managed Kubernetes instance (in this example a SSH bastion host)
    - Must not create a `ClusterEndpoint` CR (nor set the `spec.controlPlaneEndpoint` field in the `FKSCluster` object), because provisioning the control plane endpoint is not responsibility of this controller.
    - Must set the `status.Ready` field on the `FKSCluster` object when the provisioning is complete
- Must implement a `FKRControlControlplane provider`, a `FKRControlControlplane` CRD, the corresponding `FKRControlControlplane` and related controllers
  - The `FKRControlControlplane` controller:
    - Must wait for `status.InfrastructureReady` field on the `Cluster` object to be set to true before starting to provision the control plane.
    - Must not wait for `spec.ControlPlaneEndpoint` field on the `Cluster` object to be set before starting to provision the control plane.
    - As soon as the control plane endpoint is available, Must create a `ClusterEndpoint` CR to communicate the control plane endpoint to the Cluster API core controllers; the `ClusterEndpoint` CR must have an owner reference to the `FKRControlControlplane` object from which is originated.
    - Must set the `status.Ready` field on the `FKRControlControlplane` object when the provisioning is complete

_Scenario 3._

If the `Foo` cloud provider has a `FKS` managed Kubernetes offering that is not taking care of the control plane endpoint e.g. because it requires an existing `FooElasticIP`, a `FooElacticLoadBalancer` to be provisioned before creating the `FKS` managed Kubernetes cluster, the maintainers of the `cluster-api-provider-foo` provider:
- Must implement a `FKSCluster` CRD and the corresponding `FKSClusterTemplate` CRD and the related controllers; those controllers must create a `ClusterEndpoint` CR as soon as the control plane endpoint is available
  - The `FKSCluster` controller
    - Must create only the additional piece of the _Kubernetes Cluster infrastructure_ not provisioned by the `FKS` managed Kubernetes instance (in this example `FooElasticIP`, a `FooElacticLoadBalancer`)
    - As soon as the control plane endpoint is available, Must create a `ClusterEndpoint` CR; the `ClusterEndpoint` CR must have an owner reference to the `FKSCluster` object from which is originated.
    - Must set the `status.Ready` field on the `FKSCluster` object when the provisioning is complete
- Must implement a `FKRControlControlplane provider`, a `FKRControlControlplane` CRD, the corresponding `FKRControlControlplane` and related controllers
  - The `FKRControlControlplane` controller:
    - Must wait for `status.InfrastructureReady` field on the `Cluster` object to be set to true before starting to provision the `FKS` managed Kubernetes instance.
    - Must wait for `spec.ControlPlaneEndpoint` field on the `Cluster` object to be set before starting to provision the `FKS` managed Kubernetes instance.
    - Must set the `status.Ready` field on the `FKRControlControlplane` object when the provisioning is complete

Please note that this scenario is equivalent to what is implemented for a non managed Kubernetes `FooCluster`, backed by Cluster API managed `FooMachines`, with the only difference that in this case it possible to rely on `KCP` as `ControlControlplane provider`, and thus point 2 of the above list do not apply.

## Implementation History

- [x] 01/11/2023: Compile a Google Doc to organize thoughts prior to CAEP [link here](https://docs.google.com/document/d/1rqzZfsO6k_RmOHUxx47cALSr_6SeTG89e9C44-oHHdQ/)

[managedKubernetesRecommendation]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220725-managed-kubernetes.md#option-3-two-kinds-with-a-managed-control-plane-and-managed-infra-cluster-with-better-separation-of-responsibilities
