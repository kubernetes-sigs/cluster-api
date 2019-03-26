# clusterctl

`clusterctl` is the SIG-cluster-lifecycle sponsored tool that implements the Cluster API.

Read the [experience doc here](https://docs.google.com/document/d/1-sYb3EdkRga49nULH1kSwuQFf1o6GvAw_POrsNo5d8c/edit#). To gain viewing permissions, please join either the [kubernetes-dev](https://groups.google.com/forum/#!forum/kubernetes-dev) or [kubernetes-sig-cluster-lifecycle](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle) google group.

## Getting Started

**Due to the [limitations](#limitations) described below, you must currently compile and run a `clusterctl` binary
from your chosen [provider implementation](../../README.md#provider-implementations) rather than using the binary from
this repository.**


### Prerequisites

1. Install [minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/)
2. Install a [driver](https://github.com/kubernetes/minikube/blob/master/docs/drivers.md) for minikube. For Linux, we recommend kvm2. For MacOS, we recommend VirtualBox.
2. Build the `clusterctl` tool

```bash
$ git clone https://github.com/kubernetes-sigs/cluster-api $GOPATH/src/sigs.k8s.io/cluster-api
$ cd $GOPATH/src/sigs.k8s.io/cluster-api/cmd/clusterctl/
$ go build
```

### Limitations

`clusterctl` can only use a provider that is compiled in. As provider specific code has been moved out
of this repository, running the `clusterctl` binary compiled from this repository isn't particularly useful.

There is current work ongoing to rectify this issue, which centers around removing the
[`ProviderDeployer interface`](https://github.com/kubernetes-sigs/cluster-api/blob/b90c541b315ecbac096fa371b4436d60ce5715a9/clusterctl/clusterdeployer/clusterdeployer.go#L33-L40)
from the `clusterdeployer` package. The two tracking issues for removing the two functions in the interface are
https://github.com/kubernetes-sigs/cluster-api/issues/158 and https://github.com/kubernetes-sigs/cluster-api/issues/160.

### Creating a cluster

1. Create the `cluster.yaml`, `machines.yaml`, `provider-components.yaml`, and `addons.yaml` files configured for your cluster.
   See the provider specific templates and generation tools for your chosen [provider implementation](../../README.md#provider-implementations).

1. Create a cluster:

   ```shell
   ./clusterctl create cluster --provider <provider> -c cluster.yaml -m machines.yaml -p provider-components.yaml -a addons.yaml
   ```

To choose a specific minikube driver, please use the `--bootstrap-flags vm-driver=xxx` command line parameter. For example to use the kvm2 driver with clusterctl you woud add `--bootstrap-flags vm-driver=kvm2`.

Additional advanced flags can be found via help.

```shell
./clusterctl create cluster --help
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

You can scale your cluster by adding additional individual Machines, or by adding a MachineSet or MachineDeployment
and changing the number of replicas.

#### Upgrading your cluster

**NOT YET SUPPORTED!**

#### Node repair

**NOT YET SUPPORTED!**

### Deleting a cluster

When you are ready to remove your cluster, you can use clusterctl to delete the cluster:

```shell
./clusterctl delete cluster --kubeconfig kubeconfig
```

Please also check the documentation for your [provider implementation](../../README.md#provider-implementations)
to determine if any additional steps need to be taken to completely clean up your cluster.

## Contributing

If you are interested in adding to this project, see the [contributing guide](CONTRIBUTING.md) for information on how you can get involved.
