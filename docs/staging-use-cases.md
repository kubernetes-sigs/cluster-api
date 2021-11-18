---
title: Cluster API Reference Use Cases
creation-date: 2019-04-16
last-updated: 2019-04-16
---

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Cluster API Reference Use Cases](#cluster-api-reference-use-cases)
  - [Role Glossary](#role-glossary)
  - [Icon Glossary](#icon-glossary)
  - [Operator of Workload Cluster](#operator-of-workload-cluster)
    - [Creating Clusters](#creating-clusters)
    - [Staged Adoption of Cluster API By Operators](#staged-adoption-of-cluster-api-by-operators)
    - [Deleting Clusters](#deleting-clusters)
    - [Scaling](#scaling)
    - [Configuration Updates](#configuration-updates)
    - [Security](#security)
    - [Upgrades](#upgrades)
    - [Monitoring](#monitoring)
    - [Adoption](#adoption)
    - [Multitenancy Management](#multitenancy-management)
    - [Disaster Recovery](#disaster-recovery)
  - [Operator of Management Cluster](#operator-of-management-cluster)
    - [Versioning and Upgrades](#versioning-and-upgrades)
    - [Removing Cluster API](#removing-cluster-api)
    - [Cross-cluster Metrics](#cross-cluster-metrics)
    - [Specific Architecture Approaches](#specific-architecture-approaches)
    - [Multitenancy Management](#multitenancy-management-1)
  - [Multi-cluster/Multi-provider](#multi-clustermulti-provider)
    - [Managing Providers](#managing-providers)
    - [Creating Workload Clusters](#creating-workload-clusters)
    - [Provider Implementors](#provider-implementors)
  - [Cluster Health Checking](#cluster-health-checking)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Cluster API Reference Use Cases

This is a living document that serves as a reference and a staging area for use cases collected from the community during post-v1alpha1 project redesign.

## Role Glossary
- __User__: consumer of a Kubernetes-conformant cluster created by the Cluster API.
    - Does not use Cluster API.
- __Operator__: Administrator responsible for creating and managing a Kubernetes cluster deployed by Cluster API.
    - Uses Cluster API.
- __Multi-cluster operator__: An operator responsible for multiple Kubernetes clusters deployed by Cluster API.
    - Uses Cluster API.
    - Cares about keeping config similar between many clusters.


## Icon Glossary
- 🔭 Out of Scope for Cluster API itself, but should be possible via higher level tool.
    - These are use-cases that we should take care not to prevent.

## Operator of Workload Cluster

### Creating Clusters

- As an operator, given that I have a cluster running Cluster API, I want to be able to use declarative APIs to manage another Kubernetes cluster (create, upgrade, scale, delete).

- As an operator, given that I have a cluster running Cluster API, I want to be able to use declarative APIs to manage a vendor’s Kubernetes conformant cluster (create, upgrade, scale, delete).

- As an operator, when I create a new cluster using Cluster API, I want Cluster API to automatically create and manage any supporting provider infrastructure needed for my new cluster to function.

- As an operator, when I create a new cluster using Cluster API, I want to be able to use existing infrastructure (e.g. VPC’s, SecurityGroups, veth, GPUs).

- 🔭 As an operator, when I create a new cluster using Cluster API, I want to be able to take advantage of resource aware topology (e.g. compute, device availability, etc.) to place machines.

- As an operator, I need to have a way to make minor customizations before kubelet starts while using a standard node image and standard boot script. Example: write a file, disable hyperthreading, load a kernel module.

- As an operator, I need to have a way to apply labels to Nodes created through ClusterAPI. This will allow me to partition workloads across Nodes/Machines and MachineDeployments. Examples of labels include datacenter, subnet, and hypervisor, which applications can use in affinity policies.

- As an operator, I want to be able to provision the nodes of a workload cluster on an existing vnet that I don’t have admin control of.

### Staged Adoption of Cluster API By Operators

- As an operator, I would like to use some features of Cluster API without using all features of Cluster API.

- As an operator, given that I have a management cluster and a pre-existing control plane, I would like to manage the lifecycle of a group of worker nodes without managing the control plane those nodes join.

### Deleting Clusters

- As an operator, when I delete a Cluster object, I want Cluster API to delete all the infrastructure it created for that cluster.

- As an operator, when I delete a Machine object, I want Cluster API to gracefully shutdown (drain) that Node and delete all the infrastructure it created for that Machine.

### Scaling

- As an operator, given that I have deployed a cluster using Cluster API, I want to configure the cluster-autoscaler to drive scaling operations.

- As an operator, given that I have a management cluster and a workload cluster, I want to retrieve, set, and change the number of worker Nodes or control plane Nodes in my workload cluster.

- As an operator, given I have a management cluster and a workload cluster, I want to control the sizing, scaling, and optimizing of the workload cluster’s control plane in terms of Kubernetes primitives (e.g. HPA, VPA, resource limits, etc). I would like to import the best practices, sizing metrics and knowledge from e.g. the specialized SIG Scalability; the information should be expressed uniformly in Kubernetes terms.

- As an operator, I expect the Cluster API to maintain the number and type of Nodes that I have currently requested as members of the cluster.

### Configuration Updates

- As an operator, given that I have a management cluster and a workload cluster, I want to update the IaaS credentials used to lifecycle manage my workload cluster because the correct credentials have changed.

- As an operator, given I have a management cluster and a workload cluster, I want to apply configuration changes before kubelet starts.

- As an operator, given that I have deployed a workload cluster via Cluster API, I want to change config in my workload cluster for which Cluster API is authoritative and have a Cluster API controller manage the deployment of that new configuration over the workload cluster.

- As an operator, given that I have deployed a workload cluster via Cluster API and used Cluster API controller to manage the deployment of a new (broken) configuration, I want to revert to the previous working configuration.

- As an operator, I want to declare the attributes (os, kernel, CRI)  of the nodes I want my workload to run on. The CAPI provider/controller should select an appropriate image that satisfies my constraints/attributes.
    - If the provider does not support the attributes I have specified or find an appropriate image it should fail with an appropriate error.

### Security

- As an operator, given I have a management cluster and a workload cluster, I want to know when my cluster’s/machines’ certificates will expire, so that I can plan to rotate them.

- As an operator, given I have a management cluster and a workload cluster, I want to automatically, periodically repave all the nodes of my cluster to reduce the risk of unauthorized software running on my machines.

- As an operator, I want an external CA to sign certificates for the workload cluster control plane.

- 🔭As an operator, given I have a management cluster and a workload cluster, I want to rotate all the certificates and credentials my machines are using.
    - Some certificates might get rotated as part of machine upgrade, but are otherwise the above is out of scope.

- 🔭 As an operator, given I have a management cluster and a workload cluster, I want to rotate/change the CA used to sign certificates for my workload cluster.

- 🔭 As an operator, I want an external CA to sign certificates for workload cluster kubelets.

### Upgrades
- As an operator, given I have a management cluster and a workload cluster, I want to patch the OS running on all of my machines (e.g. for a CVE).

- As an operator, given I have a management cluster and a workload cluster, I want to upgrade my workload cluster (control plane and nodes) to a new version of kubernetes. I want the workload cluster control plane to be available during the upgrade.

- As an operator, given I have a management cluster and a workload cluster, I want to upgrade my workload cluster control plane to a new version of Kubernetes and also update my etcd version at the same time. I want to know in advance if the upgrade will require control plane downtime.

- As an operator, given I have a management cluster and a workload cluster, I want to upgrade the version of CNI plugin and network daemon that my workload cluster is using. I want to know in advance if the upgrade could cause application downtime.

- 🔭 As an operator, given I have a management cluster and a workload cluster, I want to upgrade my workload cluster to a new version of etcd without upgrading the Kubernetes control plane. I want to know in advance if the upgrade will require control plane downtime.

### Monitoring
- 🔭 As an operator, given I have a management cluster and a workload cluster, I want to retrieve metrics about the underlying machines (e.g. CPU usage, memory) in the workload cluster.

- 🔭 As an operator, given I have a management cluster, a workload cluster, and permission to open interactive shells on that workload cluster, I want to open an interactive shell on the machines in my workload cluster.

- 🔭 As an operator, given I have a management cluster and a workload cluster, I want to monitor the cleanup of persistent disks/volumes used by my workload cluster.

- 🔭 As an operator, given I have a management cluster and a workload cluster, I want to monitor the cleanup of created by my workload cluster.

- 🔭 As an operator, given I have a management cluster and a workload cluster, I want to ensure the etcd database in my workload cluster is backed up.

### Adoption
- 🔭 As an operator, given I have created a Kubernetes-conformant cluster without ClusterAPI, I want to use ClusterAPI to manage it. In order to do so, I need to know the requirements for adopting/importing this cluster in terms of required CRD’s and operators (e.g Machine and Cluster objects).

### Multitenancy Management
- 🔭 As an operator, given I have a management cluster and a workload cluster, I want to setup roles, role bindings, users, and usage quotas on my workload cluster.

### Disaster Recovery
- 🔭As an operator, I want to be able to recover from the complete loss of all the control plane replicas of a workload cluster. This excludes etcd.

- 🔭As an operator, I want to be able to recover the etcd cluster of a workload cluster from an irrecoverable failure. I will provide the etcd snapshot required by the recovery mechanism.

## Operator of Management Cluster

- As an operator, given I have a Kubernetes-conformant cluster, I would like to install Cluster API and a provider on it in a straight-forward process.

- As an operator, given I have a management cluster that was deployed by Cluster API (via the pivot workflow), I want to manage the lifecycle of my management cluster using Cluster API.

- As an operator, given I am following the instructions in the Cluster API (/provider) README, I expect the instructions to work and that I will end up with a working management cluster.

### Versioning and Upgrades
- As an operator, when I choose a version of Cluster API and provider to use, I want to know what version(s) of Kubernetes and other software (CNI, docker, OS, etc) can be managed by a specific Cluster API and/or provider version.

- As an operator of a management cluster, given that I have a cluster running Cluster API, I would like to upgrade the Cluster API and provider(s) without the users of Cluster API noticing (e.g. due to their API requests not working).

- As an operator of a management cluster, I want to know what versions of kubelet, control plane, OS, etc, all of the associated workload clusters are running, so that I can plan upgrades to the management cluster that will not break anyone’s ability to manage their workload clusters.

### Removing Cluster API
- As an operator of a management cluster, given that I have a management cluster that I have used to deploy several workload clusters, I want to remove the Cluster/Machine objects representing one workload cluster from my management cluster without deprovisioning workload cluster.

- As an operator of a management cluster, given that I have a management cluster that I have used to deploy several workload clusters, I want to uninstall Cluster API from my management cluster without deprovisioning my workload clusters.

- As an operator of a management cluster, given that I have a management cluster, I want to use it to manage workload clusters that were created by a different management cluster.

### Cross-cluster Metrics
- As an operator of a management cluster, I want to query my resource allocation on an infrastructure.  For example, in an on-prem case, I do not have an infinite capacity cloud, so  I need to be able to determine my reservation before deploying a workload cluster.

### Specific Architecture Approaches
- As an operator of a management cluster, given that I give operators of workload clusters access to my management cluster, they can launch new workload clusters with control planes that run in the management cluster while the nodes of those workload clusters run elsewhere.

- As a multi-cluster operator, I would like to provide an EKS-like experience in which the workload control plane nodes are joined to the management cluster and the control plane config isn’t exposed to the consumer of the workload cluster. This enables me as an operator to manage the control plane nodes for all clusters using tooling like prometheus and fluentd. I can also control the configuration of the workload control plane in accordance with business policy.

### Multitenancy Management
- As an operator of a management cluster, I want to control which users of management cluster can deploy new workload clusters, how many clusters they can deploy, and how many nodes/resources those clusters can use.

- As an operator of a management cluster, I want to ensure that only the user who creates a new workload cluster (and some specific other users) can manage and access the workload cluster.

- As an operator of a management cluster, I want the user who creates a new workload cluster to be able to give permission to other users to manage that cluster.

- As an operator of a management cluster, I want to configure whether operators of workload clusters are allowed to open interactive shells onto those clusters machines.

## Multi-cluster/Multi-provider

### Managing Providers
- As an operator, given I have a management cluster with at least one provider, I would like to install a new provider.

- As an operator, given I have a management cluster with at least one provider, I would like to remove one of those providers and orphan any clusters provisioned by that provider.

- As a multi-cluster operator, given that I have a single management cluster and that I have installed multiple providers and that one of those providers is malicious, I want that provider not to see IaaS secrets provided to any of the other providers.

- As an operator, if I have a management cluster running a particular Cluster API version and a particular set of providers, then I want to plan an upgrade of Cluster API and the providers so that I upgrade one at a time and always end up with a compatible set of versions.

### Creating Workload Clusters
- As a multi-cluster operator, given that I have a management cluster, I want to create workload clusters across multiple providers with a consistent interface. For example, if I can create clusters on AWS without any manual intervention, I should have the same level of automation and lack of gotchas when using the VSphere provider.

- As a multi-cluster operator, given that I have a management cluster, I want to create workload clusters across multiple providers that are all similarly configured.

- As a multi-cluster operator, given that I have deployed my clusters via Cluster API, I want to find general information (name, status, access details) about my clusters across multiple providers.

- As a multi-cluster operator, I want to know what versions of Kubernetes all of my workload clusters are running across multiple providers.

- As a multi-cluster operator, given that I deploy workload clusters via several providers, I want to see a health and status summary from different providers. The detailed information can be provider specific, but a general, common status for generic phases must be given.

- As a multi-cluster operator, given that I have deployed my clusters via Cluster API, I want to view the configuration of all my clusters across multiple providers.

- As a multi-cluster operator, given that I have a single management cluster and that I have installed multiple providers, I want to lifecycle manage multiple workload clusters on each installed provider.

### Provider Implementors
- As a provider, I want the machine controller to reconcile a Machine in response to an event from some other resource in the cluster. This is the sort of thing that other controllers do on a regular basis, so that's nothing particularly interesting. But having made a machine actuator, there's not an easy way to get access to the machine controller object in order to call its Watch method.

## Cluster Health Checking

Cluster Health Checking is a service to provide the health status of Kubernetes cluster and its components.

- As an operator, given I have created a Kubernetes-conformant cluster with ClusterAPI, I want to check the Kubernetes cluster node status.
  - Describe nodes and provide details if they are ready/healthy or not ready/healthy.
  - List conditions for any nodes which are `NotReady`, list information about allocated resources.

- As an operator, given I have created a Kubernetes-conformant cluster with ClusterAPI, I want to check the kube-apiserver status.

- As an operator, given I have created a Kubernetes-conformant cluster with ClusterAPI, I want to check the etcd status.

- 🔭 As an operator, given I have created a Kubernetes-conformant cluster with ClusterAPI, I want to check the Kubernetes components status, like ingress controller, other add-on components etc.

- 🔭 As an operator, given I have created a Kubernetes-conformant cluster with ClusterAPI, I want to check unhealthy Pods statuses in configured namespace.
  - Provide the details on any pods which are unhealthy in `kube-system` namespace. Filter the unhealthy pods for their status(`kubectl get pods --show-labels -n kube-system | grep -vE "Running|Completed"`)
  - Describe any Pods which are not `Completed|Running`, list the Events to provide hints on the failure.
  - Look for Pods which don't have all of their containers running.
