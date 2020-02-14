# clusterctl delete

The `clusterctl delete` command deletes the provider components from the management cluster.

The operation is designed to prevent accidental deletion of user created objects. For example:

```shell
clusterctl delete aws
```

Deletes the aws provider components, while preserving the namespace where the provider components are hosted and
the provider's CRDs.

<aside class="note warning">

<h1>Warning</h1>

If you want to delete the namespace where the provider components are hosted, you can use the `--delete-namespace` flag.

Be aware that this operation deletes all the object existing in a namespace, not only the provider's components.

</aside> 

<aside class="note warning">

<h1>Warning</h1>

If you want to delete the provider's CRDs, you can use the `--delete-crd` flag.

Be aware that this operation deletes all the object of Kind defined in the provider's CRDs, e.g. when deleting
the aws provider, it deletes all the `AWSCluster`, `AWSMachine` etc.

</aside> 

If you want to delete all the providers in a single operation , you can use the `--all` flag.

```shell
clusterctl delete --all
```
