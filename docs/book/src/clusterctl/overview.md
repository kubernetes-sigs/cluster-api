# Overview of clusterctl

The `clusterctl` CLI tool handles the lifecycle of a Cluster API [management cluster].

The `clusterctl` command line interface is specifically designed for providing a simple "day 1 experience" and a
quick start with Cluster API. It automates fetching the YAML files defining [provider components] and installing them.

Additionally it encodes a set of best practices in managing providers, that helps the user in avoiding
mis-configurations or in managing day 2 operations such as upgrades.

Below you can find a list of main clusterctl commands:

* [`clusterctl init`](commands/init.md) Initialize a management cluster.
* [`clusterctl upgrade plan`](commands/upgrade.md#upgrade-plan) Provide a list of recommended target versions for upgrading Cluster API providers in a management cluster.
* [`clusterctl upgrade apply`](commands/upgrade.md#upgrade-apply) Apply new versions of Cluster API core and providers in a management cluster.
* [`clusterctl delete`](commands/delete.md) Delete one or more providers from the management cluster.
* [`clusterctl generate cluster`](commands/generate-cluster.md) Generate templates for creating workload clusters.
* [`clusterctl generate yaml`](commands/generate-yaml.md) Process yaml using clusterctl's yaml processor.
* [`clusterctl get kubeconfig`](commands/get-kubeconfig.md) Gets the kubeconfig file for accessing a workload cluster.
* [`clusterctl move`](commands/move.md) Move Cluster API objects and all their dependencies between management clusters.
* [`clusterctl alpha rollout`](commands/alpha-rollout.md) Manages the rollout of Cluster API resources. For example: MachineDeployments.

For the full list of clusterctl commands please refer to [commands](commands/commands.md).

### Avoiding GitHub rate limiting

While using providers hosted on GitHub, clusterctl is calling GitHub API which are rate limited; for normal usage free tier is enough but when using clusterctl extensively users might hit the rate limit.

To avoid rate limiting for the public repos set the `GITHUB_TOKEN` environment variable. To generate a token [follow this](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-personal-access-token) documentation. The token only needs `repo` scope for clusterctl.

Per default `clusterctl` will use a go proxy to detect the available versions to prevent additional
API calls to the GitHub API. It is possible to configure the go proxy url using the `GOPROXY` variable as
for go itself (defaults to `https://proxy.golang.org`).
To immediately fallback to the GitHub client and not use a go proxy, the environment variable could get set to
`GOPROXY=off` or `GOPROXY=direct`.
If a provider does not follow Go's semantic versioning, `clusterctl` may fail when detecting the correct version.
In such cases, disabling the go proxy functionality via `GOPROXY=off` should be considered.

# Installing clusterctl
Instructions are available in the [Quick Start](../user/quick-start.md#install-clusterctl).

<!-- links -->
[management cluster]: ../reference/glossary.md#management-cluster
[provider components]: ../reference/glossary.md#provider-components
