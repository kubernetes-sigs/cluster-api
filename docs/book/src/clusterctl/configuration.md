# clusterctl Configuration File

The `clusterctl` config file is located at `$HOME/.cluster-api/clusterctl.yaml` and it can be used to:

- Customize the list of providers and provider repositories.
- Provide configuration values to be used for variable substitution when installing providers or creating clusters.

## Provider repositories

The `clusterctl` CLI is designed to work with providers implementing the [clusterctl Provider Contract](provider-contract.md).

Each provider is expected to define a provider repository, a well-known place where release assets are published. 

By default, `clusterctl` ships with providers sponsored by SIG Cluster
Lifecycle. Use `clusterctl config repositories` to get a list of supported
providers and their repository configuration.

Users can customize the list of available providers using the `clusterctl` configuration file, as shown in the following example:

```yaml
providers:
  # add a custom provider
  - name: "my-infra-provider"
    url: "https://github.com/myorg/myrepo/releases/latest/infrastructure_components.yaml"
    type: "InfrastructureProvider"
  # override a pre-defined provider
  - name: "cluster-api"
    url: "https://github.com/myorg/myforkofclusterapi/releases/latest/core_components.yaml"
    type: "CoreProvider"
```

See [provider contract](provider-contract.md) for instructions about how to set up a provider repository.

## Variables

When installing a provider `clusterctl` reads a YAML file that is published in the provider repository; while executing
this operation, `clusterctl` can substitute certain variables with the ones provided by the user.   

The same mechanism also applies when `clusterctl` reads the cluster templates YAML published in the repository, e.g. 
when injecting the Kubernetes version to use, or the number of worker machines to create.

The user can provide values using OS environment variables, but it is also possible to add
variables in the `clusterctl` config file:

```yaml
# Values for environment variable substitution
AWS_B64ENCODED_CREDENTIALS: XXXXXXXX
```

In case a variable is defined both in the config file and as an OS environment variable, the latter takes precedence.
