# clusterctl commands

| Command                                                                              | Description                                                                                                |
|--------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------|
| [`clusterctl alpha rollout`](alpha-rollout.md)                                       | Manages the rollout of Cluster API resources. For example: MachineDeployments.                             |
| [`clusterctl alpha topology plan`](alpha-topology-plan.md)                           | Describes the changes to a cluster topology for a given input.                                             |
| [`clusterctl backup`](additional-commands.md#clusterctl-backup)                      | Backup Cluster API objects and all their dependencies from a management cluster.                           |
| [`clusterctl completion`](completion.md)                                             | Output shell completion code for the specified shell (bash or zsh).                                        |
| [`clusterctl config`](additional-commands.md#clusterctl-config-repositories)         | Display clusterctl configuration.                                                                          |
| [`clusterctl delete`](delete.md)                                                     | Delete one or more providers from the management cluster.                                                  |
| [`clusterctl describe cluster`](describe-cluster.md)                                 | Describe workload clusters.                                                                                |
| [`clusterctl generate cluster`](generate-cluster.md)                                 | Generate templates for creating workload clusters.                                                         |
| [`clusterctl generate provider`](generate-provider.md)                               | Generate templates for provider components.                                                                |
| [`clusterctl generate yaml`](generate-yaml.md)                                       | Process yaml using clusterctl's yaml processor.                                                            |
| [`clusterctl get kubeconfig`](get-kubeconfig.md)                                     | Gets the kubeconfig file for accessing a workload cluster.                                                 |
| [`clusterctl help`](additional-commands.md#clusterctl-help)                          | Help about any command.                                                                                    |
| [`clusterctl init`](init.md)                                                         | Initialize a management cluster.                                                                           |
| [`clusterctl init list-images`](additional-commands.md#clusterctl-init-list-images)  | Lists the container images required for initializing the management cluster.                               |
| [`clusterctl move`](move.md)                                                         | Move Cluster API objects and all their dependencies between management clusters.                           |
| [`clusterctl restore`](additional-commands.md#clusterctl-restore)                    | Restore Cluster API objects from file by glob.                                                             |
| [`clusterctl upgrade plan`](upgrade.md#upgrade-plan)                                 | Provide a list of recommended target versions for upgrading Cluster API providers in a management cluster. |
| [`clusterctl upgrade apply`](upgrade.md#upgrade-apply)                               | Apply new versions of Cluster API core and providers in a management cluster.                              |
| [`clusterctl version`](additional-commands.md#clusterctl-version)                    | Print clusterctl version.                                                                                  |
