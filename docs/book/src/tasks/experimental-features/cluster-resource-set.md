# Experimental Feature: ClusterResourceSet (alpha)

The `ClusterResourceSet` feature is introduced to provide a way to automatically apply a set of resources (such as CNI/CSI) defined by users to matching newly-created/existing clusters.

**Feature gate name**: `ClusterResourceSet`

**Variable name to enable/disable the feature gate**: `EXP_CLUSTER_RESOURCE_SET`

More details on `ClusterResourceSet` and an example to test it can be found at:
[ClusterResourceSet CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20200220-cluster-resource-set.md)
