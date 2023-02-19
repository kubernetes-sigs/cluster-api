# Experimental Feature: ClusterResourceSet (alpha)

The `ClusterResourceSet` feature is introduced to provide a way to automatically apply a set of resources (such as CNI/CSI) defined by users to matching newly-created/existing clusters.

**Feature gate name**: `ClusterResourceSet`

**Variable name to enable/disable the feature gate**: `EXP_CLUSTER_RESOURCE_SET`

More details on `ClusterResourceSet` and an example to test it can be found at:
[ClusterResourceSet CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20200220-cluster-resource-set.md)

## Update from `ApplyOnce` to `Reconcile`

The `strategy` field is immutable so existing CRS can't be updated directly. However, CAPI won't delete the managed resources in the target cluster when the CRS is deleted.
So if you want to start using the `Reconcile` strategy, delete your existing CRS and create it again with the updated `strategy`.
