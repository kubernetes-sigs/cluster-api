## Using `clusterctl` to create a cluster from scratch

This document provides an overview of how `clusterctl` works and explains how one can use `clusterctl` to create a kubernetes cluster from scratch.

### What is `clusterctl`?
`clusterctl` is a CLI tool to create a kubernetes cluster. It uses Cluster API provider implementations to provision resourses needed by the kubernetes cluster.

### Creating a cluster
`clusterctl` needs 4 YAML files to start with: `provider-components.yaml`, `cluster.yaml`, `machines.yaml` , `addons.yaml`.  
* `provider-components.yaml` constain the *Custom Resource Definitions ([CRDs](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/))* of all the resources that are managed by Cluster API. Some examples of these resources are: `Cluster`, `Machine`, `MachineSet`, etc. For more details about Cluster API resources click [here](https://cluster-api.sigs.k8s.io/common_code/architecture.html#cluster-api-resources).
* `cluster.yaml` defines an object of the resource type `Cluster`.
* `machines.yaml` defines an object of the resource type `Machine`. Generally creates the machine that becomes the control-plane.
* `addons.yaml` contains the addons for the provider.

Many providers implemenatations come with helpful scripts to generate these YAMLS. Provider implementation can be found [here](https://github.com/kubernetes-sigs/cluster-api#provider-implementations).  

After generating the YAML files run the following command: 
```
clusterctl create cluster --provider <PROVIDER> --bootstrap-type <BOOTSTRAP CLUSTER TYPE> -c cluster.yaml -m machines.yaml -p provider-components.yaml --addon-components addons.yaml
```
Example usage:
```
# VMWare VSphere
clusterctl create cluster --provider vsphere --bootstrap-type kind -c cluster.yaml -m machines.yaml -p provider-components.yaml --addon-components addons.yaml

# Amazon AWS
clusterctl create cluster --provider aws --bootstrap-type kind -c cluster.yaml -m machines.yaml -p provider-components.yaml --addon-components addons.yaml
```

**What happens when we run the command?**  
After running the command first it creates a local cluster. If `kind` was passed as the `--bootstrap-type` it creates a local [kind](https://kind.sigs.k8s.io/) cluster. This cluster is generally referred to as the *bootstrap cluster*.  
On this kind kubernetes cluster the `provider-components.yaml` file is applied. This step loads the CRDs into the cluster. It also creates 2 [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) pods that run the cluster api controller and the provider specific controller. These pods register the custom controllers that manage the newly defined resources (`Cluster`, `Machine`, `MachineSet`, etc) with the [kubernetes controller manager](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-controller-manager/).  
  
Next, `clusterctl` applies the `cluster.yaml` and `machines.yaml` to the local kind kubernetes cluster. This step creates a kubernetes cluster with only a control-plane(as defined in `machines.yaml`) on the specified provider. This newly created cluster is generally referred to as the *management cluster* or *pivot cluster* or *initial target cluster*. The management cluster is resonsible for creating and maintaining the work-load cluster.  
  
Lastly, `clusterctl` moves all the CRDs and the custom controllers from the bootstrap cluster to the management cluster and deletes the locally created bootstrap cluster.  

### Creating a workload cluster using the management cluster
The *workload cluster* also sometimes referred to as the *target cluster* is the kubernetes custer on to which the final application is deployed. The target cluster is responsible for handling the workload of the application, not the management cluster.

As the management cluster is up we can create a workload cluster by simply applying the appropriate `cluster.yaml`, `machines.yaml` and `machineset.yaml` on the management cluster. This will create the VMs(Nodes) as defined in these YAMLs and uses `kubeadm` (generally pre-installed on these VMs) to perform `init` and `join` to create a kubernetes cluster on them.  
  
Once the target cluster is up the user can create the `Deplyments`, `Services`, etc that handle the workload of the application.


