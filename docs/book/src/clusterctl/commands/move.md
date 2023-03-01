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

```bash
clusterctl move --to-kubeconfig="path-to-target-kubeconfig.yaml"
```

To move the Cluster API objects existing in the current namespace of the source management cluster; in case if you want
to move the Cluster API objects defined in another namespace, you can use the `--namespace` flag.

<aside class="note">

<h1> Pause Reconciliation </h1>

Before moving a `Cluster`, clusterctl sets the `Cluster.Spec.Paused` field to `true` stopping
the controllers from reconciling the workload cluster _in the source management cluster_.

The `Cluster` object created in the target management cluster instead will be actively reconciled as soon as the move
process completes.

</aside>

<aside class="note warning">

<h1> Warning </h1>

`clusterctl move` has been designed and developed around the bootstrap use case described below, and currently this is the only
use case verified by Cluster API E2E tests.

If someone intends to use `clusterctl move` outside of this scenario, it's recommended to set up a custom validation pipeline of
it before using the command on a production environment.

Also, it is important to notice that move has not been designed for being used as a backup/restore solution and it has 
several limitation for this scenario, like e.g. the implementation assumes the cluster must be stable
while doing the move operation, and possible race conditions happening while the cluster is upgrading, scaling up, 
remediating etc. has never been investigated nor addressed.

In order to avoid further confusion about this point, `clusterctl backup` and `clusterctl restore` commands have been
removed because they were built on top of `clusterctl move` logic and they were sharing the same limitations.
User can use `clusterctl move --to-directory` and `clusterctl move --from-directory` instead; this will hopefully
make it clear those operation have the same limitations of the move command.

</aside>

<aside class="note warning">

<h1> Warning: Status subresource is never restored </h1>

Every object's `Status` subresource, including every nested field (e.g. `Status.Conditions`), is never restored during a `move` operation. A `Status` subresource should never contain fields that cannot be recreated or derived from information in spec, metadata, or external systems.
Provider implementers should not store non-ephemeral data in the `Status`. 
`Status` should be able to be fully rebuilt by controllers by observing the current state of resources.

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

1. Create a temporary bootstrap cluster, e.g. using kind or minikube
2. Use `clusterctl init` to install the provider components
3. Use `clusterctl generate cluster ... | kubectl apply -f -` to provision a target management cluster
4. Wait for the target management cluster to be up and running
5. Get the kubeconfig for the new target management cluster
6. Use `clusterctl init` with the new cluster's kubeconfig to install the provider components
7. Use `clusterctl move` to move the Cluster API resources from the bootstrap cluster to the target management cluster
8. Delete the bootstrap cluster

> Note: It's required to have at least one worker node to schedule Cluster API workloads (i.e. controllers).
> A cluster with a single control plane node won't be sufficient due to the `NoSchedule` taint. If a worker node isn't available, `clusterctl init` will timeout.

## Dry run

With `--dry-run` option you can dry-run the move action by only printing logs without taking any actual actions. Use log level verbosity `-v` to see different levels of information.
