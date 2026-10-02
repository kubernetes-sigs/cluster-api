# Feature Gates

The following tables are a summary of the feature gates that are available in Cluster API.

| Feature gate                     | Maturity level | Note                                                                                                                                                                                                                                                                                                                                 |
|----------------------------------|----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `ClusterClassInlineTemplates`    | Alpha          | env var: `EXP_CLUSTERCLASS_INLINE_TEMPLATES`<br>See this [ClusterClass with inline templates](../user/guide/configuration/cluster-class/write-clusterclass.md#clusterclass-with-inline-templates).<br>Important!: This feature gate should only be enabled if there are no more clients in the environment that are using the v1beta1 ClusterClass API. |
| `ClusterTopology`                | Alpha          | env var: `CLUSTER_TOPOLOGY`<br>See [ClusterClass](../user/guide/configuration/cluster-class/index.md).                                                                                                                                                                                                                                                  |
| `InPlaceUpdates`                 | Alpha          | env var: `EXP_IN_PLACE_UPDATES`<br>See [proposal](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240807-in-place-updates.md).<br>Important!: Also requires `RuntimeSDK`.                                                                                                                                  |
| `KubeadmBootstrapFormatIgnition` | Alpha          | env var: `EXP_KUBEADM_BOOTSTRAP_FORMAT_IGNITION`<br>See [Ignition](../user/guide/configuration/bootstrap/kubeadm-bootstrap/ignition.md).                                                                                                                                                                                                                                            |
| `MachinePool`                    | Beta           | env var: `EXP_MACHINE_POOL`<br>See [MachinePools](../user/guide/lifecycle/machine-management/machine-pools.md).                                                                                                                                                                                                                                                        |
| `MachineSetPreflightChecks`      | Beta           | env var: `EXP_MACHINE_SET_PREFLIGHT_CHECKS`<br>See [MachineSetPreflightChecks](../user/guide/lifecycle/machine-management/machineset-preflight-checks.md).                                                                                                                                                                                                             |
| `MachineTaintPropagation`        | Alpha          | env var: `EXP_MACHINE_TAINT_PROPAGATION`<br>See [Taint propagation](api/taint-propagation.md).                                                                                                                                                                                                                                       |
| `PriorityQueue`                  | GA             | env var: `EXP_PRIORITY_QUEUE`<br>See [issue](https://github.com/kubernetes-sigs/controller-runtime/issues/2374).                                                                                                                                                                                                                     |
| `ReconcilerRateLimiting`         | GA             | env var: `EXP_RECONCILER_RATE_LIMITING`<br>See [issue](https://github.com/kubernetes-sigs/cluster-api/issues/13005).<br>Important!: starting from CAPI v1.12.4 `ReconcilerRateLimiting` also requires `PriorityQueue`.                                                                                                               |
| `RuntimeSDK`                     | Alpha          | env var: `EXP_RUNTIME_SDK`<br>See [Runtime extensions](../developer/runtime-extensions/index.md).                                                                                                                                                                                                                                    |

## Enabling Feature Gates for Management Clusters Started with clusterctl

Users can enable/disable features gates by setting OS environment variables before running `clusterctl init`, e.g.:

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
For more information on how to set variables for clusterctl, see [clusterctl Configuration File](clusterctl/configuration.md)

Some features like `MachinePools` may require infrastructure providers to implement a separate CRD that handles the infrastructure side of the feature too.
For such a feature to work, infrastructure providers should also enable their controllers if it is implemented as a feature. If it is not implemented as a feature, no additional step is necessary.
As an example, Cluster API Provider Azure (CAPZ) has support for MachinePool through the infrastructure type `AzureMachinePool`.

## Enabling Feature Gates for e2e Tests

One way to enable fature gates for E2E tests it to set environment variables on the clusterctl config file used
to boostrap the management cluster used during the test. For CAPI, these configs are under ./test/e2e/config/... such as `docker.yaml`:

```yaml
variables:
  CLUSTER_TOPOLOGY: "true"
  EXP_RUNTIME_SDK: "true"
  EXP_MACHINE_SET_PREFLIGHT_CHECKS: "true"
```

Another way is to set them as environmental variables before running e2e tests.

## Enabling Feature Gates on Tilt

On development environments started with `Tilt`, features gates can be enabled by setting the feature variables in `kustomize_substitutions`, e.g.:

```yaml
kustomize_substitutions:
  CLUSTER_TOPOLOGY: 'true'
  EXP_RUNTIME_SDK: 'true'
  EXP_MACHINE_SET_PREFLIGHT_CHECKS: 'true'
```

For more details on setting up a development environment with `tilt`, see [Developing Cluster API with Tilt](../developer/core/tilt.md)

## Enabling Feature Gates on Existing Management Clusters

To enable/disable features gates on existing management clusters, users can edit the corresponding controller manager
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

Following controller manager deployments have to be edited in order to enable/disable their respective feature gates:

* [ClusterClass](../user/guide/configuration/cluster-class/index.md):
  * [CAPI](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#capi).
  * [KCP](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#kcp).
  * [CAPD](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Providers#capd). Other [Infrastructure Providers](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Providers#infrastructure-provider)
    might also require this. Please consult the docs of the concrete [Infrastructure Provider](https://cluster-api.sigs.k8s.io/reference/providers#infrastructure)
    regarding this.
* [Ignition Bootstrap configuration](../user/guide/configuration/bootstrap/kubeadm-bootstrap/ignition.md):
  * [CABPK](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#cabpk).
  * [KCP](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#kcp).
* [MachinePools](../user/guide/lifecycle/machine-management/machine-pools.md):
  * [CAPI](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#capi).
  * [CABPK](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#cabpk).
  * [CAPD](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Providers#capd). Other [Infrastructure Providers](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Providers#infrastructure-provider)
    might also require this. Please consult the docs of the concrete [Infrastructure Provider](https://cluster-api.sigs.k8s.io/reference/providers#infrastructure)
    regarding this.
* [Runtime SDK](../developer/runtime-extensions/index.md):
  * [CAPI](https://cluster-api.sigs.k8s.io/reference/glossary.html?highlight=Gloss#capi).
