# Experimental Features

Cluster API now ships with a new experimental package that lives under the `exp/` directory. This is a
temporary location for features which will be moved to their permanent locations after graduation. Users can experiment with these features by enabling them using feature gates.

## Enabling Experimental Features for Management Clusters Started with clusterctl

Users can enable/disable features by setting OS environment variables before running `clusterctl init`, e.g.:

```yaml
export EXP_CLUSTER_RESOURCE_SET=true

clusterctl init --infrastructure vsphere
```

As an alternative to environment variables, it is also possible to set variables in the clusterctl config file located at `$XDG_CONFIG_HOME/cluster-api/clusterctl.yaml`, e.g.:
```yaml
# Values for environment variable substitution
EXP_CLUSTER_RESOURCE_SET: "true"
```
In case a variable is defined in both the config file and as an OS environment variable, the environment variable takes precedence.
For more information on how to set variables for clusterctl, see [clusterctl Configuration File](../../clusterctl/configuration.md)

Some features like `MachinePools` may require infrastructure providers to implement a separate CRD that handles the infrastructure side of the feature too.
For such a feature to work, infrastructure providers should also enable their controllers if it is implemented as a feature. If it is not implemented as a feature, no additional step is necessary.
As an example, Cluster API Provider Azure (CAPZ) has support for MachinePool through the infrastructure type `AzureMachinePool`.

## Enabling Experimental Features for e2e Tests

One way is to set experimental variables on the clusterctl config file. For CAPI, these configs are under ./test/e2e/config/... such as `docker.yaml`:
```yaml
variables:
  EXP_CLUSTER_RESOURCE_SET: "true"
  EXP_MACHINE_POOL: "true"
  CLUSTER_TOPOLOGY: "true"
  EXP_RUNTIME_SDK: "true"
  EXP_MACHINE_SET_PREFLIGHT_CHECKS: "true"
```

Another way is to set them as environmental variables before running e2e tests.

## Enabling Experimental Features on Tilt

On development environments started with `Tilt`, features can be enabled by setting the feature variables in `kustomize_substitutions`, e.g.:

```yaml
kustomize_substitutions:
  EXP_CLUSTER_RESOURCE_SET: 'true'
  EXP_MACHINE_POOL: 'true'
  CLUSTER_TOPOLOGY: 'true'
  EXP_RUNTIME_SDK: 'true'
  EXP_MACHINE_SET_PREFLIGHT_CHECKS: 'true'
```

For more details on setting up a development environment with `tilt`, see [Developing Cluster API with Tilt](../../developer/tilt.md)

## Enabling Experimental Features on Existing Management Clusters

To enable/disable features on existing management clusters, users can edit the corresponding controller manager
deployments, which will then trigger a restart with the requested features. E.g. for the CAPI controller manager
deployment:

```
kubectl edit -n capi-system deployment.apps/capi-controller-manager
```
```
// Enable/disable available features by modifying Args below.
    Args:
      --leader-elect
      --feature-gates=MachinePool=true,ClusterResourceSet=true
```

Similarly, to **validate** if a particular feature is enabled, see the arguments by issuing:

```bash
kubectl describe -n capi-system deployment.apps/capi-controller-manager
```

Following controller manager deployments have to be edited in order to enable/disable their respective experimental features:

* [MachinePools](./machine-pools.md):
  * [CAPI](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#capi).
  * [CABPK](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#cabpk).
  * [CAPD](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Providers#capd). Other [Infrastructure Providers](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Providers#infrastructure-provider)
    might also require this. Please consult the docs of the concrete [Infrastructure Provider](https://cluster-api.sigs.k8s.io/reference/providers#infrastructure)
    regarding this.
* [ClusterResourceSet](./cluster-resource-set.md):
  * [CAPI](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#capi).
* [ClusterClass](./cluster-class/index.md):
  * [CAPI](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#capi).
  * [KCP](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#kcp).
  * [CAPD](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Providers#capd). Other [Infrastructure Providers](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Providers#infrastructure-provider)
    might also require this. Please consult the docs of the concrete [Infrastructure Provider](https://cluster-api.sigs.k8s.io/reference/providers#infrastructure)
    regarding this.
* [Ignition Bootstrap configuration](./ignition.md):
  * [CABPK](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#cabpk).
  * [KCP](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#kcp).
* [Runtime SDK](runtime-sdk/index.md):
  * [CAPI](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#capi).

## Active Experimental Features

* [MachinePools](./machine-pools.md)
* [ClusterResourceSet](./cluster-resource-set.md)
* [ClusterClass](./cluster-class/index.md)
* [Ignition Bootstrap configuration](./ignition.md)
* [Runtime SDK](runtime-sdk/index.md)

**Warning**: Experimental features are unreliable, i.e., some may one day be promoted to the main repository, or they may be modified arbitrarily or even disappear altogether.
In short, they are not subject to any compatibility or deprecation promise.
