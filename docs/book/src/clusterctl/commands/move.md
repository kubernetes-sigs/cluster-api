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
