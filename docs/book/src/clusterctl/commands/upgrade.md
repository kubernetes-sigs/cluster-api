# clusterctl upgrade

The `clusterctl upgrade` command can be used to upgrade the version of the Cluster API providers (CRDs, controllers)
installed into a management cluster.

# upgrade plan

The `clusterctl upgrade plan` command can be used to identify possible targets for upgrades.

```bash
clusterctl upgrade plan
```

Produces an output similar to this:

```bash
Checking cert-manager version...
Cert-Manager will be upgraded from "v1.5.0" to "v1.5.3"

Checking new release availability...

Management group: capi-system/cluster-api, latest release available for the v1beta1 API Version of Cluster API (contract):

NAME                    NAMESPACE                           TYPE                     CURRENT VERSION   NEXT VERSION
bootstrap-kubeadm       capi-kubeadm-bootstrap-system       BootstrapProvider        v0.4.0           v1.0.0
control-plane-kubeadm   capi-kubeadm-control-plane-system   ControlPlaneProvider     v0.4.0           v1.0.0
cluster-api             capi-system                         CoreProvider             v0.4.0           v1.0.0
infrastructure-docker   capd-system                         InfrastructureProvider   v0.4.0           v1.0.0
```

You can now apply the upgrade by executing the following command:

```bash
   clusterctl upgrade apply --contract v1beta1
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

```bash
clusterctl upgrade apply --contract v1beta1
```

The upgrade process is composed by three steps:

* Check the cert-manager version, and if necessary, upgrade it.
* Delete the current version of the provider components, while preserving the namespace where the provider components
  are hosted and the provider's CRDs.
* Install the new version of the provider components.

Please note that clusterctl does not upgrade Cluster API objects (Clusters, MachineDeployments, Machine etc.); upgrading
such objects are the responsibility of the provider's controllers.

It is also possible to explicitly upgrade one or more components to specific versions.

```bash
clusterctl upgrade apply \
    --core cluster-api:v1.2.4 \
    --infrastructure docker:v1.2.4
```

<aside class="note warning">

<h1>Clusterctl upgrade test coverage</h1>

Cluster API only tests a subset of possible clusterctl upgrade paths as otherwise the test matrix would be overwhelming.
Untested upgrade paths are not blocked by clusterctl and should work in general, they are just not tested. Users 
intending to use an upgrade path not tested by us should do their own validation to ensure the operation works correctly.

The following is an example of the tested upgrade paths while v1.4 is being developed:

| From | To     | Note                                                                                                                                                                                    |
|------|--------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| v0.3 | latest | v0.3 is the latest supported minor release with the v1alpha3 contract (v0.3 is EOL since 2022-02-23). This test will be removed once the v1alpha3 apiVersion has been entirely removed. |
| v0.4 | latest | v0.4 is the latest supported minor release with the v1alpha4 contract (v0.4 is EOL since 2022-04-06). This test will be removed once the v1alpha4 apiVersion has been entirely removed. |
| v1.0 | latest | v1.0 is the first release with the v1beta1 contract.                                                                                                                                    |
| v1.2 | latest | v1.2 is a currently supported release. This test will be removed when v1.4 is released and a new test for v1.4 is added.                                                                |
| v1.3 | latest | v1.3 is a currently supported release. This test will be removed when v1.5 is released and a new test for v1.5 is added.                                                                |

</aside>

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

```bash
clusterctl upgrade apply \
    --core cluster-api:v1.0.0 \
    --bootstrap kubeadm:v1.0.0 \
    --control-plane kubeadm:v1.0.0 \
    --infrastructure docker:v1.0.0-rc.0
```

In this case, all the provider's versions must be explicitly stated.

</aside>
