# Experimental Features

Cluster API now ships with a new experimental package that lives under the `exp/` directory. This is a
temporary location for features which will be moved to their permanent locations after graduation. Users can experiment with these features by enabling them using feature gates.

## Enabling Experimental Features for Management Clusters Started with clusterctl

Users can enable/disable features by setting OS environment variables before running `clusterctl init`, e.g.:

```yaml
export EXP_CLUSTER_RESOURCE_SET=true

clusterctl init --infrastructure vsphere
```

As an alternative to environment variables, it is also possible to set variables in the clusterctl config file located at `$HOME/.cluster-api/clusterctl.yaml`, e.g.:
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

One way is to set experimental variables on the clusterctl config file. For CAPI, these configs are under ./test/e2e/config/... such as `docker-ci.yaml`:
```yaml
variables:
  EXP_CLUSTER_RESOURCE_SET: "true"
  EXP_MACHINE_POOL: "true"
```
Another way is to set them as environmental variables before running e2e tests.

## Enabling Experimental Features on Tilt

On development environments started with `Tilt`, features can be enabled by setting the feature variables in `kustomize_substitutions`, e.g.:

```yaml
{
  "enable_providers": ["kubeadm-bootstrap","kubeadm-control-plane"],
  "allowed_contexts": ["kind-kind"],
  "default_registry": "gcr.io/cluster-api-provider",
  "provider_repos": [],
  "kustomize_substitutions": {
    "EXP_CLUSTER_RESOURCE_SET": "true",
    "EXP_MACHINE_POOL": "true"
  }
}
```

For more details on setting up a development environment with `tilt`, see [Developing Cluster API with Tilt](../../developer/tilt.md)

## Enabling Experimental Features on Existing Management Clusters

To enable/disable features on existing management clusters, users can modify CAPI controller manager deployment which will restart all controllers with requested features.

```
#  kubectl edit -n capi-system deployment.apps/capi-controller-manager
   // Enable/disable available feautures by modifying Args below.
    Args:
      --leader-elect
      --feature-gates=MachinePool=true,ClusterResourceSet=true
```

Similarly, to **validate** if a particular feature is enabled, see cluster-api-provider deployment arguments by:

```
# kubectl describe -n capi-system deployment.apps/capi-controller-manager
```

## Active Experimental Features

* [MachinePools](./machine-pools.md)
* [ClusterResourceSet](./cluster-resource-set.md)

**Warning**: Experimental features are unreliable, i.e., some may one day be promoted to the main repository, or they may be modified arbitrarily or even disappear altogether.
In short, they are not subject to any compatibility or deprecation promise.
