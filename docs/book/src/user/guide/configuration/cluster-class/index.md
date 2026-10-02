# ClusterClass

> [!IMPORTANT]
> The `ClusterTopology` feature flag must be set to true in order to use this feature.
> See [Feature Gates](../../../../reference/feature-gates.md) for more details.

> [!TIP]
> While ClusterClass is an advanced feature, using it can greatly simplify the user experience in configuring and
> managing a cluster with Cluster API.
>
> We highly recommend that you try this feature and provide feedback.

The ClusterClass feature introduces a new way to create clusters which reduces boilerplate and enables flexible and powerful customization of clusters.
ClusterClass is a powerful abstraction implemented on top of existing abstractions and offers a set of tools and operations to streamline cluster lifecycle management while maintaining the same underlying API.

**Feature gate name**: `ClusterTopology`

**Variable name to enable/disable the feature gate**: `CLUSTER_TOPOLOGY`

Additional documentation:
* Background information: [ClusterClass and Managed Topologies CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20210526-cluster-class-and-managed-topologies.md)
* For ClusterClass authors:
    * [Writing a ClusterClass](./write-clusterclass.md)
    * [Changing a ClusterClass](./change-clusterclass.md)
    * Publishing a ClusterClass for clusterctl usage: [clusterctl Provider contract]
* For cluster operators:
    * Creating a cluster: [Quick Start guide]
        Please note that the experience for creating a cluster using ClusterClass is very similar to the one for creating a standalone cluster. Infrastructure providers supporting ClusterClass provide Cluster templates leveraging this feature (e.g the Docker infrastructure provider has a development-topology template).
    * [Operating a managed cluster](./operate-cluster.md)

<!-- links -->
[Quick Start guide]: ../../../quick-start.md
[clusterctl Provider contract]: ../../../../developer/providers/contracts/clusterctl.md
