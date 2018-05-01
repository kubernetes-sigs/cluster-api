# clusterctl

`clusterctl` is the SIG-cluster-lifecycle sponsored tool that implements the Cluster API.

Read the [experience doc here](https://docs.google.com/document/d/1-sYb3EdkRga49nULH1kSwuQFf1o6GvAw_POrsNo5d8c/edit#).

## Getting Started

### Prerequisites

Follow the steps listed at [CONTRIBUTING.md](https://github.com/kubernetes-sigs/cluster-api/blob/master/cluster-api/clusterctl/CONTRIBUTING.md) to:

1. Build the `clusterctl` tool

```
go build
```
 
2. Create a `machines.yaml` file configured for your cluster. See the provided template for an example.

### Limitations


### Creating a cluster

**NOT YET SUPPORTED!**

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
