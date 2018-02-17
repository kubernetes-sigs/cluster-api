# Cluster API Development Guide

## Cloud Provider Dev Guide

### Overview

The Cluster API is a Kubernetes project to bring declarative, Kubernetes-style APIs to cluster creation, configuration, and management. It provides optional, additive functionality on top of core Kubernetes.

This document is meant to help OSS contributors implement support for providers (cloud or on-prem).

As part of adding support for a provider (cloud or on-prem), you will need to:

1.  Create tooling that conforms to the Cluster API (described further below)
1.  A machine controller that can run independent of the cluster. This controller should handle the lifecycle of the machines, whether it's run in-cluster or out-cluster.

The machine controller should be able to act on a subset of machines that form a cluster (for example using a label selector).

### Resources

*   [kubernetes/kube-deploy](https://github.com/kubernetes/kube-deploy)
*   [Cluster Management API KEP](https://github.com/kubernetes/community/blob/master/keps/sig-cluster-lifecycle/0003-cluster-api.md)
*   [Cluster type](https://github.com/kubernetes/kube-deploy/blob/master/ext-apiserver/pkg/apis/cluster/v1alpha1/cluster_types.go#L40)
*   [Machine type](https://github.com/kubernetes/kube-deploy/blob/master/ext-apiserver/pkg/apis/cluster/v1alpha1/machine_types.go#L42)

### A new Machine can be created in a declarative way

**A new Machine can be created in a declarative way, including Kubernetes version and container runtime version. It should also be able to specify provider-specific information such as OS image, instance type, disk configuration, etc., though this will not be portable.**

When a cluster is first created with a cluster config file, there is no master node or api server. So the user will need to bootstrap a cluster. While the implementation details are specific to the provider, the following guidance should help you:

* Your tool should spin up the external apiserver and the machine controller.
* POST the objects to the apiserver.
* The machine controller creates resources (Machines etc)
* Pivot the apiserver and the machine controller in to the cluster.

### A specific Machine can be deleted, freeing external resources associated with it.

When the client deletes a Machine object, your controller's reconciler should trigger the deletion of the Machine that backs that machine. The delete is provider specific, but usually requires deleting the VM and freeing up any external resources (like IP).

### A specific Machine can be upgraded or downgraded

These include:

*   A specific Machine can have its kubelet version upgraded or downgraded.
*   A specific Machine can have its container runtime changed, or its version upgraded or downgraded.
*   A specific Machine can have its OS image upgraded or downgraded.

A sample implementation for an upgrader is [provided here](https://github.com/kubernetes/kube-deploy/blob/master/ext-apiserver/tools/upgrader/util/upgrade.go). Each machine is upgraded serially, which can amount to:

```
for machine in machines:
    upgrade machine
```

The specific upgrade logic will be implement as part of the machine controller, and is specific to the provider. The user provided provider config will be in `machine.Spec.ProviderConfig`.

Discussion around in-place vs replace upgrades [is here](https://github.com/kubernetes/community/blob/master/keps/sig-cluster-lifecycle/0003-cluster-api.md#in-place-vs-replace).
