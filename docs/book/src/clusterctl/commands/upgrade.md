# clusterctl upgrade

The `clusterctl upgrade` command can be used to upgrade the version of the Cluster API providers (CRDs, controllers)
installed into a management cluster.

## Background info: management groups

The upgrade procedure is designed to ensure all the providers in a *management group* use the same
API Version of Cluster API (contract), e.g. the v1alpha 3 Cluster API contract.

A management group is a group of providers composed by a CoreProvider and a set of Bootstrap/ControlPlane/Infrastructure
providers watching objects in the same namespace.

Usually, in a management cluster there is only a management group, but in case of [n-core multi tenancy](init.md#multi-tenancy) 
there can be more than one.

# upgrade plan

The `clusterctl upgrade plan` command can be used to identify possible targets for upgrades.


```shell
clusterctl upgrade plan
```

Produces an output similar to this:

```shell
Checking new release availability...

Management group: capi-system/cluster-api, latest release available for the v1alpha3 API Version of Cluster API (contract):

NAME                NAMESPACE                       TYPE                     CURRENT VERSION   TARGET VERSION
kubeadm-bootstrap   capi-kubeadm-bootstrap-system   BootstrapProvider        v0.3.0            v0.3.1
cluster-api         capi-system                     CoreProvider             v0.3.0            v0.3.1
docker              capd-system                     InfrastructureProvider   v0.3.0            v0.3.1


You can now apply the upgrade by executing the following command:

   clusterctl upgrade apply --management-group capi-system/cluster-api  --contract v1alpha3
```

The output contains the latest release available for each management group in the cluster/for each API Version of Cluster API (contract)
available at the moment.

# upgrade apply

After choosing the desired option for the upgrade, you can run the provided command.

```shell
clusterctl upgrade apply --management-group capi-system/cluster-api  --cluster-api-version v1alpha3
```

The upgrade process is composed by two steps:

* Delete the current version of the provider components, while preserving the namespace where the provider components 
  are hosted and the provider's CRDs.
* Install the new version of the provider components.

Please note that clusterctl does not upgrade Cluster API objects (Clusters, MachineDeployments, Machine etc.); upgrading 
such objects are the responsibility of the provider's controllers.

<aside class="note warning">

<h1>Warning!</h1>

The current implementation of the upgrade process does not preserve controllers flags that are not set through the
components YAML/at the installation time.

User is required to re-apply flag values after the upgrade completes.

</aside>
