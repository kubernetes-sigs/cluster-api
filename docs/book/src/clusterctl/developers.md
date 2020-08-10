# clusterctl for Developers

This document describes how to use `clusterctl` during the development workflow.

## Prerequisites

* A Cluster API development setup (go, git, kind v0.7 or newer, Docker v19.03 or newer etc.)
* A local clone of the Cluster API GitHub repository
* A local clone of the GitHub repositories for the providers you want to install

## Getting started

### Build clusterctl

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
  "providers": ["cluster-api","bootstrap-kubeadm","control-plane-kubeadm", "infrastructure-aws"],
  "provider_repos": ["../cluster-api-provider-aws"]
}
```

**providers** (Array[]String, default=[]): A list of the providers to enable.
See [available providers](#available-providers) for more details.

**provider_repos** (Array[]String, default=[]): A list of paths to all the providers you want to use. Each provider must have
a `clusterctl-settings.json` file describing how to build the provider assets.

### Run the local-overrides hack!

<aside class="note">

<h1>If using the docker provider, take additional steps</h1>

Before running the local-overrides hack with the Docker provider, follow <a href="#additional-steps-for-the-docker-provider">these steps</a>.

</aside>

You can now run the local-overrides hack from the root of the local copy of Cluster API:

```shell
cmd/clusterctl/hack/local-overrides.py
```

The script reads from the local repositories of the providers you want to install, builds the providers' assets,
and places them in a local override folder located under `$HOME/.cluster-api/overrides/`.
Additionally, the command output provides you the `clusterctl init` command with all the necessary flags.

```shell
clusterctl local overrides generated from local repositories for the cluster-api, bootstrap-kubeadm, control-plane-kubeadm, infrastructure-aws providers.
in order to use them, please run:

clusterctl init  --core cluster-api:v0.3.0 --bootstrap kubeadm:v0.3.0 --infrastructure aws:v0.5.0
```

See [Overrides Layer](configuration.md#overrides-layer) for more information
on the purpose of overrides.

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

#### Additional Steps for the Docker Provider

<aside class="note warning">

<h1>Warning</h1>

The Docker provider is not designed for production use and is intended for development environments only.

</aside>

Before running the local-overrides hack:

- Run `make -C test/infrastructure/docker docker-build REGISTRY=gcr.io/k8s-staging-cluster-api` to build the docker provider image
  using a specific REGISTRY (you can choose your own).

- Run `make -C test/infrastructure/docker generate-manifests REGISTRY=gcr.io/k8s-staging-cluster-api` to generate
  the docker provider manifest using the above registry/Image.

Run the local-overrides hack and save the `clusterctl init` command provided in the command output to be used later.

Edit the clusterctl config file located at `~/.cluster-api/clusterctl.yaml` and configure the docker provider
by adding the following lines (replace $HOME with your home path):

```bash
providers:
  - name: docker
    url: $HOME/.cluster-api/overrides/infrastructure-docker/latest/infrastructure-components.yaml
    type: InfrastructureProvider
```

If you are using Kind for creating the management cluster, you should:

{{#tabs name:"install-kind" tabs:"v0.7.x,v0.8.x"}}
{{#tab v0.7.x}}

- Run the following command to create a kind config file for allowing the Docker provider to access Docker on the host:

```bash
cat > kind-cluster-with-extramounts.yaml <<EOF
kind: Cluster
apiVersion: kind.sigs.k8s.io/v1alpha3
nodes:
  - role: control-plane
    extraMounts:
      - hostPath: /var/run/docker.sock
        containerPath: /var/run/docker.sock
EOF
```
- Run `kind create cluster --config kind-cluster-with-extramounts.yaml` to create the management cluster using the above file

- Run `kind load docker-image gcr.io/k8s-staging-cluster-api/capd-manager-amd64:dev` to make the docker provider image available for the kubelet in the management cluster.

- Run `clusterctl init` command provided as output of the local-overrides hack.

{{#/tab }}
{{#tab v0.8.x}}

- Export the **KIND_EXPERIMENTAL_DOCKER_NETWORK=bridge** so kind and CAPD can create containers in the same network, since **kind v0.8.0**, kind by default creates the containers in the **kind** network while CAPD is creating them in the **bridge** one.
```bash
export KIND_EXPERIMENTAL_DOCKER_NETWORK=bridge
```
- Run the following command to create a kind config file for allowing the Docker provider to access Docker on the host:
```bash
cat > kind-cluster-with-extramounts.yaml <<EOF
kind: Cluster
apiVersion: kind.sigs.k8s.io/v1alpha3
nodes:
  - role: control-plane
    extraMounts:
      - hostPath: /var/run/docker.sock
        containerPath: /var/run/docker.sock
EOF
```
- Run `kind create cluster --config kind-cluster-with-extramounts.yaml` to create the management cluster using the above file

- Run `kind load docker-image gcr.io/k8s-staging-cluster-api/capd-manager-amd64:dev` to make the docker provider image available for the kubelet in the management cluster.

- Run `clusterctl init` command provided as output of the local-overrides hack.

{{#/tab }}
{{#/tabs }}

### Connecting to a workload cluster on docker

The command for getting the kubeconfig file for connecting to a workload cluster is the following:

```bash
clusterctl get kubeconfig capi-quickstart
```

When using docker-for-mac MacOS, you will need to do a couple of additional
steps to get the correct kubeconfig for a workload cluster created with the docker provider:

```bash
# Point the kubeconfig to the exposed port of the load balancer, rather than the inaccessible container IP.
sed -i -e "s/server:.*/server: https:\/\/$(docker port capi-quickstart-lb 6443/tcp | sed "s/0.0.0.0/127.0.0.1/")/g" ./capi-quickstart.kubeconfig

# Ignore the CA, because it is not signed for 127.0.0.1
sed -i -e "s/certificate-authority-data:.*/insecure-skip-tls-verify: true/g" ./capi-quickstart.kubeconfig
```

<!-- links -->
[kind]: https://kind.sigs.k8s.io/
