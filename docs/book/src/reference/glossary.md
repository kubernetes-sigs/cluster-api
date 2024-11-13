# Table of Contents

[A](#a) | [B](#b) | [C](#c) | [D](#d) | [E](#e) | [H](#h) | [I](#i) | [K](#k) | [L](#l)| [M](#m) | [N](#n) | [O](#o) | [P](#p) | [R](#r) | [S](#s) | [T](#t) | [W](#w)

# A
---

### Add-ons

Services beyond the fundamental components of Kubernetes.

* __Core Add-ons__: Addons that are required to deploy a Kubernetes-conformant cluster: DNS, kube-proxy, CNI.
* __Additional Add-ons__: Addons that are not required for a Kubernetes-conformant cluster (e.g. metrics/Heapster, Dashboard).

# B
---

### Bootstrap

The process of turning a server into a Kubernetes node. This may involve assembling data to provide when creating the server that backs the Machine, as well as runtime configuration of the software running on that server.

### Bootstrap cluster

A temporary cluster that is used to provision a Target Management cluster.

### Bootstrap provider

Refers to a [provider](#provider) that implements a solution for the [bootstrap](#bootstrap) process.
Bootstrap provider's interaction with Cluster API is based on what is defined in the [Cluster API contract](#contract).

See [CABPK](#cabpk).

# C
---

### CAEP
Cluster API Enhancement Proposal - patterned after [KEP](https://git.k8s.io/enhancements/keps/README.md). See [template](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/YYYYMMDD-template.md)

### CAPI
[Core Cluster API](#core-cluster-api)

### CAPA
Cluster API Provider AWS

### CABPK
Cluster API Bootstrap Provider Kubeadm

### CABPOCNE
Cluster API Bootstrap Provider Oracle Cloud Native Environment (OCNE)

### CACPOCNE
Cluster API Control Plane Provider Oracle Cloud Native Environment (OCNE)

### CAPC
Cluster API Provider CloudStack

### CAPD
Cluster API Provider Docker

### CAPDO
Cluster API Provider DigitalOcean

### CAPG
Cluster API Google Cloud Provider

### CAPH
Cluster API Provider Hetzner

### CAPHV
Cluster API Provider Hivelocity

### CAPIBM
Cluster API Provider IBM Cloud

### CAPIM
Cluster API Provider In Memory

### CAPIO
Cluster API Operator

### CAPL
Cluster API Provider Akamai (Linode)

### CAPM3
Cluster API Provider Metal3

### CAPN
Cluster API Provider Nested

### CAPX
Cluster API Provider Nutanix

### CAPKK
Cluster API Provider KubeKey

### CAPK
Cluster API Provider Kubevirt

### CAPO
Cluster API Provider OpenStack

### CAPOSC
Cluster API Provider Outscale

### CAPOCI
Cluster API Provider Oracle Cloud Infrastructure (OCI)

### CAPT
Cluster API Provider Tinkerbell

### CAPV
Cluster API Provider vSphere

### CAPVC
Cluster API Provider vcluster

### CAPVCD
Cluster API Provider VMware Cloud Director

### CAPZ
Cluster API Provider Azure

### CAIPAMIC
Cluster API IPAM Provider In Cluster

### CAIPAMX
Cluster API IPAM Provider Nutanix

### CAREX
Cluster API Runtime Extensions Provider Nutanix

### Cloud provider

Or __Cloud service provider__

Refers to an information technology (IT) company that provides computing resources (e.g. AWS, Azure, Google, etc.).

### Cluster

A full Kubernetes deployment. See Management Cluster and Workload Cluster.

### ClusterClass

A collection of templates that define a topology (control plane and workers) to be used to continuously reconcile one or more Clusters.
See [ClusterClass](../tasks/experimental-features/cluster-class/index.md)

### Cluster API

Or __Cluster API project__

The Cluster API sub-project of the SIG-cluster-lifecycle. It is also used to refer to the software components, APIs, and community that produce them.

See [Core Cluster API](#core-cluster-api), [CAPI](#capi)

### Cluster API Runtime

The Cluster API execution model, a set of controllers cooperating in managing the Kubernetes cluster lifecycle.

### Cluster Infrastructure

or __Kubernetes Cluster Infrastructure__

Defines the **infrastructure that supports a Kubernetes cluster**, like e.g. VPC, security groups, load balancers, etc. Please note that in the context of managed Kubernetes some of those components are going to be provided by the corresponding abstraction for a specific Cloud provider (EKS, OKE, AKS etc), and thus Cluster API should not take care of managing a subset or all those components.

### Contract

Or __Cluster API contract__

Defines a set of rules a [provider](#provider) is expected to comply with in order to interact with Cluster API.
Those rules can be in the form of CustomResourceDefinition (CRD) fields and/or expected behaviors to be implemented.

### Control plane

The set of Kubernetes services that form the basis of a cluster. See also [https://kubernetes.io/docs/concepts/#kubernetes-control-plane](https://kubernetes.io/docs/concepts/#kubernetes-control-plane) There are two variants:

* __Self-provisioned__: A Kubernetes control plane consisting of pods or machines wholly managed by a single Cluster API deployment.
* __External__ or __Managed__: A control plane offered and controlled by some system other than Cluster API (e.g., GKE, AKS, EKS, IKS).

### Control plane provider

Refers to a [provider](#provider) that implements a solution for the management of a Kubernetes [control plane](#control-plane).
Control plane provider's interaction with Cluster API is based on what is defined in the [Cluster API contract](#contract).

See [KCP](#kcp).

### Core Cluster API

With "core" Cluster API we refer to the common set of API and controllers that are required to run
any Cluster API provider.

Please note that in the Cluster API code base, side by side of "core" Cluster API components there
is also a limited number of in-tree providers: [CABPK](#cabpk), [KCP](#kcp), [CAPD](#capd), [CAPIM](#capim)

See [Cluster API](#cluster-api), [CAPI](#capi).

### Core provider

Refers to a [provider](#provider) that implements Cluster API [core controllers](#core-controllers)

See [Cluster API](#cluster-api), [CAPI](#capi).

### Core controllers

The set of controllers in [Core Cluster API](#core-cluster-api).

See [Cluster API](#cluster-api), [CAPI](#capi).

# D
---

### Default implementation

A feature implementation offered as part of the Cluster API project and maintained by the CAPI core team; For example
[KCP](#kcp) is a default implementation for a [control plane provider](#control-plane-provider).

# E
---

### External patch

[Patch](#patch) generated by an external component using [Runtime SDK](#runtime-sdk). Alternative to [inline patch](#inline-patch).

### External patch extension

A [runtime extension](#runtime-extension) that implements a [topology mutation hook](#topology-mutation-hook).

# H
---

### Horizontal Scaling

The ability to add more machines based on policy and well-defined metrics. For example, add a machine to a cluster when CPU load average > (X) for a period of time (Y).

### Host

see [Server](#server)

# I
---

### Infrastructure provider

Refers to a [provider](#provider) that implements provisioning of infrastructure/computational resources required by
the Cluster or by Machines (e.g. VMs, networking, etc.).
Infrastructure provider's interaction with Cluster API is based on what is defined in the [Cluster API contract](#contract).

Clouds infrastructure providers include AWS, Azure, or Google; while VMware, MAAS, or metal3.io can be defined as bare metal providers.
When there is more than one way to obtain resources from the same infrastructure provider (e.g. EC2 vs. EKS in AWS) each way is referred to as a variant.

For a complete list of providers see [Provider Implementations](providers.md).

### Inline patch

A [patch](#patch) defined inline in a [ClusterClass](#clusterclass). An alternative to an [external patch](#external-patch).

### In-place mutable fields

Fields which changes would only impact Kubernetes objects or/and controller behaviour
but they won't mutate in any way provider infrastructure nor the software running on it. In-place mutable fields
are propagated in place by CAPI controllers to avoid the more elaborated mechanics of a replace rollout.
They include metadata, MinReadySeconds, NodeDrainTimeout, NodeVolumeDetachTimeout and NodeDeletionTimeout but are
not limited to be expanded in the future.

### Instance

see [Server](#server)

### Immutability

A resource that does not mutate.  In Kubernetes we often state the instance of a running pod is immutable or does not change once it is run.  In order to make a change, a new pod is run.  In the context of [Cluster API](#cluster-api) we often refer to a running instance of a [Machine](#machine) as being immutable, from a [Cluster API](#cluster-api) perspective.

### IPAM provider

Refers to a [provider](#provider) that allows Cluster API to interact with IPAM solutions.
IPAM provider's interaction with Cluster API is based on the `IPAddressClaim` and `IPAddress` API types.

# K
---

### Kubernetes-conformant

Or __Kubernetes-compliant__

A cluster that passes the Kubernetes conformance tests.

### k/k

Refers to the [main Kubernetes git repository](https://github.com/kubernetes/kubernetes) or the main Kubernetes project.

### KCP

Kubeadm Control plane Provider

# L
---

### Lifecycle hook
A [Runtime Hook](#runtime-hook) that allows external components to interact with the lifecycle of a Cluster.

See [Implementing Lifecycle Hooks](../tasks/experimental-features/runtime-sdk/implement-lifecycle-hooks.md)
# M
---

### Machine

Or __Machine Resource__

The Custom Resource for Kubernetes that represents a request to have a place to run kubelet.

See also: [Server](#server)

### Manage a cluster

Perform create, scale, upgrade, or destroy operations on the cluster.

### Managed Kubernetes

Managed Kubernetes refers to any Kubernetes cluster provisioning and maintenance abstraction, usually exposed as an API, that is natively available in a Cloud provider. For example: [EKS](https://aws.amazon.com/eks/), [OKE](https://www.oracle.com/cloud/cloud-native/container-engine-kubernetes/), [AKS](https://azure.microsoft.com/en-us/products/kubernetes-service), [GKE](https://cloud.google.com/kubernetes-engine), [IBM Cloud Kubernetes Service](https://www.ibm.com/cloud/kubernetes-service), [DOKS](https://www.digitalocean.com/products/kubernetes), and many more throughout the Kubernetes Cloud Native ecosystem.

### Managed Topology

See [Topology](#topology)

### Management cluster

The cluster where one or more Infrastructure Providers run, and where resources (e.g. Machines) are stored. Typically referred to when you are provisioning multiple workload clusters.

### Multi-tenancy

Multi tenancy in Cluster API defines the capability of an infrastructure provider to manage different credentials, each
one of them corresponding to an infrastructure tenant.

Please note that up until v1alpha3 this concept had a different meaning, referring to the capability to run multiple
instances of the same provider, each one with its own credentials; starting from v1alpha4 we are disambiguating the two concepts.

See also [Support multiple instances](../developer/core/support-multiple-instances.md).

# N
---

### Node pools

A node pool is a group of nodes within a cluster that all have the same configuration.

# O
---

### Operating system

Or __OS__

A generically understood combination of a kernel and system-level userspace interface, such as Linux or Windows, as opposed to a particular distribution.

# P
---

### Patch

A set of instructions describing modifications to a Kubernetes object. Examples include JSON Patch and JSON Merge Patch.

### Pivot

Pivot is a process for moving the provider components and declared cluster-api resources from a Source Management cluster to a Target Management cluster.

The pivot process is also used for deleting a management cluster and could also be used during an upgrade of the management cluster.

### Provider

Or __Cluster API provider__

This term was originally used as abbreviation for [Infrastructure provider](#infrastructure-provider), but currently it is used
to refer to any project that can be deployed and provides functionality to the Cluster API management Cluster.

See [Bootstrap provider](#bootstrap-provider), [Control plane provider](#control-plane-provider), [Core provider](#core-provider),
[Infrastructure provider](#infrastructure-provider), [IPAM provider](#ipam-provider)  [Runtime extension provider](#runtime-extension-provider).

### Provider components

Refers to the YAML artifact published as part of the release process for [providers](#provider);
it usually includes Custom Resource Definitions (CRDs), Deployments (to run the controller manager), RBAC, etc.

In some cases, the same expression is used to refer to the instances of above components deployed in a management cluster.

See [Provider repository](#provider-repository)

### Provider repository

Refers to the location where the YAML for [provider components](#provider-components) are hosted; usually a provider repository hosts
many version of provider components, one for each released version.

# R
---

### Runtime Extension

An external component which is part of a system built on top of Cluster API that can handle requests for a specific Runtime Hook.

See [Runtime SDK](#runtime-sdk)

### Runtime Extension provider

Refers to a [provider](#provider) that implements one or more [runtime extensions](#runtime-extension).
Runtime Extension provider's interaction with Cluster API are based on the Open API spec for [runtime hooks](#runtime-hook).

### Runtime Hook

A single, well identified, extension point allowing applications built on top of Cluster API to hook into specific moments of the [Cluster API Runtime](#cluster-api-runtime), e.g. [BeforeClusterUpgrade](../tasks/experimental-features/runtime-sdk/implement-lifecycle-hooks.md#beforeclusterupgrade), [TopologyMutationHook](#topology-mutation-hook).

See [Runtime SDK](#runtime-sdk)

### Runtime SDK

A developer toolkit required to build Runtime Hooks and Runtime Extensions.

See [Runtime SDK](../tasks/experimental-features/runtime-sdk/index.md)

# S
---

### Scaling

Unless otherwise specified, this refers to horizontal scaling.

### Stacked control plane

A control plane node where etcd is colocated with the Kubernetes API server, and
is running as a static pod.

### Server

The infrastructure that backs a [Machine Resource](#machine), typically either a cloud instance, virtual machine, or physical host.

# T
---

### Topology

A field in the Cluster object spec that allows defining and managing the shape of the Cluster's control plane and worker machines from a single point of control. The Cluster's topology is based on a [ClusterClass](#clusterclass).
Sometimes it is also referred as a managed topology.

See [ClusterClass](#clusterclass)


### Topology Mutation Hook

A [Runtime Hook](#runtime-hook) that allows external components to generate [patches](#patch) for customizing Kubernetes objects that are part of a [Cluster topology](#topology).

See [Topology Mutation](../tasks/experimental-features/runtime-sdk/implement-topology-mutation-hook.md)

# W
---

### Workload Cluster

A cluster created by a ClusterAPI controller, which is *not* a bootstrap cluster, and is meant to be used by end-users, as opposed to by CAPI tooling.

### WorkerClass

A collection of templates that define a set of worker nodes in the cluster. A ClusterClass contains zero or more WorkerClass definitions.

See [ClusterClass](#clusterclass)
