# clusterctl Configuration File

The `clusterctl` config file is located at `$HOME/.cluster-api/clusterctl.yaml`.
It can be used to:

- Customize the list of providers and provider repositories.
- Provide configuration values to be used for variable substitution when installing providers or creating clusters.
- Define image overrides for air-gapped environments.

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
    url: "https://github.com/myorg/myrepo/releases/latest/infrastructure-components.yaml"
    type: "InfrastructureProvider"
  # override a pre-defined provider
  - name: "cluster-api"
    url: "https://github.com/myorg/myforkofclusterapi/releases/latest/core-components.yaml"
    type: "CoreProvider"
```

See [provider contract](provider-contract.md) for instructions about how to set up a provider repository.

## Variables

When installing a provider `clusterctl` reads a YAML file that is published in the provider repository. While executing
this operation, `clusterctl` can substitute certain variables with the ones provided by the user.

The same mechanism also applies when `clusterctl` reads the cluster templates YAML published in the repository, e.g.
when injecting the Kubernetes version to use, or the number of worker machines to create.

The user can provide values using OS environment variables, but it is also possible to add
variables in the `clusterctl` config file:

```yaml
# Values for environment variable substitution
AWS_B64ENCODED_CREDENTIALS: XXXXXXXX
```

In case a variable is defined both in the config file and as an OS environment variable,
the environment variable takes precedence.

## Cert-Manager configuration

While doing init, clusterctl checks if there is a version of cert-manager already installed. If not, clusterctl will
install a default version.

By default, cert-manager will be fetched from `https://github.com/jetstack/cert-manager/releases`; however, if the user
wants to use a different repository, it is possible to use the following configuration:

```yaml
cert-manager:
  url: "/Users/foo/.cluster-api/dev-repository/cert-manager/latest/cert-manager.yaml"
```

Similarly, it is possible to override the default version installed by clusterctl by configuring:

```yaml
cert-manager:
  ...
  version: "v1.1.1"
```

For situations when resources are limited or the network is slow, the cert-manager wait time to be running can be customized by adding a field to the clusterctl config file, for example:

```yaml

```yaml
cert-manager:
  ...
  timeout: 15m
```

The value string is a possibly signed sequence of decimal numbers, each with optional fraction and a unit suffix, such as "300ms", "-1.5h" or "2h45m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".

If no value is specified, or the format is invalid, the default value of 10 minutes will be used.

Please note that the configuration above will be considered also when doing `clusterctl upgrade plan` or `clusterctl upgrade plan`.

## Overrides Layer

`clusterctl` uses an overrides layer to read in injected provider components,
cluster templates and metadata. By default, it reads the files from
`$HOME/.cluster-api/overrides`.

The directory structure under the `overrides` directory should follow the
template:

```
<providerType-providerName>/<version>/<fileName>
```

For example,

```
├── bootstrap-kubeadm
│   └── v0.3.0
│       └── bootstrap-components.yaml
├── cluster-api
│   └── v0.3.0
│       └── core-components.yaml
├── control-plane-kubeadm
│   └── v0.3.0
│       └── control-plane-components.yaml
└── infrastructure-aws
    └── v0.5.0
            ├── cluster-template-dev.yaml
            └── infrastructure-components.yaml
```

For developers who want to generate the overrides layer, see
[Build artifacts locally](developers.md#build-artifacts-locally).

Once these overrides are specified, `clusterctl` will use them instead of
getting the values from the default or specified providers.

One example usage of the overrides layer is that it allows you to deploy
clusters with custom templates that may not be available from the official
provider repositories.
For example, you can now do:

```bash
clusterctl generate cluster mycluster --flavor dev --infrastructure aws:v0.5.0 -v5
```

The `-v5` provides verbose logging which will confirm the usage of the
override file.

```bash
Using Override="cluster-template-dev.yaml" Provider="infrastructure-aws" Version="v0.5.0"
```

Another example, if you would like to deploy a custom version of CAPA, you can
make changes to `infrastructure-components.yaml` in the overrides folder and
run,

```bash
clusterctl init --infrastructure aws:v0.5.0 -v5
...
Using Override="infrastructure-components.yaml" Provider="infrastructure-aws" Version="v0.5.0"
...
```

If you prefer to have the overrides directory at a different location (e.g.
`/Users/foobar/workspace/dev-releases`) you can specify the overrides
directory in the clusterctl config file as

```yaml
overridesFolder: /Users/foobar/workspace/dev-releases
```

## Image overrides

<aside class="note warning">

<h1> Warning! </h1>

Image override is an advanced feature and wrong configuration can easily lead to non-functional clusters.
It's strongly recommended to test configurations on dev/test environments before using this functionality in production.

</aside>

When working in air-gapped environments, it's necessary to alter the manifests to be installed in order to pull
images from a local/custom image repository instead of public ones (e.g. `gcr.io`, or `quay.io`).

The `clusterctl` configuration file can be used to instruct `clusterctl` to override images automatically.

This can be achieved by adding an `images` configuration entry as shown in the example:

```yaml
images:
  all:
    repository: myorg.io/local-repo
```

Please note that the image override feature allows for more fine-grained configuration, allowing to set image
overrides for specific components, for example:

```yaml
images:
  all:
    repository: myorg.io/local-repo
  cert-manager:
    tag: v1.5.0
```

In this example we are overriding the image repository for all the components and the image tag for
all the images in the cert-manager component.

If required to alter only a specific image you can use:

```yaml
images:
  all:
    repository: myorg.io/local-repo
  cert-manager/cert-manager-cainjector:
    tag: v1.5.0
```

## Debugging/Logging

To have more verbose logs you can use the `-v` flag when running the `clusterctl` and set the level of the logging verbose with a positive integer number, ie. `-v 3`.

If you do not want to use the flag every time you issue a command you can set the environment variable `CLUSTERCTL_LOG_LEVEL` or set the variable in the `clusterctl` config file located by default at `$HOME/.cluster-api/clusterctl.yaml`.
