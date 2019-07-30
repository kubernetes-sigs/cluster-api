# Using `clusterctl` to create a cluster from scratch
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [What is `clusterctl`?](#what-is-clusterctl)
- [Creating a cluster](#creating-a-cluster)
  - [Creating a workload cluster using the management cluster](#creating-a-workload-cluster-using-the-management-cluster)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

This document provides an overview of how `clusterctl` works and explains how one can use `clusterctl`
to create a Kubernetes cluster from scratch.

## What is `clusterctl`?

`clusterctl` is a CLI tool to create a Kubernetes cluster. `clusterctl` is provided by the [provider implementations](https://github.com/Kubernetes-sigs/cluster-api#provider-implementations).
It uses Cluster API provider implementations to provision resources needed by the Kubernetes cluster.

## Creating a cluster

`clusterctl` needs 4 YAML files to start with: `provider-components.yaml`, `cluster.yaml`, `machines.yaml` ,
`addons.yaml`.

* `provider-components.yaml` contains the *Custom Resource Definitions ([CRDs](https://Kubernetes.io/docs/concepts/extend-Kubernetes/api-extension/custom-resources/))* 
of all the resources that are managed by Cluster API. Some examples of these resources
are: `Cluster`, `Machine`, `MachineSet`, etc. For more details about Cluster API resources
click [here](https://cluster-api.sigs.k8s.io/common_code/architecture.html#cluster-api-resources).
* `cluster.yaml` defines an object of the resource type `Cluster`.
* `machines.yaml` defines an object of the resource type `Machine`. Generally creates the machine
that becomes the control-plane.
* `addons.yaml` contains the addons for the provider.

Many providers implementations come with helpful scripts to generate these YAMLS. Provider implementation
can be found [here](https://github.com/Kubernetes-sigs/cluster-api#provider-implementations).  

`clusterctl` also comes with additional features. For example, `clusterctl` can also take in an optional
`bootstrap-only-components.yaml` to provide resources to the bootstrap cluster without also providing them
to the target cluster post-pivot.

For more details about all the supported options run:

```
clusterctl create cluster --help
```

After generating the YAML files run the following command:

```
clusterctl create cluster --provider <PROVIDER> --bootstrap-type <BOOTSTRAP CLUSTER TYPE> -c cluster.yaml -m machines.yaml -p provider-components.yaml --addon-components addons.yaml
```

Example usage:

```
# VMware vSphere
clusterctl create cluster --provider vsphere --bootstrap-type kind -c cluster.yaml -m machines.yaml -p provider-components.yaml --addon-components addons.yaml

# Amazon AWS
clusterctl create cluster --provider aws --bootstrap-type kind -c cluster.yaml -m machines.yaml -p provider-components.yaml --addon-components addons.yaml
```

**What happens when we run the command?**  
After running the command first it creates a local cluster. If `kind` was passed as the `--bootstrap-type`
it creates a local [kind](https://kind.sigs.k8s.io/) cluster. This cluster is generally referred to as the *bootstrap cluster*.  
On this kind Kubernetes cluster the `provider-components.yaml` file is applied. This step loads the CRDs into
the cluster. It also creates 2 [StatefulSet](https://Kubernetes.io/docs/concepts/workloads/controllers/statefulset/)
pods that run the cluster api controller and the provider specific controller. These pods register the custom
controllers that manage the newly defined resources (`Cluster`, `Machine`, `MachineSet`, etc).  

Next, `clusterctl` applies the `cluster.yaml` and `machines.yaml` to the local kind Kubernetes cluster. This
step creates a Kubernetes cluster with only a control-plane(as defined in `machines.yaml`) on the specified
provider. This newly created cluster is generally referred to as the *management cluster* or *pivot cluster*
or *initial target cluster*. The management cluster is responsible for creating and maintaining the work-load cluster.  
  
Lastly, `clusterctl` moves all the CRDs and the custom controllers from the bootstrap cluster to the
management cluster and deletes the locally created bootstrap cluster. This step is referred to as the *pivot*.

### Creating a workload cluster using the management cluster

The *workload cluster* also sometimes referred to as the *target cluster* is the Kubernetes cluster on to which
the final application is deployed. The target cluster is responsible for handling the workload of the application,
not the management cluster.

As the management cluster is up we can create a workload cluster by simply applying the appropriate
`cluster.yaml`, `machines.yaml` and `machineset.yaml` on the management cluster. This will create the VMs(Nodes)
as defined in these YAMLs. Following this, a bootstrap mechanism is used to create a Kubernetes cluster on these VMs.
While any of the several bootstrapping mechanisms can be used `kubeadm` is the popular option.

**NOTE:** Workload clusters do not have any addons applied. Nodes in your workload clusters will be in the `NotReady`
state until you apply addons for CNI plugin.
  
Once the target cluster is up the user can create the `Deployments`, `Services`, etc that handle the workload
of the application.  
