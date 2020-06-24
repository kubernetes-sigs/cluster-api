# clusterctl delete

The `clusterctl delete` command deletes the provider components from the management cluster.

The operation is designed to prevent accidental deletion of user created objects. For example:

```shell
clusterctl delete --infrastructure aws
```

Deletes the AWS infrastructure provider components, while preserving the namespace where the provider components are hosted and
the provider's CRDs.

<aside class="note warning">

<h1>Warning</h1>

If you want to delete the namespace where the provider components are hosted, you can use the `--include-namespace` flag.

Be aware that this operation deletes all the object existing in a namespace, not only the provider's components.

</aside> 

<aside class="note warning">

<h1>Warning</h1>

If you want to delete the provider's CRDs, and all the components related to CRDs like e.g. the ValidatingWebhookConfiguration etc.,
you can use the `--include-crd` flag.

Be aware that this operation deletes all the object of Kind defined in the provider's CRDs, e.g. when deleting
the aws provider, it deletes all the `AWSCluster`, `AWSMachine` etc.

</aside>

<aside class="note warning">

<h1>Warning</h1>

KNOWN BUG:

Deleting an infrastructure component from a namespace _that share
the same prefix_ with other namespaces (e.g. `foo` and `foo-bar`) will result
in erroneous deletion of cluster scoped objects such as `ClusterRole` and
`ClusterRoleBindings` that share the same namespace prefix.

This is true if the prefix before a dash `-` is same. That is, namespaces such
as `foo` and `foobar` are fine however namespaces such as `foo` and `foo-bar`
are not.

For example,

1. Init an infrastructure provider in namespace `foo` and `foo-bar`.
    ```
    clusterctl init --infrastructure aws --watching-namespace foo --target-namespace foo
    clusterctl init --infrastructure aws --watching-namespace foo-bar --target-namespace foo-bar
    ```
1. Delete infrastructure components from namespace `foo`
    ```
    clusterctl delete --infrastructure aws --namespace foo
    ```

ClusterRole and ClusterRoleBindings for both the namespaces are deleted.

See [issue 3119] for more details.

</aside>

If you want to delete all the providers in a single operation , you can use the `--all` flag.

```shell
clusterctl delete --all
```
[issue 3119]: https://github.com/kubernetes-sigs/cluster-api/issues/3119
