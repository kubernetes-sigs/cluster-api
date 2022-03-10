# Experimental Feature: ClusterClass (alpha)

The ClusterClass feature introduces a new way to create clusters which reduces boilerplate and enables flexible and powerful customization of clusters.
ClusterClass is a powerful abstraction implemented on top of existing interfaces and offers a set of tools and operations to streamline cluster lifecycle management while maintaining the same underlying API.

**Feature gate name**: `ClusterTopology`

**Variable name to enable/disable the feature gate**: `CLUSTER_TOPOLOGY`

Additional documentation:
* Background information:  [ClusterClass and Managed Topologies CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20210526-cluster-class-and-managed-topologies.md)
* For ClusterClass authors:
    * [Writing a ClusterClass](./write-clusterclass.md)
    * [Changing a ClusterClass](./change-clusterclass.md)
    * Publishing a ClusterClass for clusterctl usage: [clusterctl Provider contract]
* For Cluster operators:
    * Creating a Cluster: [Quick Start guide]
        Please note that the experience for creating a Cluster using ClusterClass is very similar to the one for creating a standalone Cluster. Infrastructure providers supporting ClusterClass provide Cluster templates leveraging this feature (e.g the Docker infrastructure provider has a development-topology template).
    * [Operating a managed Cluster](./operate-cluster.md)
    * Planning topology rollouts: [clusterctl alpha topology plan]

<!-- links -->
[Quick Start guide]: ../../../user/quick-start.md
[clusterctl Provider contract]: ../../../clusterctl/provider-contract.md
[clusterctl alpha topology plan]: ../../../clusterctl/commands/alpha-topology-plan.md
