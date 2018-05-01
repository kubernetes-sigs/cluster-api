# clusterctl

`clusterctl` is the SIG-cluster-lifecycle sponsored tool that implements the Cluster API.

Read the [experience doc here](https://docs.google.com/document/d/1-sYb3EdkRga49nULH1kSwuQFf1o6GvAw_POrsNo5d8c/edit#).

## Getting Started

### Prerequisites

Follow the steps listed at [CONTRIBUTING.md](https://github.com/kubernetes/kube-deploy/blob/master/cluster-api/clusterctl/CONTRIBUTING.md) to:

1. Build the `clusterctl` tool 
3. Create a `machines.yaml` file configured for your cluster. See the provided template for an example.

### Limitation


### Creating a cluster

**NOT YET SUPPORTED!**

In order to create a cluster with the Cluster API, the user will supply these:

* Cluster which defines the spec common across the entire cluster.
* Machine which defines the spec of a machine. Further abstractions of
MachineSets and MachineClass and MachineDeployments are supported as well.
* Extras (optional) spec extras.yaml file with specs of all controllers
(ConfigMaps) that the cluster needs. This would include the Machine controller,
MachineSet controller, Machine Setup ConfigMap etc. Note that this is not a new API
object. There will be defaults running. This will make the tool easily pluggage
(change the controller spec) instead of changing the tool or mucking with flags.

1. Create a cluster: `./clusterctl create cluster -c cluster.yaml -m machines.yaml -e extras.yaml`

### Interacting with your cluster

Once you have created a cluster, you can interact with the cluster and machine
resources using kubectl:

```
$ kubectl get clusters
$ kubectl get machines
$ kubectl get machines -o yaml
```

#### Scaling your cluster

**NOT YET SUPPORTED!**

#### Upgrading your cluster

**NOT YET SUPPORTED!**

#### Node repair

**NOT YET SUPPORTED!**

### Deleting a cluster

**NOT YET SUPPORTED!**
