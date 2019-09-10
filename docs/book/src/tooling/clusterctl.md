<aside class="note warning">
<h1>Deprecation Notice</h1>

This utility has been deprecated in v1alpha2 and will be removed in a future version. 

</aside>

# Using `clusterctl` to create a cluster from scratch

This document provides an overview of how `clusterctl` works and explains how one can use `clusterctl`
to create a Kubernetes cluster from scratch.

## What is `clusterctl`?

`clusterctl` is a CLI tool to create a Kubernetes cluster. `clusterctl` is provided by the [provider implementations](https://cluster-api.sigs.k8s.io/reference/providers).
It uses Cluster API provider implementations to provision resources needed by the Kubernetes cluster.

## Creating a cluster

`clusterctl` needs 4 YAML files to start with: `provider-components.yaml`, `cluster.yaml`, `machines.yaml` ,
`addons.yaml`.

* `provider-components.yaml` contains the *Custom Resource Definitions ([CRDs](https://Kubernetes.io/docs/concepts/extend-Kubernetes/api-extension/custom-resources/))* 
of all the resources that are managed by Cluster API. Some examples of these resources
are: `Cluster`, `Machine`, `MachineSet`, etc. For more details about Cluster API resources
click [here](https://cluster-api.sigs.k8s.io/architecture/controllers).
* `cluster.yaml` defines an object of the resource type `Cluster`.
* `machines.yaml` defines an object of the resource type `Machine`. Generally creates the machine
that becomes the control-plane.
* `addons.yaml` contains the addons for the provider.

Many providers implementations come with helpful scripts to generate these YAMLS. Provider implementation
can be found [here](https://cluster-api.sigs.k8s.io/reference/providers).  

`clusterctl` also comes with additional features. For example, `clusterctl` can also take in an optional
`bootstrap-only-components.yaml` to provide resources to the bootstrap cluster without also providing them
to the target cluster post-pivot.

For more details about all the supported options run:

```
clusterctl create cluster --help
```

After generating the YAML files run the following command:

```
clusterctl create cluster --bootstrap-type <BOOTSTRAP CLUSTER TYPE> -c cluster.yaml -m machines.yaml -p provider-components.yaml --addon-components addons.yaml
```

Example usage:

```
clusterctl create cluster --bootstrap-type kind -c cluster.yaml -m machines.yaml -p provider-components.yaml --addon-components addons.yaml
```

**What happens when we run the command?**  
After running the command first it creates a local cluster. If `kind` was passed as the `--bootstrap-type`
it creates a local [kind](https://kind.sigs.k8s.io/) cluster. This cluster is generally referred to as the *bootstrap cluster*.  
On this kind Kubernetes cluster the `provider-components.yaml` file is applied. This step loads the CRDs into
the cluster. It also creates two [Deployment](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)
pods that run the cluster api controller manager and the provider specific controller manager. These pods register 
the custom controllers that manage the newly defined resources (`Cluster`, `Machine`, `MachineSet`, `MachineDeployment`, 
as well as provider-specific resources).  

Next, `clusterctl` applies the `cluster.yaml` and `machines.yaml` to the local kind Kubernetes cluster. This
step creates a Kubernetes cluster with only a control-plane (as defined in `machines.yaml`) on the specified
provider. This newly created cluster is generally referred to as the *management cluster* or *pivot cluster*
or *initial target cluster*. The management cluster is responsible for creating and maintaining the workload cluster.  
  
Lastly, `clusterctl` moves all the CRDs and the custom controllers from the bootstrap cluster to the
management cluster and deletes the locally created bootstrap cluster. This step is referred to as the *pivot*.
