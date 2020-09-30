# Table of Contents

[A](#a) | [B](#b) | [C](#c) | [D](#d) | [H](#h) | [I](#i) | [K](#k) | [M](#m) | [N](#n) | [O](#o) | [P](#p) | [S](#s) | [T](#t) | [W](#w)

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

# C
---

### CAEP
Cluster API Enhancement Proposal - patterned after [KEP](https://git.k8s.io/enhancements/keps/README.md). See [template](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/YYYYMMDD-template.md)

### CAPI
Core Cluster API

### CAPA
Cluster API Provider AWS

### CABPK
Cluster API Bootstrap Provider Kubeadm

### CAPD
Cluster API Provider Docker

### CAPG
Cluster API Google Cloud Provider

### CAPIBM
Cluster API Provider IBM Cloud

### CAPO
Cluster API Provider OpenStack

### CAPV
Cluster API Provider vSphere

### CAPZ
Cluster API Provider Azure


### Cluster

A full Kubernetes deployment. See Management Cluster and Workload Cluster

### Cluster API

Or __Cluster API project__

The Cluster API sub-project of the SIG-cluster-lifecycle. It is also used to refer to the software components, APIs, and community that produce them.

### Control plane

The set of Kubernetes services that form the basis of a cluster. See also https://kubernetes.io/docs/concepts/#kubernetes-control-plane There are two variants:

* __Self-provisioned__: A Kubernetes control plane consisting of pods or machines wholly managed by a single Cluster API deployment.
* __External__: A control plane offered and controlled by some system other than Cluster API (e.g., GKE, AKS, EKS, IKS).

# D
---

### Default implementation

A feature implementation offered as part of the Cluster API project, infrastructure providers can swap it out for a different one.

# H
---

### Horizontal Scaling

The ability to add more machines based on policy and well defined metrics. For example, add a machine to a cluster when CPU load average > (X) for a period of time (Y).

### Host

see [Server](#server)

# I
---

### Infrastructure provider

A source of computational resources (e.g. machines, networking, etc.). Examples for cloud include AWS, Azure, Google, etc.; for bare metal include VMware, MAAS, metal3.io, etc. When there is more than one way to obtain resources from the same infrastructure provider (e.g. EC2 vs. EKS) each way is referred to as a variant.

### Instance

see [Server](#server)

### Immutability

A resource that does not mutate.  In kubernetes we often state the instance of a running pod is immutable or does not change once it is run.  In order to make a change, a new pod is run.  In the context of [Cluster API](#cluster-api) we often refer to a running instance of a [Machine](#machine) as being immutable, from a [Cluster API](#cluster-api) perspective.

# K
---

### Kubernetes-conformant

Or __Kubernetes-compliant__

A cluster that passes the Kubernetes conformance tests.

### k/k

Refers to the [main Kubernetes git repository](https://github.com/kubernetes/kubernetes) or the main Kubernetes project.

# M
---

### Machine

Or __Machine Resource__

The Custom Resource for Kubernetes that represents a request to have a place to run kubelet.

See also: [Server](#server)

### Manage a cluster

Perform create, scale, upgrade, or destroy operations on the cluster.

### Management cluster

The cluster where one or more Infrastructure Providers run, and where resources (e.g. Machines) are stored. Typically referred to when you are provisioning multiple workload clusters.

### Management group

A management group is a group of providers composed by a CoreProvider and a set of Bootstrap/ControlPlane/Infrastructure providers
watching objects in the same namespace. For example, a management group can be used for upgrades, in order to ensure all the providers
in a management group support the same Cluster API version.

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

### Pivot

Pivot is a process for moving the provider components and declared cluster-api resources from a Source Management cluster to a Target Management cluster.

The pivot process is also used for deleting a management cluster and could also be used during an upgrade of the management cluster.

### Provider

See [Infrastructure Provider](#user-content-infrastructure-provider)

### Provider components

Refers to the YAML artifact a provider publishes as part of their releases which is required to use the provider components,
it usually contains Custom Resource Definitions (CRDs), Deployments (to run the controller manager), RBAC, etc.

### Provider implementation

Existing Cluster API implementations consist of generic and infrastructure provider-specific logic. The [infrastructure provider](#infrastructure-provider)-specific logic is currently maintained in infrastructure provider repositories.

# S
---

### Scaling

Unless otherwise specified, this refers to horizontal scaling.

### Stacked control plane

A control plane node where etcd is colocated with the Kubernetes API server, and
is running as a static pod.

### Server

The infrastructure that backs a [Machine Resource](#user-content-machine), typically either a cloud instance, virtual machine, or physical host.

# W
---

### Workload Cluster

A cluster created by a ClusterAPI controller, which is *not* a bootstrap cluster, and is meant to be used by end-users, as opposed to by CAPI tooling.
