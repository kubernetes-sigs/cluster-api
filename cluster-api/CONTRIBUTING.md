# Cluster API Development Guide

## Cloud Provider Dev Guide

### Overview

The Cluster API is a Kubernetes project to bring declarative, Kubernetes-style APIs to cluster creation, configuration, and management. It provides optional, additive functionality on top of core Kubernetes.

This document is meant to help OSS contributors implement support for cloud providers.

As part of adding support for a provider (cloud or on-prem), you will need to:

1.  Create tooling that conforms to the Cluster API (described further below)
1.  A machine controller that can run independent of the cluster. This controller should handle the lifecycle of the machines, whether it's run inside the master in a pod, in an external API server, or even on your local workstation.

### Resources

*   [kubernetes/kube-deploy](https://github.com/kubernetes/kube-deploy)
*   [Cluster Management API KEP](https://github.com/kubernetes/community/blob/master/keps/sig-cluster-lifecycle/0003-cluster-api.md)
*   [Cluster type](https://github.com/kubernetes/kube-deploy/blob/fafc50e6420783179fd5fc7d73c7453f1de68eb4/cluster-api/api/cluster/v1alpha1/types.go#L32)
*   [Machine type](https://github.com/kubernetes/kube-deploy/blob/fafc50e6420783179fd5fc7d73c7453f1de68eb4/cluster-api/api/cluster/v1alpha1/types.go#L161)

[TODO: Add architecture diagram](https://github.com/kubernetes/kube-deploy/issues/546)

### A new Node can be created in a declarative way

**A new Node can be created in a declarative way, including Kubernetes version and container runtime version. It should also be able to specify provider-specific information such as OS image, instance type, disk configuration, etc., though this will not be portable.**

When a cluster is first created with a cluster config file, there is no master node or api server. So the user will need to bootstrap a cluster. While the implementation details are specific to the provider, the following guidance should help you:

*   Your tooling should create a Node for the master machine specified by the user. This master can use kubeadm to initialize the master. See [this example](https://github.com/kubernetes/kube-deploy/blob/68e27e43894efebb45f5f014aa5510c11015c3b3/cluster-api-gcp/cloud/google/templates.go).
*   Start the machine controller specific to the provider.
*   Register the clusters CRD with the machine controller.
*   POST the cluster config.
*   Register the machines CRD's with the machine controller.
*   POST the machines config.
*   A correctly implemented Update (discussed below) will then reconcile the machines.

### A specific Node can be deleted, freeing external resources associated with it.

When the client deletes a Machine object, your controller's reconciler should trigger the deletion of the Node that backs that machine. The delete is provider specific, but usually requires deleting the VM and freeing up any external resources (like IP).

### A specific Node can be upgraded or downgraded

These include:

*   A specific Node can have its kubelet version upgraded or downgraded.
*   A specific Node can have its container runtime changed, or its version upgraded or downgraded.
*   A specific Node can have its OS image upgraded or downgraded.

A sample implementation for an upgrader is [provided here](https://github.com/kubernetes/kube-deploy/blob/master/cluster-api/tools/upgrader/util/upgrade.go). Each machine is upgraded serially, which can amount to:

```
for machine in machines:
    upgrade machine
```

The specific upgrade logic will be implement as part of the machine controller, and is specific to the provider. The user provided provider config will be in `machine.Spec.ProviderConfig`. Users are free to use `machine.ObjectMeta.Annotations` as storage.

Discussion around in-place vs replace upgrades [is here](https://github.com/kubernetes/community/blob/master/keps/sig-cluster-lifecycle/0003-cluster-api.md#in-place-vs-replace).
