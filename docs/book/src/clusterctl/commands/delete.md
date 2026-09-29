# clusterctl delete

The `clusterctl delete` command deletes the provider components from the management cluster.

The operation is designed to prevent accidental deletion of user created objects. For example:

```bash
clusterctl delete --infrastructure aws
```

This command deletes the AWS infrastructure provider components, while preserving
the namespace where the provider components are hosted and the provider's CRDs.


If you want to delete the namespace where the provider components are hosted, you can use the `--include-namespace` flag.

> [!CAUTION]
> Be aware that `--include-namespace` will delete all the object existing in a namespace, not only the provider's components.

If you want to delete the provider's CRDs, and all the components related to CRDs like e.g. the ValidatingWebhookConfiguration etc.,
you can use the `--include-crd` flag.

> [!CAUTION]
> Be aware that `--include-crd` will delete all the objects of Kind's defined in the provider's CRDs, e.g. when deleting
> the aws provider, it deletes all the `AWSCluster`, `AWSMachine` etc.

If you want to delete all the providers in a single operation, you can use the `--all` flag.

```bash
clusterctl delete --all
```
[issue 3119]: https://github.com/kubernetes-sigs/cluster-api/issues/3119
