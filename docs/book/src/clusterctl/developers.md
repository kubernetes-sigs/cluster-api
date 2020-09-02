# clusterctl for Developers

This document describes how to use `clusterctl` during the development workflow.

## Prerequisites

* A Cluster API development setup (go, git, kind v0.7 or newer, Docker v19.03 or newer etc.)
* A local clone of the Cluster API GitHub repository
* A local clone of the GitHub repositories for the providers you want to install

## Build clusterctl

From the root of the local copy of Cluster API, you can build the `clusterctl` binary by running:

```shell
make clusterctl
```

The output of the build is saved in the `bin/` folder; In order to use it you have to specify
the full path, create an alias or copy it into a folder under your `$PATH`.

## Use local artifacts

Clusterctl by default uses artifacts published in the [providers repositories];
during the development workflow it might happen you want to use artifacts from your local workstation.

There are two options to do so:

-  Use the [overrides layer], when you want to override a single published artifact with a local one.
-  Create a local repository, when you want to avoid to use published artifacts and use intead the local ones.

If you want to create a local artifact, follow those instructions:

### Build artifacts locally

In order to build artifacts for the CAPI core provider, the kubeadm bootstrap provider and the kubeadm control plane provider:

```
make docker-build REGISTRY=gcr.io/k8s-staging-cluster-api
make -C test/infrastructure/docker generate-manifests REGISTRY=gcr.io/k8s-staging-cluster-api
```

In order to build docker provider artifacts

```
make -C test/infrastructure/docker docker-build REGISTRY=gcr.io/k8s-staging-cluster-api
make -C test/infrastructure/docker generate-manifests REGISTRY=gcr.io/k8s-staging-cluster-api
```

### Create a clusterctl-settings.json file

Next, create a `clusterctl-settings.json` file and place it in your local copy of Cluster API. Here is an example:

```yaml
{
  "providers": ["cluster-api","bootstrap-kubeadm","control-plane-kubeadm", "infrastructure-aws", "infrastructure-docker"],
  "provider_repos": ["../cluster-api-provider-aws"]
}
```

**providers** (Array[]String, default=[]): A list of the providers to enable.
See [available providers](#available-providers) for more details.

**provider_repos** (Array[]String, default=[]): A list of paths to all the providers you want to use. Each provider must have
a `clusterctl-settings.json` file describing how to build the provider assets.

### Create the local repository

Run the create-local-repository hack from the root of the local copy of Cluster API:

```shell
cmd/clusterctl/hack/create-local-repository.py
```

The script reads from the source folders for the providers you want to install, builds the providers' assets,
and places them in a local repository folder located under `$HOME/.cluster-api/dev-repository/`.
Additionally, the command output provides you the `clusterctl init` command with all the necessary flags.
The output should be similar to:

```shell
clusterctl local overrides generated from local repositories for the cluster-api, bootstrap-kubeadm, control-plane-kubeadm, infrastructure-docker, infrastructure-aws providers.
in order to use them, please run:

clusterctl init \
   --core cluster-api:v0.3.8 \
   --bootstrap kubeadm:v0.3.8 \
   --control-plane kubeadm:v0.3.8 \
   --infrastructure aws:v0.5.0 \
   --infrastructure docker:v0.3.8 \
   --config ~/.cluster-api/dev-repository/config.yaml

```

As you might notice, the command is using the `$HOME/.cluster-api/dev-repository/config.yaml` config file,
containing all the required setting to make clusterctl use the local repository.

<aside class="note warning">

<h1>Warnings</h1>

You must pass `--config ~/.cluster-api/dev-repository/config.yaml` to all the clusterctl commands you are running
during your dev session. 

The above config file changes the location of the [overrides layer] folder thus ensuring
you dev session isn't hijacked by other local artifacts.

With the only exception of the docker provider, the local repository folder does not contain cluster templates, 
so `clusterctl config cluster` command will fail.

</aside>

#### Available providers

The following providers are currently defined in the script:

* `cluster-api`
* `bootstrap-kubeadm`
* `control-plane-kubeadm`
* `infrastructure-docker`

More providers can be added by editing the `clusterctl-settings.json` in your local copy of Cluster API;
please note that each `provider_repo` should have its own `clusterctl-settings.json` describing how to build the provider assets, e.g.

```yaml
{
  "name": "infrastructure-aws",
  "config": {
    "componentsFile": "infrastructure-components.yaml",
    "nextVersion": "v0.5.0",
  }
}
```

## Create a kind cluster

[kind] can provide a Kubernetes cluster to be used a management cluster.
See [Install and/or configure a kubernetes cluster] for more about how.

Before running clusterctl init, you must ensure all the required images are available in the kind cluster.

This is always the case for images published in some image repository like docker hub or gcr.io, but it can't be
the case for images built locally; in this case, you can use `kind load` to move the images built locally. e.g 
 
```
kind load docker-image gcr.io/k8s-staging-cluster-api/cluster-api-controller-amd64:dev
kind load docker-image gcr.io/k8s-staging-cluster-api/kubeadm-bootstrap-controller-amd64:dev
kind load docker-image gcr.io/k8s-staging-cluster-api/kubeadm-control-plane-controller-amd64:dev
kind load docker-image gcr.io/k8s-staging-cluster-api/capd-manager-amd64:dev
``` 

to make the controller images available for the kubelet in the management cluster.

## Additional Notes for the Docker Provider

The command for getting the kubeconfig file for connecting to a workload cluster is the following:

```bash
clusterctl get kubeconfig capi-quickstart > capi-quickstart.kubeconfig
```

When using docker on MacOS, you will need to do a couple of additional
steps to get the correct kubeconfig for a workload cluster created with the docker provider:

```bash
# Point the kubeconfig to the exposed port of the load balancer, rather than the inaccessible container IP.
sed -i -e "s/server:.*/server: https:\/\/$(docker port capi-quickstart-lb 6443/tcp | sed "s/0.0.0.0/127.0.0.1/")/g" ./capi-quickstart.kubeconfig

# Ignore the CA, because it is not signed for 127.0.0.1
sed -i -e "s/certificate-authority-data:.*/insecure-skip-tls-verify: true/g" ./capi-quickstart.kubeconfig
```

<!-- links -->
[kind]: https://kind.sigs.k8s.io/
[providers repositories]: configuration.md#provider-repositories
[overrides layer]: configuration.md#overrides-layer
[Install and/or configure a kubernetes cluster]: ../user/quick-start.md#install-andor-configure-a-kubernetes-cluster