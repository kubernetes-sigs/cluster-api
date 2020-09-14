# clusterctl move

The `clusterctl move` command allows to move the Cluster API objects defining workload clusters, like e.g. Cluster, Machines,
MachineDeployments, etc. from one management cluster to another management cluster.

<aside class="note warning">

<h1> Warning </h1>

Before running `clusterctl move`, the user should take care of preparing the target management cluster, including also installing
all the required provider using `clusterctl init`.
 
The version of the providers installed in the target management cluster should be at least the same version of the
corresponding provider in the source cluster.

</aside>

You can use:

```shell
clusterctl move --to-kubeconfig="path-to-target-kubeconfig.yaml"
```

To move the Cluster API objects existing in the current namespace of the source management cluster; in case if you want
to move the Cluster API objects defined in another namespace, you can use the `--namespace` flag.

<aside class="note">

<h1> Pause Reconciliation </h1>

Before moving a `Cluster`, clusterctl sets the `Cluster.Spec.Paused` field to `true` stopping
the controllers to reconcile the workload cluster _in the source management cluster_.

The `Cluster` object created in the target management cluster instead will be actively reconciled as soon as the move
process completes. 

</aside>

## Pivot

Pivoting is a process for moving the provider components and declared Cluster API resources from a source management
cluster to a target management cluster.

This can now be achieved with the following procedure:

1. Use `clusterctl init` to install the provider components into the target management cluster
2. Use `clusterctl move` to move the cluster-api resources from a Source Management cluster to a Target Management cluster

## Bootstrap & Pivot

The pivot process can be bounded with the creation of a temporary bootstrap cluster
used to provision a target Management cluster.

This can now be achieved with the following procedure:

1. Create a temporary bootstrap cluster, e.g. using Kind or Minikube
2. Use `clusterctl init` to install the provider components
3. Use `clusterctl config cluster ... | kubectl apply -f -` to provision a target management cluster
4. Wait for the target management cluster to be up and running
5. Get the kubeconfig for the new target management cluster
6. Use `clusterctl init` with the new cluster's kubeconfig to install the provider components 
7. Use `clusterctl move` to move the Cluster API resources from the bootstrap cluster to the target management cluster
8. Delete the bootstrap cluster

> Note: It's required to have at least one worker node to schedule Cluster API workloads (i.e. controllers).
> A cluster with a single control plane node won't be sufficient due to the `NoSchedule` taint. If a worker node isn't available, `clusterctl init` will timeout.

## Dry run

With `--dry-run` option you can dry-run the move action by only printing logs without taking any actual actions. Use log level verbosity `-v` to see different levels of information.
