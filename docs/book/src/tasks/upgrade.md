# Upgrading from Cluster API v1alpha2 to Cluster API v1alpha3

We will be using the [clusterctl init] command to upgrade an existing [management cluster] from `v1alpha2` to `v1alpha3`. 

For detailed information about the changes from `v1alpha2` to `v1alpha3`, please refer to the [Cluster API v1alpha2 compared to v1alpha3 section].

## Prerequisites

There are a few preliminary steps needed to be able to run `clusterctl init` on a [management cluster] with `v1alpha2` [components] installed.

### Delete the cabpk-system namespace

<aside class="note warning">

<h1>Warning</h1>

Please proceed with caution and ensure you do not have any additional components deployed on the namespace.

</aside>

Delete the `cabpk-system` namespace by running: 

```bash
kubectl delete namespace cabpk-system
```

### Delete the core and infrastructure provider controller-manager deployments

Delete the `capi-controller-manager` deployment from the `capi-system` namespace:

```bash
kubectl delete deployment capi-controller-manager -n capi-system 
```

Depending on your infrastructure provider, delete the controller-manager deployment. 

For example, if you are using the [AWS provider], delete the `capa-controller-manager` deployment from the `capa-system` namespace:

```bash
kubectl delete deployment capa-controller-manager -n capa-system 
```

### Optional: Ensure preserveUnknownFields is set to 'false' for the infrastructure provider CRDs Spec
This should be the case for all infrastructure providers using conversion webhooks to allow upgrading from `v1alpha2` to
`v1alpha3`.

This can verified by running `kubectl get crd <crd name>.infrastructure.cluster.x-k8s.io -o yaml` for all the
infrastructure provider CRDs. 

## Upgrade the management cluster using clusterctl

Run [clusterctl init] with the relevant infrastructure flag. For the [AWS provider] you would run:

```bash
clusterctl init --infrastructure aws
```

You should now be able to manage your resources using the `v1alpha3` version of the Cluster API components.

<!-- links -->
[components]: ../reference/glossary.md#provider-components
[management cluster]: ../reference/glossary.md#management-cluster
[AWS provider]: https://github.com/kubernetes-sigs/cluster-api-provider-aws
[clusterctl init]: ../clusterctl/commands/init.md
[Cluster API v1alpha2 compared to v1alpha3 section]: ../developer/providers/v1alpha2-to-v1alpha3.md