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

NAME                NAMESPACE                          TYPE                     CURRENT VERSION   TARGET VERSION
cluster-api         capi-system                        CoreProvider             v0.3.0            v0.3.1
kubeadm             capi-kubeadm-bootstrap-system      BootstrapProvider        v0.3.0            v0.3.1
kubeadm             capi-kubeadm-control-plane-system  ControlPlaneProvider     v0.3.0            v0.3.1
docker              capd-system                        InfrastructureProvider   v0.3.0            v0.3.1


You can now apply the upgrade by executing the following command:

   clusterctl upgrade apply --management-group capi-system/cluster-api  --contract v1alpha3
```

The output contains the latest release available for each management group in the cluster/for each API Version of Cluster API (contract)
available at the moment.

<aside class="note">

<h1> Pre-release provider versions </h1>

`clusterctl upgrade plan` does not display pre-release versions by default. For
example, if a provider has releases `v0.7.0-alpha.0` and `v0.6.6` available, the latest
release availble for upgrade will be `v0.6.6`.

</aside>

# upgrade apply

After choosing the desired option for the upgrade, you can run the following
command to upgrade all the providers in the management group. This upgrades
all the providers to the latest stable releases.

```shell
clusterctl upgrade apply \
  --management-group capi-system/cluster-api  \
  --contract v1alpha3
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
clusterctl upgrade apply --management-group capi-system/cluster-api \
    --core capi-system/cluster-api:v0.3.1 \
    --bootstrap capi-kubeadm-bootstrap-system/kubeadm:v0.3.1 \
    --control-plane capi-kubeadm-control-plane-system/kubeadm:v0.3.1 \
    --infrastructure capv-system/vsphere:v0.7.0-alpha.0
```

In this case, all the provider's versions must be explicitly stated.

</aside>

## Upgrading a Multi-tenancy management cluster

[Multi-tenancy](init.md#multi-tenancy) for Cluster API means a management cluster where multiple instances of the same
provider are installed, and this is achieved by multiple calls to `clusterctl init`, and in most cases, each one with
different environment variables for customizing the provider instances.

In order to upgrade a multi-tenancy management cluster, and preserve the instance specific settings, you should do
the same during upgrades and execute multiple calls to `clusterctl upgrade apply`, each one with different environment
variables.

For instance, in case of a management cluster with n>1 instances of an infrastructure provider, and only one instance
of Cluster API core provider, bootstrap provider and control plane provider, you should:

Run once `clusterctl upgrade apply` for the core provider, the bootstrap provider and the control plane provider;
this can be achieved by using the `--core`, `--bootstrap` and `--control-plane` flags followed by the upgrade target
for each one of those providers, e.g.

```shell
clusterctl upgrade apply --management-group capi-system/cluster-api \
    --core capi-system/cluster-api:v0.3.1 \
    --bootstrap capi-kubeadm-bootstrap-system/kubeadm:v0.3.1 \
    --control-plane capi-kubeadm-control-plane-system/kubeadm:v0.3.1
```

Run `clusterctl upgrade apply` for each infrastructure provider instance, using the `--infrastructure` flag,
taking care to provide different environment variables for each call (as in the initial setup), e.g.

Set the environment variables for instance 1 and then run:

```shell
clusterctl upgrade apply --management-group capi-system/cluster-api \
    --infrastructure instance1/docker:v0.3.1
```

Afterwards, set the environment variables for instance 2 and then run:

```shell
clusterctl upgrade apply --management-group capi-system/cluster-api \
    --infrastructure instance2/docker:v0.3.1
```

etc.

<aside class="note warning">

<h1>tips</h1>

As alternative of using multiple set of env variables it is possible to use
multiple config files and pass them to the different `clusterctl upgrade apply` calls
using the `--config` flag.

</aside>
