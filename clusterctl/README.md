# clusterctl

`clusterctl` is the SIG-cluster-lifecycle sponsored tool that implements the Cluster API.

Read the [experience doc here](https://docs.google.com/document/d/1-sYb3EdkRga49nULH1kSwuQFf1o6GvAw_POrsNo5d8c/edit#).

## Getting Started

### Prerequisites

1. Install [minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) 
2. Install a driver. For Linux, we recommend kvm. For MacOS, we recommend VirtualBox.
2. Build the `clusterctl` tool

```bash
$ git clone https://github.com/kubernetes-sigs/cluster-api.git $GOPATH/src/sig.k8s.io/cluster-api
$ cd $GOPATH/src/sigs.k8s.io/cluster-api/clusterctl/
$ go build
```
 
### Limitations
TBD

### Creating a cluster
1. Create a `cluster.yaml`, `machines.yaml` and `provider-components.yaml` files configured for your cluster. See the provider specific templates and generation tools at `$GOPATH/src/sigs.k8s.io/cluster-api/clusterctl/examples/<provider>`. 
2. Create a cluster 

```shell
clusterctl create cluster --provider [google/vsphere] -c cluster.yaml -m machines.yaml -p provider-components.yaml
```
Additional advanced flags can be found via help

```shell
clusterctl create cluster --help
```

### Interacting with your cluster

Once you have created a cluster, you can interact with the cluster and machine
resources using kubectl:

```
$ kubectl --kubeconfig kubeconfig get clusters
$ kubectl --kubeconfig kubeconfig get machines
$ kubectl --kubeconfig kubeconfig get machines -o yaml
```

#### Scaling your cluster

**NOT YET SUPPORTED!**

#### Upgrading your cluster

**NOT YET SUPPORTED!**

#### Node repair

**NOT YET SUPPORTED!**

### Deleting a cluster

**NOT YET SUPPORTED!**

## Contributing

If you are interested in adding to this project, see the [contributing guide](CONTRIBUTING.md) for information on how you can get involved.
