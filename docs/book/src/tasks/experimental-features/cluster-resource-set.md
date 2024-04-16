# Experimental Feature: ClusterResourceSet (beta)

The `ClusterResourceSet` feature is introduced to provide a way to automatically apply a set of resources (such as CNI/CSI) defined by users to matching newly-created/existing clusters.

**Feature gate name**: `ClusterResourceSet`

**Variable name to enable/disable the feature gate**: `EXP_CLUSTER_RESOURCE_SET`

The `ClusterResourceSet` feature is enabled by default, but can be disabled by setting the `EXP_CLUSTER_RESOURCE_SET` environment variable to `false`.

More details on `ClusterResourceSet` can be found at:
[ClusterResourceSet CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20200220-cluster-resource-set.md)

## Example

Suppose you want to automatically install the relevant external cloud provider on all workload clusters.
This can be accomplished by labeling the clusters with the specific cloud (e.g. AWS, GCP or OpenStack) and then creating a `ClusterResourceSet` for each.
For example, you could have the following for OpenStack:

```yaml
apiVersion: addons.cluster.x-k8s.io/v1beta1
kind: ClusterResourceSet
metadata:
  name: cloud-provider-openstack
  namespace: default
spec:
  strategy: Reconcile
  clusterSelector:
    matchLabels:
      cloud: openstack
  resources:
    - name: cloud-provider-openstack
      kind: ConfigMap
    - name: cloud-config
      kind: Secret
```

This `ClusterResourceSet` would apply the content of the `Secret` `cloud-config` and of the `ConfigMap` `cloud-provider-openstack` in all workload clusters with the label `cloud=openstack`.
Suppose you have the file `cloud.conf` that should be included in the `Secret` and `cloud-provider-openstack.yaml` that should be in the `ConfigMap`.
The `Secret` and `ConfigMap` can then be created in the following way:

```bash
kubectl create secret generic cloud-config --from-file=cloud.conf --type=addons.cluster.x-k8s.io/resource-set
kubectl create configmap cloud-provider-openstack --from-file=cloud-provider-openstack.yaml
```

Note that it is required that the `Secret` has the type `addons.cluster.x-k8s.io/resource-set` for it to be picked up.

## Update from `ApplyOnce` to `Reconcile`

The `strategy` field is immutable so existing CRS can't be updated directly. However, CAPI won't delete the managed resources in the target cluster when the CRS is deleted.
So if you want to start using the `Reconcile` strategy, delete your existing CRS and create it again with the updated `strategy`.
