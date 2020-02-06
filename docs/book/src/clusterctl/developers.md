# clusterctl for Developers

This document describes how to use `clusterctl` during the development workflow.

## Prerequisites

* A Cluster API development setup (go, git, etc.)
* A local clone of the Cluster API GitHub repository
* A local clone of the GitHub repositories for the providers you want to install

## Getting started

### Build clustertl

From the root of the local copy of Cluster API, you can build the `clusterctl` binary by running:

```shell
make clusterctl
```

The output of the build is saved in the `bin/` folder; In order to use it you have to specify
the full path, create an alias or copy it into a folder under your `$PATH`.

### Create a clusterctl-settings.json file

Next, create a `clusterctl-settings.json` file and place it in your local copy of Cluster API. Here is an example:

```yaml
{
  "providers": ["kubeadm-bootstrap","kubeadm-control-plane", "aws"],
  "provider_repos": ["../cluster-api-provider-aws"]
}
```

**providers** (Array[]String, default=[]): A list of the providers to enable.
See [available providers](#available-providers) for more details.

**provider_repos** (Array[]String, default=[]): A list of paths to all the providers you want to use. Each provider must have
a `clusterctl-settings.json` file describing how to build the provider assets.

## Run the local-overrides hack!

You can now run the local-overrides hack from the root of the local copy of Cluster API:

```shell
cmd/clusterctl/hack/local-overrides.py
```

The script reads from the local repositories of the providers you want to install, builds the providers' assets,
and places them in a local override folder located under `$HOME/.cluster-api/overrides/`.
Additionally, the command output provides you the `clusterctl init` command with all the necessary flags.

```shell
clusterctl local overrides generated from local repositories for the cluster-api, kubeadm-bootstrap, aws providers.
in order to use them, please run:

clusterctl init  --core cluster-api:v0.3.0 --bootstrap kubeadm-bootstrap:v0.3.0 --infrastructure aws:v0.5.0
```

## Available providers

The following providers are currently defined in the script:

* `cluster-api`
* `kubeadm-bootstrap`
* `kubeadm-control-plane`
* `docker`

More providers can be added by editing the `clusterctl-settings.json` in your local copy of Cluster API;
please note that each `provider_repo` should have its own `clusterctl-settings.json` describing how to build the provider assets, e.g.

```yaml
{
  "name": "aws",
  "config": {
    "componentsFile": "infrastructure-components.yaml",
    "nextVersion": "v0.5.0",
    "type": "InfrastructureProvider"
  }
}
```