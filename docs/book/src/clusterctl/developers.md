# clusterctl for Developers

This document describes how to use `clusterctl` during the development workflow.

## Prerequisites

* A Cluster API development setup (go, git, kind v0.9 or newer, Docker v19.03 or newer etc.)
* A local clone of the Cluster API GitHub repository
* A local clone of the GitHub repositories for the providers you want to install

## Build clusterctl

From the root of the local copy of Cluster API, you can build the `clusterctl` binary by running:

```bash
make clusterctl
```

The output of the build is saved in the `bin/` folder; In order to use it you have to specify
the full path, create an alias or copy it into a folder under your `$PATH`.

## Use local artifacts

Clusterctl by default uses artifacts published in the [providers repositories];
during the development workflow you may want to use artifacts from your local workstation.

There are two options to do so:

* Use the [overrides layer], when you want to override a single published artifact with a local one.
* Create a local repository, when you want to avoid using published artifacts and use the local ones instead.

If you want to create a local artifact, follow these instructions:

### Build artifacts locally

In order to build artifacts for the CAPI core provider, the kubeadm bootstrap provider, the kubeadm control plane provider and the Docker infrastructure provider:

```bash
make docker-build REGISTRY=gcr.io/k8s-staging-cluster-api PULL_POLICY=IfNotPresent
```

### Create a clusterctl-settings.json file

Next, create a `clusterctl-settings.json` file and place it in your local copy
of Cluster API. This file will be used by [create-local-repository.py](#create-the-local-repository). Here is an example:

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

```bash
cmd/clusterctl/hack/create-local-repository.py
```

The script reads from the source folders for the providers you want to install, builds the providers' assets,
and places them in a local repository folder located under `$XDG_CONFIG_HOME/cluster-api/dev-repository/`.
Additionally, the command output provides you the `clusterctl init` command with all the necessary flags.
The output should be similar to:

```bash
clusterctl local overrides generated from local repositories for the cluster-api, bootstrap-kubeadm, control-plane-kubeadm, infrastructure-docker, infrastructure-aws providers.
in order to use them, please run:

clusterctl init \
   --core cluster-api:v0.3.8 \
   --bootstrap kubeadm:v0.3.8 \
   --control-plane kubeadm:v0.3.8 \
   --infrastructure aws:v0.5.0 \
   --infrastructure docker:v0.3.8 \
   --config $XDG_CONFIG_HOME/cluster-api/dev-repository/config.yaml
```

As you might notice, the command is using the `$XDG_CONFIG_HOME/cluster-api/dev-repository/config.yaml` config file,
containing all the required setting to make clusterctl use the local repository (it fallbacks to `$HOME` if `$XDG_CONFIG_HOME` 
is not set on your machine).

<aside class="note warning">

<h1>Warnings</h1>

You must pass `--config ...` to all the clusterctl commands you are running during your dev session.

The above config file changes the location of the [overrides layer] folder thus ensuring
you dev session isn't hijacked by other local artifacts.

With the exceptions of the Docker and the in memory provider, the local repository folder does not contain cluster templates,
so the `clusterctl generate cluster` command will fail if you don't copy a template into the local repository.

</aside>

<aside class="note warning">

<h1>Nightly builds</h1>

if you want to run your tests using a Cluster API nightly build, you can run the hack passing the nightly build folder
(change the date at the end of the bucket name according to your needs):

```bash
cmd/clusterctl/hack/create-local-repository.py https://storage.googleapis.com/k8s-staging-cluster-api/components/nightly_main_20240425
```

Note: this works only with core Cluster API nightly builds. 

</aside>

#### Available providers

The following providers are currently defined in the script:

* `cluster-api`
* `bootstrap-kubeadm`
* `control-plane-kubeadm`
* `infrastructure-docker`

More providers can be added by editing the `clusterctl-settings.json` in your local copy of Cluster API;
please note that each `provider_repo` should have its own `clusterctl-settings.json` describing how to build the provider assets, e.g.

```json
{
  "name": "infrastructure-aws",
  "config": {
    "componentsFile": "infrastructure-components.yaml",
    "nextVersion": "v0.5.0"
  }
}
```

## Create a kind management cluster

[kind] can provide a Kubernetes cluster to be used as a management cluster.
See [Install and/or configure a Kubernetes cluster] for more information.

*Before* running clusterctl init, you must ensure all the required images are available in the kind cluster.

This is always the case for images published in some image repository like Docker Hub or gcr.io, but it can't be
the case for images built locally; in this case, you can use `kind load` to move the images built locally. e.g.

```bash
kind load docker-image gcr.io/k8s-staging-cluster-api/cluster-api-controller-amd64:dev
kind load docker-image gcr.io/k8s-staging-cluster-api/kubeadm-bootstrap-controller-amd64:dev
kind load docker-image gcr.io/k8s-staging-cluster-api/kubeadm-control-plane-controller-amd64:dev
kind load docker-image gcr.io/k8s-staging-cluster-api/capd-manager-amd64:dev
```

to make the controller images available for the kubelet in the management cluster.

When the kind cluster is ready and all the required images are in place, run
the clusterctl init command generated by the create-local-repository.py
script.

Optionally, you may want to check if the components are running properly. The
exact components are dependent on which providers you have initialized. Below
is an example output with the Docker provider being installed.

```bash
kubectl get deploy -A | grep "cap\|cert"
```
```bash
capd-system                         capd-controller-manager                         1/1     1            1           25m
capi-kubeadm-bootstrap-system       capi-kubeadm-bootstrap-controller-manager       1/1     1            1           25m
capi-kubeadm-control-plane-system   capi-kubeadm-control-plane-controller-manager   1/1     1            1           25m
capi-system                         capi-controller-manager                         1/1     1            1           25m
cert-manager                        cert-manager                                    1/1     1            1           27m
cert-manager                        cert-manager-cainjector                         1/1     1            1           27m
cert-manager                        cert-manager-webhook                            1/1     1            1           27m
```

## Additional Notes for the Docker Provider

### Select the appropriate Kubernetes version

When selecting the `--kubernetes-version`, ensure that the `kindest/node`
image is available.

For example, assuming that on [docker hub][kind-docker-hub] there is no
image for version `vX.Y.Z`, therefore creating a CAPD workload cluster with
`--kubernetes-version=vX.Y.Z` will fail. See [issue 3795] for more details.

### Get the kubeconfig for the workload cluster when using Docker Desktop

For Docker Desktop on macOS, Linux or Windows use kind to retrieve the kubeconfig.

```bash
kind get kubeconfig --name capi-quickstart > capi-quickstart.kubeconfig
````

Docker Engine for Linux works with the default clusterctl approach.
```bash
clusterctl get kubeconfig capi-quickstart > capi-quickstart.kubeconfig
```

### Fix kubeconfig when using Docker Desktop and clusterctl
When retrieving the kubeconfig using `clusterctl` with Docker Desktop on macOS or Windows or Docker Desktop (Docker Engine works fine) on Linux, you'll need to take a few extra steps to get the kubeconfig for a workload cluster created with the Docker provider.

```bash
clusterctl get kubeconfig capi-quickstart > capi-quickstart.kubeconfig
```

To fix the kubeconfig run:
```bash
# Point the kubeconfig to the exposed port of the load balancer, rather than the inaccessible container IP.
sed -i -e "s/server:.*/server: https:\/\/$(docker port capi-quickstart-lb 6443/tcp | sed "s/0.0.0.0/127.0.0.1/")/g" ./capi-quickstart.kubeconfig
```

<!-- links -->
[kind]: https://kind.sigs.k8s.io/
[providers repositories]: configuration.md#provider-repositories
[overrides layer]: configuration.md#overrides-layer
[Install and/or configure a Kubernetes cluster]: ../user/quick-start.md#install-andor-configure-a-kubernetes-cluster
[kind-docker-hub]: https://hub.docker.com/r/kindest/node/tags
[issue 3795]: https://github.com/kubernetes-sigs/cluster-api/issues/3795
