---
title: Managed Kubernetes in CAPI
authors:
  - “@pydctw”
  - "@richardcase"
reviewers:
  - “@alexeldeib”
  - “@CecileRobertMichon”
  - “@enxebre”
  - “@fabriziopandini”
  - “@jackfrancis”
  - "@joekr"
  - “@sbueringer”
  - "@shyamradhakrishnan"
  - "@yastij"
creation-date: 2022-07-25
last-updated: 2022-08-23
status: implementable
see-also:
replaces:
superseded-by:
---

# Managed Kubernetes in CAPI

## Table of Contents

- [Managed Kubernetes in CAPI](#managed-kubernetes-in-capi)
  - [Table of Contents](#table-of-contents)
  - [Glossary](#glossary)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [Personas](#personas)
      - [Cluster Service Provider](#cluster-service-provider)
      - [Cluster Service Consumer](#cluster-service-consumer)
      - [Cluster Admin](#cluster-admin)
      - [Cluster User](#cluster-user)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
      - [Story 3](#story-3)
      - [Story 4](#story-4)
      - [Story 5](#story-5)
      - [Story 6](#story-6)
    - [Current State of Managed Kubernetes in CAPI](#current-state-of-managed-kubernetes-in-capi)
      - [EKS in CAPA](#eks-in-capa)
      - [AKS in CAPZ](#aks-in-capz)
      - [OKE in CAPOCI](#oke-in-capoci)
    - [Managed Kubernetes API Design Approaches](#managed-kubernetes-api-design-approaches)
      - [Option 1: Single kind for Control Plane and Infrastructure](#option-1-single-kind-for-control-plane-and-infrastructure)
        - [Background: Why did EKS in CAPA choose this option?](#background-why-did-eks-in-capa-choose-this-option)
      - [Option 2: Two kinds with a ControlPlane and a pass-through InfraCluster](#option-2-two-kinds-with-a-controlplane-and-a-pass-through-infracluster)
      - [Option 3: Two kinds with a Managed Control Plane and Managed Infra Cluster with Better Separation of Responsibilities](#option-3-two-kinds-with-a-managed-control-plane-and-managed-infra-cluster-with-better-separation-of-responsibilities)
      - [Option 4: Two kinds with a Managed Control Plane and Shared Infra Cluster with Better Separation of Responsibilities](#option-4-two-kinds-with-a-managed-control-plane-and-shared-infra-cluster-with-better-separation-of-responsibilities)
  - [Recommendations](#recommendations)
    - [Additional notes on option 3](#additional-notes-on-option-3)
    - [Managed Node Groups for Worker Nodes](#managed-node-groups-for-worker-nodes)
    - [Provider Implementers Documentation](#provider-implementers-documentation)
  - [Other Considerations for CAPI](#other-considerations-for-capi)
    - [ClusterClass support for MachinePool](#clusterclass-support-for-machinepool)
    - [clusterctl integration](#clusterctl-integration)
    - [Add-ons management](#add-ons-management)
  - [Upgrade Strategy](#upgrade-strategy)
  - [Implementation History](#implementation-history)
  
## Glossary

- **Managed Kubernetes** - a Kubernetes service offered/hosted by a service provider where the control plane is run & managed by the service provider. As a cluster service consumer, you don’t have to worry about managing/operating the control plane machines. Additionally, the managed Kubernetes service may extend to cover running managed worker nodes. Examples are EKS in AWS and AKS in Azure. This is different from a traditional implementation in Cluster API, where the control plane and worker nodes are deployed and managed by the cluster admin.
- **Unmanaged Kubernetes** - a Kubernetes cluster where a cluster admin is responsible for provisioning and operating the control plane and worker nodes. In Cluster API this traditionally means a Kubeadm bootstrapped cluster on infrastructure machines (virtual or physical).
- **Managed Worker Node** - an individual Kubernetes worker node where the underlying compute (vm or bare-metal) is provisioned and managed by the service provider. This usually includes the joining of the newly provisioned node into a Managed Kubernetes cluster. The lifecycle is normally controlled via a higher level construct such as a Managed Node Group.
- **Managed Node Group** - is a service that a service provider offers that automates the provisioning of managed worker nodes. Depending on the service provider this group of nodes could contain a fixed number of replicas or it might contain a dynamic pool of replicas that auto-scales up and down. Examples are Node Pools in GCP and EKS managed node groups.  
- **Cluster Infrastructure Provider (Infrastructure)** - an Infrastructure provider supplies whatever prerequisites are necessary for creating & running clusters such as networking, load balancers, firewall rules, and so on. ([docs](../book/src/developer/providers/cluster-infrastructure.md))
- **ControlPlane Provider (ControlPlane)** - a control plane provider instantiates a Kubernetes control plane consisting of k8s control plane components such as kube-apiserver, etcd, kube-scheduler and kube-controller-manager. ([docs](../book/src/developer/architecture/controllers/control-plane.md#control-plane-provider))
- **MachineDeployment** - a MachineDeployment orchestrates deployments over a fleet of MachineSets, which is an immutable abstraction over Machines. ([docs](../book/src/developer/architecture/controllers/machine-deployment.md))
- **MachinePool (experimental)** - a MachinePool is similar to a MachineDeployment in that they both define configuration and policy for how a set of machines are managed. While the MachineDeployment uses MachineSets to orchestrate updates to the Machines, MachinePool delegates the responsibility to a cloud provider specific resource such as AWS Auto Scale Groups, GCP Managed Instance Groups, and Azure Virtual Machine Scale Sets. ([docs](./20190919-machinepool-api.md))

## Summary

This proposal discusses various options on how a managed Kubernetes services could be represented in Cluster API by providers. Recommendations will be made on which approach(s) to adopt for new implementations by providers with a view of eventually having consistency across provider implementations.

## Motivation

Cluster API was originally designed with unmanaged Kubernetes clusters in mind as the cloud providers did not offer managed Kubernetes services (except GCP with GKE). However, all 3 main cloud providers (and many other cloud/service providers) now have managed Kubernetes services.

Some Cluster API Providers (i.e. Azure with AKS first and then AWS with EKS) have implemented support for their managed Kubernetes services. These implementations have followed the existing documentation & contracts (that were designed for unmanaged Kubernetes) and have ended up with 2 different implementations.

While working on supporting ClusterClass for EKS in Cluster API Provider AWS (CAPA), it was discovered that the current implementation of EKS within CAPA, where a single resource kind (AWSManagedControlPlane) is used for both ControlPlane and Infrastructure, is incompatible with ClusterClass (See the [issue](https://github.com/kubernetes-sigs/cluster-api/issues/6126)). Separation of ControlPlane and Infrastructure is expected for the ClusterClass implementation to work correctly.

The responsibilities between the CAPI control plane and infrastructure are blurred with a managed Kubernetes service like AKS or EKS. For example, when you create a EKS control plane in AWS it also creates infrastructure that CAPI would traditionally view as the responsibility of the cluster “infrastructure provider”.

A good example here is the API server load balancer:

- Currently CAPI expects the control plane load balancer to be created by the cluster infrastructure provider and for its endpoint to be returned via the `ControlPlaneEndpoint` on the InfraCluster.
- In AWS when creating an EKS cluster (which is the Kubernetes control plane), a load balancer is automatically created for you. When representing EKS in CAPI this naturally maps to a “control plane provider” but this causes complications as we need to report the endpoint back via the cluster infrastructure provider and not the control plane provider.

### Goals

- Provide a recommendation of a consistent approach for representing Managed Kubernetes services in CAPI for new implementations.
  - It would be ideal for there to be consistency between providers when it comes to representing Managed Kubernetes services. However, it's unrealistic to ask providers to refactor their existing implementations.
- Ensure the recommendation provides a working model for Managed Kubernetes integration with ClusterClass.
- As a result of the recommendations of this proposal we should update the [Provider Implementers](../book/src/developer/providers/implementers.md) documentation to aid with future provider implementations.

### Non-Goals/Future Work

- Enforce the Managed Kubernetes recommendations as a requirement for Cluster API providers when they implement Managed Kubernetes.
  - If providers that have already implemented Managed Kubernetes and would like guidance on if/how they could move to be aligned with the recommendations of this proposal then discussions should be facilitated.
- Provide advice in this proposal on how to refactor the existing implementations of managed Kubernetes in CAPA & CAPZ.
- Propose a new architecture or API changes to CAPI for managed Kubernetes
- Be a concrete design for the GKE implementation in Cluster API Provider GCP (CAPG).
  - A separate CAPG proposal will be created for GKE implementation based on the recommendations of this proposal.
- Recommend how Managed Kubernetes services would leverage CAPI internally to run their offer.

## Proposal

### Personas

#### Cluster Service Provider

The user hosting cluster control planes, responsible for up-time, UI for fleet wide alerts, configuring a cloud account to host control planes in, views user provisioned infra (available compute). Has cluster admin management.

#### Cluster Service Consumer

A user empowered to request control planes, request workers to a service provider, and drive upgrades or modify externalized configuration.

#### Cluster Admin

A user with cluster-admin role in the provisioned cluster, but may or may not have power over when/how cluster is upgraded or configured.

#### Cluster User

A user who uses a provisioned cluster, which usually maps to a developer.

### User Stories

#### Story 1

As a cluster service consumer,
I want to use Cluster API to provision and manage Kubernetes Clusters that utilize my service providers Managed Kubernetes Service (i.e. EKS, AKS, GKE),
So that I don’t have to worry about the management/provisioning of control plane nodes, and so I can take advantage of any value add services offered by the service provider.

#### Story 2

As a cluster service consumer,
I want to use Cluster API to provision and manage the lifecycle of worker nodes that utilizes my cloud providers’ managed instances (if they support them),
So that I don't have to worry about the management of these instances.

#### Story 3

As a cluster admin,
I want to be able to provision both “unmanaged” and “managed” Kubernetes clusters from the same management cluster,
So that I can support different requirements & use cases as needed whilst using a single operating model.

#### Story 4

As a Cluster API provider developer,
I want guidance on how to incorporate a managed Kubernetes service into my provider,
So that its usage is compatible with Cluster API architecture/features
And its usage is consistent with other providers.

#### Story 5

As a Cluster API provider developer,
I want to enable the ClusterClass feature for a managed Kubernetes service,
So that cluster users can take advantage of an improved UX with ClusterClass-based clusters.

#### Story 6

As a cluster service provider,
I want to be able to offer “Managed Kubernetes” powered by CAPI,
So that I can eliminate the responsibility of owning and SREing the Control Plane from the Cluster service consumer and cluster admin.

### Current State of Managed Kubernetes in CAPI

#### EKS in CAPA

- [Docs](https://cluster-api-aws.sigs.k8s.io/topics/eks/index.html)
- Feature Status: GA
- CRDs
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
  - Managing “EKS Addons”
  - Creating an EKS Fargate profile (experimental)
  - Managing aws-iam-authenticator configuration

#### AKS in CAPZ

- [Docs](https://capz.sigs.k8s.io/topics/managedcluster.html)
- Feature Status: Experimental
- CRDs
  - AzureManagedControlPlane, AzureManagedCluster - provision AKS cluster
  - AzureManagedMachinePool - corresponds to AKS node pool
- Supported Flavor
  - AzureManagedControlPlane + AzureManagedCluster with AzureManagedMachinePool

#### OKE in CAPOCI

- [Issue](https://github.com/oracle/cluster-api-provider-oci/issues/110)
- Design discussion starting

### Managed Kubernetes API Design Approaches

When discussing the different approaches to represent a managed Kubernetes service in CAPI, we will be using the implementation of GKE support in CAPG as an example, as this isn’t currently implemented.

> NOTE: “naming things is hard” so the names of the kinds/structs/fields used in the CAPG examples below are illustrative only and are not the focus of this proposal. There is debate, for example, as to whether `GCPManagedCluster` or `GKECluster` should be used. This type of discussion will be within the CAPG proposal.

The following section discusses different API implementation options along with pros and cons of each.

#### Option 1: Single kind for Control Plane and Infrastructure

This option introduces a new single resource kind:

- **GCPManagedControlPlane**: this represents both a control-plane (i.e. GKE) and infrastructure required for the cluster. It contains properties for both the general cloud infrastructure (that would traditionally be represented by an infrastructure cluster) and the managed Kubernetes control plane (that would traditionally be represented by a control plane provider).

```go
type GCPManagedControlPlaneSpec struct {
    // Project is the name of the project to deploy the cluster to.
    Project string `json:"project"`

    // NetworkSpec encapsulates all things related to the GCP network.
    // +optional
    Network NetworkSpec `json:"network"`

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
    ....
}
```

**This is the design pattern used by EKS in CAPA.**

##### Background: Why did EKS in CAPA choose this option?

CAPA decided to represent an EKS cluster as a CAPI control-plane. This meant that control-plane is responsible for creating the API server load balancer.

Initially CAPA had an infrastructure cluster kind that reported back the control plane endpoint. This required less than ideal code in its controller to watch the control plane and use its value of the control plane endpoint.

As the infrastructure cluster kind only acted as a passthrough (to satisfy the contract with CAPI) it was decided that it would be removed and the control-plane kind (AWSManagedControlPlane) could be used to satisfy both the “infrastructure” and “control-plane” contracts. This worked well until ClusterClass arrived with its expectation that the “infrastructure” and “control-plane” are 2 different resource kinds.

Note that CAPZ had a similar discussion and an [issue](https://github.com/kubernetes-sigs/cluster-api-provider-azure/issues/1396) to remove AzureManagedCluster: AzureManagedCluster is useless; let's remove it (and keep AzureManagedControlPlane)

**Pros**

- A simple design with a single resource kind and controller.

**Cons**

- Doesn’t work with the current implementation of ClusterClass, which expects a separation of ControlPlane and Infrastructure.
- Doesn’t provide separation of responsibilities between creating the general cloud infrastructure for the cluster and the actual cluster control plane.
- Managed Kubernetes look different from unmanaged Kubernetes where two separate kinds are used for a control plane and infrastructure. This would impact products building on top of CAPI.

#### Option 2: Two kinds with a ControlPlane and a pass-through InfraCluster

This option introduces 2 new resource kinds:

- **GCPManagedControlPlane**: same as in option 1
- **GCPManagedCluster**: contains the minimum properties in its spec and status to satisfy the [CAPI contract for an infrastructure cluster](../book/src/developer/providers/cluster-infrastructure.md) (i.e. ControlPlaneEndpoint, Ready condition). Its controller watches GCPManagedControlPlane and copies the ControlPlaneEndpoint field to GCPManagedCluster to report back to CAPI. This is used as a pass-through layer only.

```go
type GCPManagedControlPlaneSpec struct {
    // Project is the name of the project to deploy the cluster to.
    Project string `json:"project"`

    // NetworkSpec encapsulates all things related to the GCP network.
    // +optional
    Network NetworkSpec `json:"network"`

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
    ....
}
```

```go
type GCPManagedClusterSpec struct {
    // ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
    // +optional
    ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`
}
```

**This is the design pattern used by AKS in CAPZ**. [An example of how ManagedCluster watches ControlPlane in CAPZ.](https://github.com/kubernetes-sigs/cluster-api-provider-azure/blob/main/exp/controllers/azuremanagedcluster_controller.go#L165)

**Pros**

- Better aligned with CAPI’s traditional infra provider model
- Works with ClusterClass

**Cons**

- Need to maintain Infra cluster kind, which is a pass-through layer and has no other functions. In addition to the CRD, controllers, webhooks and conversions webhooks need to be maintained.
- Infra provider doesn’t provision infrastructure and whilst it may meet the CAPI contract, it doesn’t actually create infrastructure as this is done via the control plane.

#### Option 3: Two kinds with a Managed Control Plane and Managed Infra Cluster with Better Separation of Responsibilities

This option more closely follows the original separation of concerns with the different CAPI provider types. With this option, 2 new resource kinds will be introduced:

- **GCPManagedControlPlane**: this presents the actual GKE control plane in GCP. Its spec would only contain properties that are specific to the provisioning & management of a GKE cluster in GCP (excluding worker nodes). It would not contain any properties related to the general GCP operating infrastructure, like the networking or project.
- **GCPManagedCluster**: this presents the properties needed to provision and manage the general GCP operating infrastructure for the cluster (i.e project, networking, iam). It would contain similar properties to **GCPCluster** and its reconciliation would be very similar.

```go
type GCPManagedControlPlaneSpec struct {
    // ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
    // +optional
    ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

    // AddonsConfig defines the addons to enable with the GKE cluster.
    // +optional
    AddonsConfig *AddonsConfig `json:"addonsConfig,omitempty"`

    // Logging contains the logging configuration for the GKE cluster.
    // +optional
    Logging *ControlPlaneLoggingSpec `json:"logging,omitempty"`

    // EnableKubernetesAlpha will indicate the kubernetes alpha features are enabled
    // +optional
    EnableKubernetesAlpha bool

    ...
}
```

```go
type GCPManagedClusterSpec struct {
    // Project is the name of the project to deploy the cluster to.
    Project string `json:"project"`

    // The GCP Region the cluster lives in.
    Region string `json:"region"`

    // ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
    // +optional
    ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`

    // NetworkSpec encapsulates all things related to the GCP network.
    // +optional
    Network NetworkSpec `json:"network"`

    // FailureDomains is an optional field which is used to assign selected availability zones to a cluster
    // FailureDomains if empty, defaults to all the zones in the selected region and if specified would override
    // the default zones.
    // +optional
    FailureDomains []string `json:"failureDomains,omitempty"`

    // AdditionalLabels is an optional set of tags to add to GCP resources managed by the GCP provider, in addition to the
    // ones added by default.
    // +optional
    AdditionalLabels Labels `json:"additionalLabels,omitempty"`

    ...
}
```

**Pros**

- Clearer separation between the lifecycle management of the general cloud infrastructure required for the cluster and the actual managed control plane  (i.e. GKE in this example)
- Follows the original intentions of an “infrastructure” and “control-plane” provider
- Enables removal/addition of properties for a managed kubernetes that may be different compared to an unmanaged kubernetes
- Works with ClusterClass

**Cons**

- Duplication of API definitions between GCPCluster and GCPManagedCluster and reconciliation for the infrastructure cluster

#### Option 4: Two kinds with a Managed Control Plane and Shared Infra Cluster with Better Separation of Responsibilities

This option is a variation of option 3 and as such it more closely follows the original separation of concerns with the different CAPI provider types. The difference with this option compared to option 3 is that only 1 new resource kind is introduced:

- **GCPManagedControlPlane**: this presents the actual GKE control plane in GCP. Its spec would only contain properties that are specific to provisioning & management of GKE. It would not contain any general properties related to the general GCP operating infrastructure, like the networking or project.

The general cluster infrastructure will be declared via the existing **GCPCluster** kind and reconciled via the existing controller.

However, this approach will require changes to the controller for **GCPCluster**. The steps to create the required infrastructure may be different between an unmanaged cluster and a GKE based cluster. For example, for an unmanaged cluster a load balancer will need to be created but with a GKE based cluster this won’t be needed and instead we’d need to use the endpoint created as part of **GCPManagedControlPlane** reconciliation.

So the **GCPCluster** controller will need to know if its creating infrastructure for an unmanaged or managed cluster (probably by looking at the parent's (i.e. `Cluster`) **controlPlaneRef**) and do different steps.

**Pros**

- Single infra cluster kind irrespective of if you are creating an unmanaged or GKE based cluster. It doesn’t require the user to pick the right one.
- Clear separation between cluster infrastructure and the actual managed (i.e. GKE) control plane
- Works with cluster class

**Cons**

- Additional complexity and logic in the infra cluster controller
- API definition could be messy if only certain fields are required for one type of cluster

## Recommendations

It is proposed that option 3 (two kinds with a managed control plane and managed infra cluster with better separation of responsibilities) is the best way to proceed for **new implementations** of managed Kubernetes in a provider.

The reasons for this recommendation are as follows:

- It adheres closely to the original separation of concerns between the infra and control plane providers
- The infra cluster provisions and manages the general infrastructure required for the cluster but not the control plane.
- By having a separate infra cluster API definition, it allows differences in the API between managed and unmanaged clusters.

Providers like CAPZ and CAPA have already implemented managed Kubernetes support and there should be no requirement on them to move to Option 3. Both Options 2 and 4 are solutions that would work with ClusterClass and so could be used if required.

Option 1 is the only option that will not work with ClusterClass and would require a change to CAPI. Therefore this option is not recommended.

*** This means that CAPA will have to make changes to move away from Option 1 if it wants to support ClusterClass.

### Additional notes on option 3

There are a number of cons listed for option 3. With having 2 API kinds for the infra cluster (and associated controllers), there is a risk of code duplication. To reduce this the 2 controllers can utilize shared reconciliation code from the different controllers so as to reduce this duplication.

The user will need to be aware of when to use which specific infra cluster kind. In our example this means that a user will need to know when to use `GCPCluster` vs `GCPManagedCluster`. To give clear guidance to users, we will provide templates (including ClusterClasses) and documentation for both the unmanaged and managed varieties of clusters. If we used the same infra cluster kind across both unmanaged & managed (i.e. option 4) then we run the risk of complicating the API for the infra cluster & controller if the required properties diverge.

### Managed Node Groups for Worker Nodes

Some cloud providers also offer Managed Node Groups as part of their Managed Kubernetes service as a way to provision worker nodes for a cluster. For example, in GCP there are Node Pools and in AWS there are EKS Managed Node Groups.

There are 2 different ways to represent a group of machines in CAPI:

- **MachineDeployments** - you specify the number of replicas of a machine template and CAPI will manage the creation of immutable Machine-Infrastructure Machine pairs via MachineSets. The user is responsible for explicitly declaring how many machines (a.k.a replicas) they want and these are provisioned and joined to the cluster.
- **MachinePools** - are similar to MachineDeployments in that they specify a number of machine replicas to be created and joined to the cluster. However, instead of using MachineSets to manage the lifecycle of individual machines a provider implementer utilizes a cloud provided solution to manage the lifecycle of the individual machines instead. Generally with a pool you don’t have to define an exact amount of replicas and instead you have the option to supply a minimum and maximum number of nodes and let the cloud service manage the scaling up and down the number of replicas/nodes. Examples of cloud provided solutions are Auto Scale Groups (ASG) in AWS and Virtual Machine Scale Sets (VMSS) in Azure.

With the implementation of a managed node group the cloud provider is responsible for managing the lifecycle of the individual machines that are used as nodes. This implies that a machine pool representation is needed which utilises a cloud provided solution to manage the lifecycle of machines.

For our example, GCP offers Node Pools that will manage the lifecycle of a pool of machines that can scale up and down. We can use this service to implement machine pools:

```go
type GCPManagedMachinePoolSpec struct {
    // Location specifies where the nodes should be created.
    Location []string `json:"location"`

    // The Kubernetes version for the node group.
    Version string `json:"version"`

    // MinNodeCount is the minimum number of nodes for one location.
    MinNodeCount int `json:"minNodeCount"`

    // MaxNodeCount is the maximum number of nodes for one location.
    MaxNodeCount int `json:"minNodeCount"`

    ...
}
```

### Provider Implementers Documentation

Its recommended that changes are made to the [Provider Implementers documentation](../book/src/developer/providers/cluster-infrastructure.md) based on the recommending approach for representing managed Kubernetes in Cluster API.

Some of the areas of change (this is not an exhaustive list):

- A new "implementing managed kubernetes" guide that contains details about how to represent a managed Kubernetes service in CAPI. The content will be based on option 3 from this proposal along with other considerations such as managed node and addon management.
- Update the [Provider contracts documentation](../book/src/developer/providers/contracts.md) to state that the same kind should not be used to satisfy 2 different provider contracts.
- Update the [Cluster Infrastructure documentation](../book/src/developer/providers/cluster-infrastructure.md) to provide guidance on how to populate the `controlPlaneEndpoint` in the scenario where the control plane creates the api server load balancer. We should include sample code.
- Update the [Control Plane Controller](../book/src/developer/architecture/controllers/control-plane.md) diagram for managed k8s services case. The Control Plane reconcile needs to start when `InfrastructureReady` is true.

## Other Considerations for CAPI

### ClusterClass support for MachinePool

- MachinePool is an important feature for managed Kubernetes as it is preferred over MachineDeployment to fully utilize the native capabilities such as autoscaling, health-checking, zone balancing provided by cloud providers’ node groups.
- AKS supports MachinePool based worker nodes only and ClusterClass support for MachinePool is required. See [issue](https://github.com/kubernetes-sigs/cluster-api/issues/5991)

### clusterctl integration

- `clusterctl` assumes a minimal set of providers (core, bootstrap, control plane, infra) is required to form a valid management cluster. Currently, it does not expect a single provider being many things at the same time.
- EKS in CAPA has its own control plane provider and a bootstrap provider packaged in a single manager. Moving forward, it would be great to separate them out.

### Add-ons management

- EKS and AKS provide an ability to install add-ons (e.g., CNI, CSI, DNS) managed by cloud providers.
  - [EKS add-ons](https://docs.aws.amazon.com/eks/latest/userguide/eks-add-ons.html)
  - [AKS add-ons](https://docs.microsoft.com/en-us/azure/aks/integrations)
- CAPA and CAPZ enabled support for cloud provider managed addons via API
  - [CAPA](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/main/controlplane/eks/api/v1beta1/awsmanagedcontrolplane_types.go#L155)
  - [CAPZ](https://github.com/kubernetes-sigs/cluster-api-provider-azure/pull/2095)
- Managed Kubernetes implementations should be able to opt-in/opt-out of what will be provided by [CAPI’s add-ons orchestration solution](https://github.com/kubernetes-sigs/cluster-api/issues/5491)

## Upgrade Strategy

As mentioned in the goals section, it is up to providers with existing implementations, CAPA and CAPZ, to decide how they want to proceed.

- EKS and AKS are in different lifecycle stages, EKS is in GA vs. AKS is in the experimental stage. This requires different considerations for making breaking changes to APIs.
- Their current design is different, EKS using option 1 while AKS using option 2. This makes it difficult to propose a single upgrade strategy.

## Implementation History

- [x] 03/01/2022: Had a community meeting to discuss an issue regarding ClusterClass support for EKS and managed k8s in CAPI
- [x] 03/17/2022: Compile a Google Doc following the CAEP template ([link](https://docs.google.com/document/d/1dMN4-KppBkA51sxXPSQhYpqETp2AG_kHzByXTmznxFA/edit?usp=sharing))
- [x] 04/20/2022: Present proposal at a community meeting
- [x] 07/27/2022: Move the proposal to a PR in CAPI repo
