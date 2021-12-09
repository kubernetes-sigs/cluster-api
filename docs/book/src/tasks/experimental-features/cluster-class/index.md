# Experimental Feature: ClusterClass (alpha)

The ClusterClass feature introduces a new way to create clusters which reduces boilerplate and enables flexible and powerful customization of clusters.
ClusterClass is a powerful abstraction implemented on top of existing interfaces and offers a set of tools and operations to streamline cluster lifecycle management while maintaining the same underlying API.

**Feature gate name**: `ClusterTopology`

**Variable name to enable/disable the feature gate**: `CLUSTER_TOPOLOGY`

Additional documentation:
* For ClusterClass authors:
  * [Writing a ClusterClass](./write-clusterclass.md)
  * Publishing a ClusterClass is documented in the [clusterctl Provider contract]
* For Cluster operators:
    * Creating a Cluster is documented in the [Quick Start guide]
    * [Upgrading a Cluster](./upgrade-cluster.md)
    * [Changing a ClusterClass](./change-clusterclass.md)
* Additional background information can be found in the  [ClusterClass and Managed Topologies CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/202105256-cluster-class-and-managed-topologies.md)

<!-- links -->
[Quick Start guide]: ../../../user/quick-start.md
[clusterctl Provider contract]: ../../../clusterctl/provider-contract.md
