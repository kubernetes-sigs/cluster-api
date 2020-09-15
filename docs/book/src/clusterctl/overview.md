# Overview of clusterctl

The `clusterctl` CLI tool handles the lifecycle of a Cluster API [management cluster].

The `clusterctl` command line interface is specifically designed for providing a simple "day 1 experience" and a
quick start with Cluster API; it automates fetching the YAML files defining [provider components] and installing them.

Additionally it encodes a set of best practices in managing providers, that helps the user in avoiding
mis-configurations or in managing day 2 operations such as upgrades.

* use [`clusterctl init`](commands/init.md) to install Cluster API providers
* use [`clusterctl upgrade`](commands/upgrade.md) to upgrade Cluster API providers
* use [`clusterctl delete`](commands/delete.md) to delete Cluster API providers

* use [`clusterctl config cluster`](commands/config-cluster.md) to spec out workload clusters
* use [`clusterctl generate yaml`](commands/generate-yaml.md) to process yaml
* use [`clusterctl get kubeconfig`](commands/get-kubeconfig.md) to get the kubeconfig of an existing workload cluster.
  using clusterctl's internal yaml processor.
* use [`clusterctl move`](commands/move.md) to migrate objects defining a workload clusters (e.g. Cluster, Machines) from a management cluster to another management cluster

<!-- links -->
[management cluster]: ../reference/glossary.md#management-cluster
[provider components]: ../reference/glossary.md#provider-components
