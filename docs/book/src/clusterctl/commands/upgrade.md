# clusterctl upgrade

The `clusterctl upgrade` command can be used to upgrade the version of the Cluster API providers (CRDs, controllers)
installed into a management cluster.

# upgrade plan

The `clusterctl upgrade plan` command can be used to identify possible targets for upgrades.


```shell
clusterctl upgrade plan
```

Produces an output similar to this:

```shell
Checking cert-manager version...
Cert-Manager will be upgraded from "v0.11.0" to "v1.5.0"

Checking new release availability...

Management group: capi-system/cluster-api, latest release available for the v1alpha4 API Version of Cluster API (contract):

NAME                    NAMESPACE                           TYPE                     CURRENT VERSION   NEXT VERSION
bootstrap-kubeadm       capi-kubeadm-bootstrap-system       BootstrapProvider        v0.4.0           v0.4.1
control-plane-kubeadm   capi-kubeadm-control-plane-system   ControlPlaneProvider     v0.4.0           v0.4.1
cluster-api             capi-system                         CoreProvider             v0.4.0           v0.4.1
infrastructure-azure    capz-system                         InfrastructureProvider   v0.4.0           v0.4.1


You can now apply the upgrade by executing the following command:

   clusterctl upgrade apply --contract v1alpha4
```

The output contains the latest release available for each API Version of Cluster API (contract)
available at the moment.

<aside class="note">

<h1> Pre-release provider versions </h1>

`clusterctl upgrade plan` does not display pre-release versions by default. For
example, if a provider has releases `v0.7.0-alpha.0` and `v0.6.6` available, the latest
release available for upgrade will be `v0.6.6`.

</aside>

# upgrade apply

After choosing the desired option for the upgrade, you can run the following
command to upgrade all the providers in the management cluster. This upgrades
all the providers to the latest stable releases.

```shell
clusterctl upgrade apply --contract v1alpha4
```

The upgrade process is composed by three steps:

* Check the cert-manager version, and if necessary, upgrade it.
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

<aside class="note warning">

<h1> Upgrading to pre-release provider versions </h1>

In order to upgrade to a provider's pre-release version, we can do
the following:

```shell
clusterctl upgrade apply \
    --core capi-system/cluster-api:v0.4.1 \
    --bootstrap capi-kubeadm-bootstrap-system/kubeadm:v0.4.1 \
    --control-plane capi-kubeadm-control-plane-system/kubeadm:v0.4.1 \
    --infrastructure capv-system/vsphere:v0.7.0-alpha.0
```

In this case, all the provider's versions must be explicitly stated.

</aside>
