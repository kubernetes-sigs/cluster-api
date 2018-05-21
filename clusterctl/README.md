# clusterctl

`clusterctl` is the SIG-cluster-lifecycle sponsored tool that implements the Cluster API.

Read the [experience doc here](https://docs.google.com/document/d/1-sYb3EdkRga49nULH1kSwuQFf1o6GvAw_POrsNo5d8c/edit#).

## Getting Started

### Prerequisites

1. Install [minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) 
2. Build the `clusterctl` tool

```bash
$ cd $GOPATH/src/sigs.k8s.io/
$ git clone https://github.com/<GITHUB_USERNAME>/cluster-api.git
$ cd $GOPATH/src/sigs.k8s.io/cluster-api/clusterctl/
$ go build
```
 
### Limitations


### Creating a cluster

**NOT YET SUPPORTED!** - Use [provider-specific deployer](../README.md) to create clusters till cluster creation is supported.

1. Create a `cluster.yaml` and `machines.yaml` files configured for your cluster. See the provider specific templates and generation tools at `$GOPATH/src/sigs.k8s.io/cluster-api/clusterctl/examples/<provider>`. 
2. Create a cluster 
```
clusterctl create cluster -c cluster.yaml -m machines.yaml
```
Additional advanced flags can be found via help
```
clusterctl create cluster --help
```

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
