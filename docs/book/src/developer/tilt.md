# Developing Cluster API with Tilt

## Overview

This document describes how to use [kind](https://kind.sigs.k8s.io) and [Tilt](https://tilt.dev) for a simplified
workflow that offers easy deployments and rapid iterative builds.

## Prerequisites

1. [Docker](https://docs.docker.com/install/) v19.03 or newer
1. [kind](https://kind.sigs.k8s.io) v0.9 or newer (other clusters can be
   used if `preload_images_for_kind` is set to false)
1. [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/)
   standalone (`kubectl kustomize` does not work because it is missing
   some features of kustomize v3)
1. [Tilt](https://docs.tilt.dev/install.html) v0.16.0 or newer
1. [envsubst](https://github.com/drone/envsubst) or similar to handle
   clusterctl var replacement. Note: drone/envsubst releases v1.0.2 and
   earlier do not have the binary packaged under cmd/envsubst. It is
   available in Go pseudo-version `v1.0.3-0.20200709231038-aa43e1c1a629`
1. Clone the [Cluster
   API](https://github.com/kubernetes-sigs/cluster-api) repository
   locally
1. Clone the provider(s) you want to deploy locally as well

We provide a make target to generate the envsubst binary if desired.
See the [provider contract](./../clusterctl/provider-contract.md) for
more details about how clusterctl uses variables.

```
make envsubst
```

## Getting started

### Create a kind cluster
A script to create a KIND cluster along with a local docker registry and the correct mounts to run CAPD is included in the hack/ folder.

To create a pre-configured cluster run:

```bash 
./hack/kind-install-for-capd.sh
````

You can see the status of the cluster with:

```bash
kubectl cluster-info --context kind-capi-test
```

### Create a tilt-settings.json file

Next, create a `tilt-settings.json` file and place it in your local copy of `cluster-api`. Here is an example:

```json
{
  "default_registry": "gcr.io/your-project-name-here",
  "provider_repos": ["../cluster-api-provider-aws"],
  "enable_providers": ["aws", "docker", "kubeadm-bootstrap", "kubeadm-control-plane"]
}
```

#### tilt-settings.json fields

**allowed_contexts** (Array, default=[]): A list of kubeconfig contexts Tilt is allowed to use. See the Tilt documentation on
[allow_k8s_contexts](https://docs.tilt.dev/api.html#api.allow_k8s_contexts) for more details.

**default_registry** (String, default=""): The image registry to use if you need to push images. See the [Tilt
documentation](https://docs.tilt.dev/api.html#api.default_registry) for more details.

**provider_repos** (Array[]String, default=[]): A list of paths to all the providers you want to use. Each provider must have a
`tilt-provider.json` file describing how to build the provider.

**enable_providers** (Array[]String, default=['docker']): A list of the providers to enable. See [available providers](#available-providers)
for more details.

**kind_cluster_name** (String, default="kind"): The name of the kind cluster to use when preloading images.

**kustomize_substitutions** (Map{String: String}, default={}): An optional map of substitutions for `${}`-style placeholders in the
provider's yaml.

{{#tabs name:"tab-tilt-kustomize-substitution" tabs:"AWS,Azure,DigitalOcean,GCP"}}
{{#tab AWS}}

For example, if the yaml contains `${AWS_B64ENCODED_CREDENTIALS}`, you could do the following:

```json
"kustomize_substitutions": {
  "AWS_B64ENCODED_CREDENTIALS": "your credentials here"
}
```

{{#/tab }}
{{#tab AZURE}}

An Azure Service Principal is needed for populating the controller manifests. This utilizes [environment-based authentication](https://docs.microsoft.com/en-us/go/azure/azure-sdk-go-authorization#use-environment-based-authentication).

  1. Save your Subscription ID

  ```bash
  AZURE_SUBSCRIPTION_ID=$(az account show --query id --output tsv)
  az account set --subscription $AZURE_SUBSCRIPTION_ID
  ```

  2. Set the Service Principal name

  ```bash
  AZURE_SERVICE_PRINCIPAL_NAME=ServicePrincipalName
  ```

  3. Save your Tenant ID, Client ID, Client Secret

  ```bash
  AZURE_TENANT_ID=$(az account show --query tenantId --output tsv)
  AZURE_CLIENT_SECRET=$(az ad sp create-for-rbac --name http://$AZURE_SERVICE_PRINCIPAL_NAME --query password --output tsv)
  AZURE_CLIENT_ID=$(az ad sp show --id http://$AZURE_SERVICE_PRINCIPAL_NAME --query appId --output tsv)
  ```

Add the output of the following as a section in your `tilt-settings.json`:

  ```shell
  cat <<EOF
  "kustomize_substitutions": {
     "AZURE_SUBSCRIPTION_ID_B64": "$(echo "${AZURE_SUBSCRIPTION_ID}" | tr -d '\n' | base64 | tr -d '\n')",
     "AZURE_TENANT_ID_B64": "$(echo "${AZURE_TENANT_ID}" | tr -d '\n' | base64 | tr -d '\n')",
     "AZURE_CLIENT_SECRET_B64": "$(echo "${AZURE_CLIENT_SECRET}" | tr -d '\n' | base64 | tr -d '\n')",
     "AZURE_CLIENT_ID_B64": "$(echo "${AZURE_CLIENT_ID}" | tr -d '\n' | base64 | tr -d '\n')"
    }
  EOF
```

{{#/tab }}
{{#tab DigitalOcean}}

```json
"kustomize_substitutions": {
  "DO_B64ENCODED_CREDENTIALS": "your credentials here"
}
```

{{#/tab }}
{{#tab GCP}}

You can generate a base64 version of your GCP json credentials file using:
```bash
base64 -i ~/path/to/gcp/credentials.json
```

```json
"kustomize_substitutions": {
  "GCP_B64ENCODED_CREDENTIALS": "your credentials here"
}
```

{{#/tab }}
{{#/tabs }}

**deploy_cert_manager** (Boolean, default=`true`): Deploys cert-manager into the cluster for use for webhook registration.

**preload_images_for_kind** (Boolean, default=`true`): Uses `kind load docker-image` to preload images into a kind cluster.

**trigger_mode** (String, default=`auto`): Optional setting to configure if tilt should automatically rebuild on changes.
Set to `manual` to disable auto-rebuilding and require users to trigger rebuilds of individual changed components through the UI.

**extra_args** (Object, default={}): A mapping of provider to additional arguments to pass to the main binary configured
for this provider. Each item in the array will be passed in to the manager for the given provider.

Example:

```json
{
    "extra_args": {
        "core": ["--feature-gates=MachinePool=true"],
        "kubeadm-bootstrap": ["--feature-gates=MachinePool=true"],
        "azure": ["--feature-gates=MachinePool=true"]
    }
}
```

With this config, the respective managers will be invoked with:

```bash
manager --feature-gates=MachinePool=true
```

### Run Tilt!

To launch your development environment, run

``` bash
tilt up
```

This will open the command-line HUD as well as a web browser interface. You can monitor Tilt's status in either
location. After a brief amount of time, you should have a running development environment, and you should now be able to
create a cluster. There are [example worker cluster
configs](https://github.com/kubernetes-sigs/cluster-api/tree/main/test/infrastructure/docker/examples) available.
These can be customized for your specific needs.

<aside class="note">

<h1>Use of clusterctl</h1>

When the worker cluster has been created using tilt, `clusterctl` should not be used for management
operations; this is because tilt doesn't initialize providers on the management cluster like clusterctl init does, so
some of the clusterctl commands like clusterctl config won't work.

This limitation is an acceptable trade-off while executing fast dev-test iterations on controllers logic. If instead
you are interested in testing clusterctl workflows, you should refer to the
[clusterctl developer instructions](https://cluster-api.sigs.k8s.io/clusterctl/developers.html).

</aside>

## Available providers

The following providers are currently defined in the Tiltfile:

* **core**: cluster-api itself (Cluster/Machine/MachineDeployment/MachineSet/KubeadmConfig/KubeadmControlPlane)
* **docker**: Docker provider (DockerCluster/DockerMachine)

### tilt-provider.json

A provider must supply a `tilt-provider.json` file describing how to build it. Here is an example:

```json
{
    "name": "aws",
    "config": {
        "image": "gcr.io/k8s-staging-cluster-api-aws/cluster-api-aws-controller",
        "live_reload_deps": [
            "main.go", "go.mod", "go.sum", "api", "cmd", "controllers", "pkg"
        ]
    }
}
```

#### config fields

**image**: the image for this provider, as referenced in the kustomize files. This must match; otherwise, Tilt won't
build it.

**live_reload_deps**: a list of files/directories to watch. If any of them changes, Tilt rebuilds the manager binary
for the provider and performs a live update of the running container.

**additional_docker_helper_commands** (String, default=""): Additional commands to be run in the helper image
docker build. e.g.

``` Dockerfile
RUN wget -qO- https://dl.k8s.io/v1.21.2/kubernetes-client-linux-amd64.tar.gz | tar xvz
RUN wget -qO- https://get.docker.com | sh
```

**additional_docker_build_commands** (String, default=""): Additional commands to be appended to
the dockerfile.
The manager image will use docker-slim, so to download files, use `additional_helper_image_commands`. e.g.

``` Dockerfile
COPY --from=tilt-helper /usr/bin/docker /usr/bin/docker
COPY --from=tilt-helper /go/kubernetes/client/bin/kubectl /usr/bin/kubectl
```

**kustomize_config** (Bool, default=true): Whether or not running kustomize on the ./config folder of the provider.
Set to `false` if your provider does not have a ./config folder or you do not want it to be applied in the cluster.

**go_main** (String, default="main.go"): The go main file if not located at the root of the folder

**label** (String, default=provider name): The label to be used to group provider components in the tilt UI 
in tilt version >= v0.22.2 (see https://blog.tilt.dev/2021/08/09/resource-grouping.html); as a convention,
provider abbreviation should be used (CAPD, KCP etc.).

**manager_name** (String): If provided, it will allow tilt to move the provider controller under the above label.

## Customizing Tilt

If you need to customize Tilt's behavior, you can create files in cluster-api's `tilt.d` directory. This file is ignored
by git so you can be assured that any files you place here will never be checked in to source control.

These files are included after the `providers` map has been defined and after all the helper function definitions. This
is immediately before the "real work" happens.

## Under the covers, a.k.a "the real work"

At a high level, the Tiltfile performs the following actions:

1. Read `tilt-settings.json`
1. Configure the allowed Kubernetes contexts
1. Set the default registry
1. Define the `providers` map
1. Include user-defined Tilt files
1. Deploy cert-manager
1. Enable providers (`core` + what is listed in `tilt-settings.json`)
    1. Build the manager binary locally as a `local_resource`
    1. Invoke `docker_build` for the provider
    1. Invoke `kustomize` for the provider's `config/` directory

### Live updates

Each provider in the `providers` map has a `live_reload_deps` list. This defines the files and/or directories that Tilt
should monitor for changes. When a dependency is modified, Tilt rebuilds the provider's manager binary **on your local
machine**, copies the binary to the running container, and executes a restart script. This is significantly faster
than rebuilding the container image for each change. It also helps keep the size of each development image as small as
possible (the container images do not need the entire go toolchain, source code, module dependencies, etc.).
