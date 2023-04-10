---
title: Contract Changes to Support Managed Kubernetes
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

# Contract Changes to Support Managed Kubernetes

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Contract Changes to Support Managed Kubernetes](#contract-changes-to-support-managed-kubernetes)
  - [Table of Contents](#table-of-contents)
  - [Glossary](#glossary)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
    - [Future work](#future-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
    - [Requirements (Optional)](#requirements-optional)
      - [Functional Requirements](#functional-requirements)
        - [FR1](#fr1)
        - [FR2](#fr2)
      - [Non-Functional Requirements](#non-functional-requirements)
        - [NFR1](#nfr1)
        - [NFR2](#nfr2)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [Security Model](#security-model)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Alternatives](#alternatives)
  - [Upgrade Strategy](#upgrade-strategy)
  - [Additional Details](#additional-details)
    - [Test Plan [optional]](#test-plan-optional)
    - [Graduation Criteria [optional]](#graduation-criteria-optional)
    - [Version Skew Strategy [optional]](#version-skew-strategy-optional)
  - [Implementation History](#implementation-history)

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

The following terms will be used in this document.

- `<Infra>Cluster`
  - When we say `<Infra>Cluster` we refer to any provider's infra-specific implementation of the Cluster API `Cluster` resource spec. When you see `<Infra>`, interpret that as a placeholder for any provider implementation. Some concrete examples of provider infra cluster implementations are Azure's CAPZ provider (e.g., `AzureCluster` and `AzureManagedCluster`), AWS's CAPA provider (e.g., `AWSCluster` and `AWSManagedCluster`), and Google Cloud's CAPG provider (e.g., `GCPCluster` and `GCPManagedCluster`). Rather than referencing any one of the preceding actual implementations of infra cluster resources, we prefer to generalize to `<Infra>Cluster` so that we don't suggest any provider-specific bias informing our conclusions.
- `<Infra>ControlPlane`
  - When we say `<Infra>ControlPlane` we refer to any provider's infra-specific implementation of the a Kubernetes cluster's control plane. When you see `<Infra>`, interpret that as a placeholder for any provider implementation. Some concrete examples of provider infra control plane implementations are Azure's CAPZ provider (e.g., `AzureManagedControlPlane`), AWS's CAPA provider (e.g., `AWSManagedControlPlane`), and Google Cloud's CAPG provider (e.g., `GCPManagedControlPlane`).
- Managed Kubernetes
  - Managed Kubernetes refers to any Kubernetes Cluster provisioning and maintenance platform that is exposed by a service API. For example: [EKS](https://aws.amazon.com/eks/), [OKE](https://www.oracle.com/cloud/cloud-native/container-engine-kubernetes/), [AKS](https://azure.microsoft.com/en-us/products/kubernetes-service), [GKE](https://cloud.google.com/kubernetes-engine), [IBM Cloud Kubernetes Service](https://www.ibm.com/cloud/kubernetes-service), [DOKS](https://www.digitalocean.com/products/kubernetes), and many more throughout the Kubernetes Cloud Native ecosystem.
- _Kubernetes Cluster Infrastructure_
  - When we refer to _Kubernetes Cluster Infrastructure_ we aim to distinguish required environmental infrastructure (e.g., cloud virtual networks) in which a Kubernetes cluster resides as a "set of child resources" from the Kubernetes cluster resources themselves (e.g., virtual machines that underlie nodes, managed by Cluster API). Sometimes this is referred to as "BYO Infrastructure"; essentially, we are talking about **infrastructure that supports a Kubernetes cluster, but is not actively managed by Cluster API**. As we will see, this boundary is different when discussing Managed Kubernetes: more infrastructure resources are not managed by Cluster API when running Managed Kubernetes.
- e.g.
  - This just means "For example:"!

## Summary

We propose to make provider `<Infra>Cluster` resources optional in order to better represent Managed Kubernetes scenarios where all _Kubernetes Cluster Infrastructure_ is managed by the service provider, and not by Cluster API. In order to support that, we propose that the API Server endpoint reference can also originate from the `<Infra>ControlPlane` resource, and not the `<Infra>Cluster` resource. These changes will introduce two new possible implementation options for providers implementing Managed Kubernetes in Cluster API:

1. A Managed Kubernetes cluster solution whose configuration surface area is expressed exclusively in a `<Infra>ControlPlane` resource (no `<Infra>Cluster` resource).
2. A Managed Kubernetes cluster solution whose configuration surface area comprises both a `<Infra>Cluster` and a `<Infra>ControlPlane` resource, with `<Infra>ControlPlane` being solely responsible for configuring the API Server endpoint (instead of the API Server endpoint being configured via the `<Infra>Cluster`).

## Motivation

The implementation of Managed Kubernetes scenarios by Cluster API providers occurred after the architectural design of Cluster API, and thus that design process did not consider these Managed Kubernetes scenarios as a user story. In practice, Cluster API's specification has allowed Managed Kubernetes solutions to emerge that aid running fleets of clusters at scale, with CAPA's `AWSManagedCluster` and `AzureManagedCluster` being notable examples. However, because these Managed Kubernetes solutions arrived after the Cluster API contract was defined, providers have not settled on a consistent rendering of how a "Service-Managed Kubernetes" specification fits into a "Cluster API-Managed Kubernetes" surface area.

One particular part of the existing Cluster API surface area that is inconsistent with most Managed Kubernetes user experiences is the accounting of the [Kubernetes API server](https://kubernetes.io/docs/concepts/overview/components/#kube-apiserver). In the canonical "self-managed" user story that Cluster API addresses, it is the provider implementation of Cluster API (e.g., CAPA) that is responsible for scaffolding the necessary _Kubernetes Cluster Infrastructure_ that is required in order to create the Kubernetes API server (e.g., a Load Balancer and a public IP address). This provider responsibility is declared in the `<Infra>Cluster` resource, and carried out via its controllers; and then finally this reconciliation is synchronized with the parent `Cluster` Cluster API resource.

Because there exist Managed Kubernetes scenarios that handle all _Kubernetes Cluster Infrastructure_ responsibilities themselves, Cluster API's requirement of a `<Infra>Cluster` resource leads to weird implementation decisions, because in these scenarios there is no actual work for a Cluster API provider to do to scaffold _Kubernetes Cluster Infrastructure_.

### Goals

- Build upon [the existing Cluster API Managed Kubernetes proposal](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220725-managed-kubernetes.md). Any net new recommendations and/or proposals will be a continuation of the existing proposal, and consistent with its original conclusions.
- Make `<Infra>Cluster` resources optional.
- Enable API Server endpoint reporting from a provider's Control Plane resource rather than from its `<Infra>Cluster` resource.
- Ensure any changes to the current behavioral contract are backwards-compatible.

### Non-Goals

- Changes to existing Cluster API CRDs.
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

As a Cluster API provider implementor, I want to be able to return the control plane endpoint via the ControlPlane custom resource, so that it fits naturally with how I create an instance of the service provider's Managed Kubernetes which creates the endpoint, and so i don't have to pass through the value via another custom resource.

#### Story 4

As a Cluster API provider developer, I want guidance on how to incorporate a managed Kubernetes service into my provider, so that its usage is compatible with Cluster API architecture/features and its usage is consistant with other providers.

#### Story 5

As a Cluster API provider developer, I want to enable the ClusterClass feature for a Managed Kubernetes service, so that users can take advantage of an improved UX with ClusterClass-based clusters.

#### Story 6

As a cluster operator, I want to use Cluster API to provision and manage the lifecycle of worker nodes that utilizes my cloud providers' managed instances (if they support them), so that I don't have to worry about the management of these instances.

#### Story 7

As a service provider I want to be able to offer Managed Kubernetes clusters by using CAPI referencing my own managed control plane implementation that satisfies Cluster API contracts.

### Current State of Managed Kubernetes in CAPI

#### EKS in CAPA

- [Docs](https://cluster-api-aws.sigs.k8s.io/topics/eks/index.html)
- Feature Status: GA
- CRDs
  - AWSManagedCluster - passthrough kind to fullfill the capi contract
  - AWSManagedControlPlane - provision EKS cluster
  - AWSManagedMachinePool - corresponds to EKS managed node pool
- Supported Flavors
  - AWSManagedControlPlane with MachineDeployment / AWSMachine
  - AWSManagedControlPlane with MachinePool / AWSMachinePool
  - AWSManagedControlPlane with MachinePool / AWSManagedMachinePool
- Bootstrap Provider
  - Cluster API bootstrap provider EKS (CABPE)
- Features
  - Provisioning/managing an Amazon EKS Cluster
  - Upgrading the Kubernetes version of the EKS Cluster
  - Attaching self-managed machines as nodes to the EKS cluster
  - Creating a machine pool and attaching it to the EKS cluster (experimental)
  - Creating a managed machine pool and attaching it to the EKS cluster
  - Managing "EKS Addons"
  - Creating an EKS Fargate profile (experimental)
  - Managing aws-iam-authenticator configuration

#### AKS in CAPZ

- [Docs](https://capz.sigs.k8s.io/topics/managedcluster.html)
- Feature Status: GA
- CRDs
  - AzureManagedControlPlane, AzureManagedCluster - provision AKS cluster
  - AzureManagedMachinePool - corresponds to AKS node pool
- Supported Flavor
  - AzureManagedControlPlane + AzureManagedCluster with AzureManagedMachinePool

#### GKE in CAPG

- [Docs](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/v1.3.0/docs/book/src/topics/gke/index.md)
- Feature Status: Experimental ()
- CRDs
  - GCPManagedControlPlane, GCPManagedCluster - provision GKE cluster
  - GCPManagedMachinePool - corresponds to the managed node pool for the cluster
- Supported Flavor
  - GCPManagedControlPlane + GCPManagedCluster with GCPManagedMachinePool


#### Learnings from original Proposal: Two kinds with a Managed Control Plane & Managed Infra Cluster adhering to the current CAPI contracts

The original Managed Kubernetes proposal recommends managing two separate resources for cluster and control plane configuration, what we're referring to as a `<Infra>Cluster` and a `<Infra>ControlPlane`. That recommendation is outlined as [Option 3 in the proposal, here][managedKubernetesRecommendation]. This recommendation has been followed by CAPOCI and CAPG as of this writing.

This propsal was able to be implemented with no upstream changes in CAPI. It makes the following assumptions about representing Managed Kubernetes:

- **`<Infra>Cluster`** - Provides any base infrastructure that is required as a prerequisite for the target environment required for running machines and creating a Managed Kubernetes service.
- **`<Infra>ControlPlane`** - Represents an instance of the actual Managed Kubernetes service in the target environment (i.e. cloud/service provider). It’s based on the assumption that a Managed Kubernetes service supplies the Kubernetes control plane.

These broadly follow the existing separation within CAPI.

However, for many Managed Kubernetes services this will require less than ideal code in the controllers to retrieve the control plane endpoint from the `<Infra>ControlPlane` kind and report it back via the ControlPlaneEndpoint property on the `<Infra>Cluster` to satisfy CAPI contracts.

To give an idea what this means:
- `<Infra>Cluster` watches the control plane and vice versa
- `<Infra>Cluster` controller create base infra and sets Ready = true
- `<Infra>ControlPlane` waits for `<Infra>Cluster` to be Ready
- `<Infra>ControlPlane` creates an instance of the managed k8s service
- `<Infra>ControlPlane` gets the API server endpoint from the managed k8s service and stores it in the CRD instance
- `<Infra>Cluster` is watching for changes to `<Infra>ControlPlane` and if the "api server endpoint" on the `<Infra>ControlPlane` CRD instance is not empty then:
  - Map `<Infra>ControlPlane` to `<Infra>Cluster` and queue event
  - `<Infra>Cluster` reconciler loop gets the `<Infra>ControlPlane` CRD instance and takes the value for "api server endpoint" and populates `ControlPlaneEndpoint` on the `<Infra>Cluster` CRD instance.
  - (which will then cause the reconciler for `<Infra>ControlPlane` to run... again)

The implementation of the controllers for Managed Kubernetes would be simplified if there was an option to report the ControlPlaneEndpoint via `<Infra>ControlPlane` instead. Below we will outline two new flows that reduce much of the complexity of the above, while allowing Managed Kubernetes providers to represent their services intuitively.

### Two New Flows

#### Flow 1: `<Infra>Cluster` and `<Infra>ControlPlane`, with `ControlPlaneEndpoint` reported via `<Infra>ControlPlane`

We will describe a CRD composition that adheres to the original separation of concerns of the different provider types as documented in the Cluster API documentation, with a different API Server endpoint reporting flow.

As described above, at present the control plane endpoint must be returned via the `ControlPlaneEndpoint` field on the spec of the `<Infra>Cluster` [reference here](https://cluster-api.sigs.k8s.io/developer/providers/cluster-infrastructure.html). This is OK for self-managed clusters, as a load balancer is usually created as part of the reconciliation. But with Managed Kubernetes services the API Server endpoint usually comes from the service directly, which means that the `<Infra>Cluster` has to get the `ControlPlaneEndpoint` from the managed service so that it can be reported back to CAPI. In practice, this results in `<Infra>Cluster` watching the `<Infra>ControlPlane` and the `<Infra>ControlPlane` watching the `<Infra>Cluster`, and without care this can cause event storms in the CAPI management cluster.

This flow would require making changes to CAPI controllers so that there is an option to report the `ControlPlaneEndpoint` via the `<Infra>ControlPlane` as an alternative to coming from the `<Infra>Cluster`.

Using CAPG as an example:

```go
type GCPManagedControlPlaneSpec struct {
  // AddonsConfig defines the addons to enable with the GKE cluster.
  // +optional
  AddonsConfig *AddonsConfig `json:"addonsConfig,omitempty"`

  // Logging contains the logging configuration for the GKE cluster.
  // +optional
  Logging *ControlPlaneLoggingSpec `json:"logging,omitempty"`

  // EnableKubernetesAlpha will indicate the kubernetes alpha features are enabled
  // +optional
  EnableKubernetesAlpha bool

  // ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
  // +optional
  ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`

  ...
}


type GCPManagedClusterSpec struct {
  // Project is the name of the project to deploy the cluster to.
  Project string `json:"project"`

  // The GCP Region the cluster lives in.
  Region string `json:"region"`

  // NetworkSpec encapsulates all things related to the GCP network.
  // +optional
  Network NetworkSpec `json:"network"`

  // FailureDomains is an optional field which is used to assign selected availability zones to a cluster
  // FailureDomains if empty, defaults to all the zones in the selected region and if specified would override
  // the default zones.
  // +optional
  FailureDomains []string `json:"failureDomains,omitempty"`

  ...
}
```

**Pros**

- Simplifies provider implementation when reporting `ControlPlaneEndpoint`
- Clearer separation between the lifecycle management of the general cloud infrastructure required for the cluster and the actual managed control plane (GKE in this example)
- Follows the original intentions of an "infrastructure" and "control-plane" provider
- Enables removal/addition of properties for a Managed Kubernetes cluster that may be different from a self-managed Kubernetes cluster
- Works with ClusterClass

**Cons**

- Requires changes upstream to CAPI controllers to support the change of reporting `ControlPlaneEndpoint`
- Duplication of API definitions between self-managed and managed `<Infra>Cluster` definitions and related controllers
- Users need to be aware of when to use the unmanaged or managed `<Infra>Cluster` definitions.

#### Flow 2: Change CAPI to make `<Infra>Cluster` optional

This option follows along from the first flow above (`ControlPlaneEndpoint` reported by `<Infra>ControlPlane` resource rather than `<Infra>Cluster` resource), but takes it further and makes the `<Infra>Cluster` resource optional.

This option would allow providers to implement only a `<Infra>ControlPlane` resource. Using CAPG as an example, rather than:

- `Cluster` ←→ `GCPManagedCluster` + `GCPManagedControlPlane`

We would enable:

- `Cluster` ←→ `GCPManagedControlPlane`

This would have the advantage of imposing a separation of configuration between each provider’s `<Infra>Cluster` and `<Infra>ControlPlane`’s resources. Because our observations have been that various Managed Kubernetes service providers do things a little bit differently, this separation is hard to define and enforce across all providers in a way that is agreeable to each provider.

In practice this will help Managed Kubernetes provider implementations that do not provide infrastructure resources as part of the service contract, and as of now are required to implement a `<Infra>Cluster` resource (e.g., `AzureManagedCluster` ) as a sort of proxy resource that exists solely to fulfill the CAPI requirement for an `<Infra>Cluster` partner of its corresponding Cluster resource even though there is no infrastructure to describe:

```golang
type ClusterSpec struct {
    ...
    // InfrastructureRef is a reference to a provider-specific resource that holds the details
    // for provisioning infrastructure for a cluster in said provider.
    // +optional
    InfrastructureRef *corev1.ObjectReference `json:"infrastructureRef,omitempty"`
    ...
}
```

The above API specification snippet for `ClusterSpec` emphasizes (in the type comment) that in fact the `InfrastructureRef` child property is an optional property of the data model. We are able to take advantage of this data specification to accommodate these non-infrastructure-providing Managed Cluster infrastructure scenarios, and are entirely able to be represented as a “managed control plane” abstraction. Work will need to be done in the CAPI controllers to support this new workflow, which was originally implemented prior to Managed Kubernetes scenarios being considered.

**Pros**

- Does not require any change to existing Cluster API CRDs
- Flexible: enables more expressive API semantics for the various scenarios of Managed Kubernetes
- Is a natural evolution of the prior effort to standardize Managed Kubernetes on CAPI, doesn’t require users following this effort to entirely rethink how they can invest in CAPI + Managed Kubernetes

**Cons**

- Would require an update to the existing Cluster API contract to accommodate new workflows

#### Alternative Option: Introduce a new Managed Kubernetes provider type (with contract)

This option would introduce a new native Managed Kubernetes type definition into Cluster API, which would have the result of standardizing what Managed Kubernetes looks like for all providers under a common interface. We can use the CAPI type definition of “Cluster”, and the various provider implementations of that (e.g., `GCPCluster`) as a model to copy when we design a native Managed Kubernetes specification.

Defining a new CAPI Managed Kubernetes type would require us to discover and standardize the set of "common" (relevant across all providers) specification data into a new set of CAPI types, e.g.:

```golang
type ManagedCluster struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   ManagedClusterSpec   `json:"spec,omitempty"`
    Status ManagedClusterStatus `json:"status,omitempty"`
}

type ManagedClusterSpec struct {
    // Cluster network configuration.
    // +optional
    ClusterNetwork *ClusterNetwork `json:"clusterNetwork,omitempty"`

    // ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
    // +optional
    ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

    // InfrastructureRef is a reference to a provider-specific resource that holds the details
    // for provisioning infrastructure for a cluster in said provider.
    // +optional
    InfrastructureRef *corev1.ObjectReference `json:"infrastructureRef,omitempty"`
}
```

Each provider would then implement its own corresponding type definition:

```golang
type GCPManagedCluster struct {
   ....
}
```

Our job is to balance the beneficial outcomes of standardization and consistency by strictly defining certain "common" properties that each provider will fulfill, while enabling enough flexibility to allow providers to meaningfully represent their particular environments.

**Pros**

- Standardizing the spec at the foundational, Cluster API layer optimizes for consistency across providers

**Cons**

- Would require a new set of resource specifications to the existing Cluster API spec
- Differentiates "self-managed clusters" from "managed clusters" at the foundational API layer:
  - Self-managed clusters would use the `Cluster` API resource as the top-level primitive object
  - Managed clusters would use the `ManagedCluster` API resource as the top-level primitive object
  - For example, to see all clusters under management at present, you can issue a `kubectl get clusters --all-namespaces` command (or the API equivalent); going forward, you would issue `kubectl get clusters,managedclusters --all-namespaces`
- There are no existing provider implementations. All existing provider implementations (e.g., CAPA, CAPZ, CAPOCI, CAPG) would need to be replaced or augmented in order to use a new spec.

## Recommendations

Because Managed Kubernetes was not yet in scope for Cluster API when it first appeared and gained rapid adoption, we are incentivized for paths forward that use the existing, mature, widely used API specification. The option to create a new `ManagedCluster` API type to best enforce provider consistency thusly has a high bar to clear in order to justify itself as the best option for the next phase of Managed Kubernetes in Cluster API.

We conclude that enabling the the CAPI controllers to source authoritative `ControlPlaneEndpoint` data from the `<Infra>ControlPlane` resource is non-invasive to existing API contracts, and offers non-trivial flexibility for CAPI Managed Kubernetes providers at a small additional cost to CAPI maintenance going forward. Existing implementations that leverage a "proxy" `<Infra>Cluster` resource merely to satisfy CAPI contracts can be simplified by dropping the `<Infra>Cluster` resource altogether at little-to-no cost to their existing user communities. New Managed Kubernetes provider implementations will now have a little more flexibility to use a implementation that uses only a `<Infra>ControlPlane` resource, if that is appropriate, or for implementations that define both a `<Infra>Cluster` + `<Infra>ControlPlane` with the appropriate configuration distribution [following our recommendation][managedKubernetesRecommendation], those implementations can be non-trivially simplified with their `ControlPlaneEndpoint` data being observed and straightforwardly returned via `<Infra>ControlPlane`, the most common source of truth for a Managed Kubernetes service.

## Implementation History

- [x] 01/11/2023: Compile a Google Doc to organize thoughts prior to CAEP [link here](https://docs.google.com/document/d/1rqzZfsO6k_RmOHUxx47cALSr_6SeTG89e9C44-oHHdQ/)

[managedKubernetesRecommendation]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220725-managed-kubernetes.md#option-3-two-kinds-with-a-managed-control-plane-and-managed-infra-cluster-with-better-separation-of-responsibilities
