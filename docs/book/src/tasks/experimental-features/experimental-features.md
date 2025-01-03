# Experimental Features

Cluster API now ships with a new experimental package that lives under the `exp/` directory. This is a
temporary location for features which will be moved to their permanent locations after graduation. Users can experiment with these features by enabling them using feature gates.

Currently Cluster API has the following experimental features:
* `ClusterResourceSet` (env var: `EXP_CLUSTER_RESOURCE_SET`): [ClusterResourceSet](./cluster-resource-set.md)
* `MachinePool` (env var: `EXP_MACHINE_POOL`): [MachinePools](./machine-pools.md)
* `MachineSetPreflightChecks` (env var: `EXP_MACHINE_SET_PREFLIGHT_CHECKS`): [MachineSetPreflightChecks](./machineset-preflight-checks.md)
* `PriorityQueue` (env var: `EXP_PRIORITY_QUEUE`): Enables the usage of the controller-runtime PriorityQueue: https://github.com/kubernetes-sigs/controller-runtime/issues/2374
* `MachineWaitForVolumeDetachConsiderVolumeAttachments` (env var: `EXP_MACHINE_WAITFORVOLUMEDETACH_CONSIDER_VOLUMEATTACHMENTS`):
  * During Machine drain the Machine controller waits for volumes to be detached. Per default, the controller considers
    `Nodes.status.volumesAttached` and `VolumesAttachments`. This feature flag allows to opt-out from considering `VolumeAttachments`.
    The feature gate was added to allow to opt-out in case unforeseen issues occur with `VolumeAttachments`.
* `ClusterTopology` (env var: `CLUSTER_TOPOLOGY`): [ClusterClass](./cluster-class/index.md)
* `RuntimeSDK` (env var: `EXP_RUNTIME_SDK`): [RuntimeSDK](./runtime-sdk/index.md)
* `KubeadmBootstrapFormatIgnition` (env var: `EXP_KUBEADM_BOOTSTRAP_FORMAT_IGNITION`): [Ignition](./ignition.md)

## Enabling Experimental Features for Management Clusters Started with clusterctl

Users can enable/disable features by setting OS environment variables before running `clusterctl init`, e.g.:

```yaml
export EXP_SOME_FEATURE_NAME=true

clusterctl init --infrastructure vsphere
```

As an alternative to environment variables, it is also possible to set variables in the clusterctl config file located at `$XDG_CONFIG_HOME/cluster-api/clusterctl.yaml`, e.g.:
```yaml
# Values for environment variable substitution
EXP_SOME_FEATURE_NAME: "true"
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
  CLUSTER_TOPOLOGY: "true"
  EXP_RUNTIME_SDK: "true"
  EXP_MACHINE_SET_PREFLIGHT_CHECKS: "true"
```

Another way is to set them as environmental variables before running e2e tests.

## Enabling Experimental Features on Tilt

On development environments started with `Tilt`, features can be enabled by setting the feature variables in `kustomize_substitutions`, e.g.:

```yaml
kustomize_substitutions:
  CLUSTER_TOPOLOGY: 'true'
  EXP_RUNTIME_SDK: 'true'
  EXP_MACHINE_SET_PREFLIGHT_CHECKS: 'true'
```

For more details on setting up a development environment with `tilt`, see [Developing Cluster API with Tilt](../../developer/core/tilt.md)

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
