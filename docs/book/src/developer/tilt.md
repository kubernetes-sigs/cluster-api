# Developing Cluster API with Tilt

## Overview

This document describes how to use [kind](https://kind.sigs.k8s.io) and [Tilt](https://tilt.dev) for a simplified
workflow that offers easy deployments and rapid iterative builds.

## Prerequisites

1. [Docker](https://docs.docker.com/install/): v19.03 or newer
2. [kind](https://kind.sigs.k8s.io): v0.20.0 or newer
3. [Tilt](https://docs.tilt.dev/install.html): v0.30.8 or newer
4. [kustomize](https://github.com/kubernetes-sigs/kustomize): provided via `make kustomize`
5. [envsubst](https://github.com/drone/envsubst): provided via `make envsubst`
6. [helm](https://github.com/helm/helm): v3.7.1 or newer
7. Clone the [Cluster API](https://github.com/kubernetes-sigs/cluster-api) repository
   locally
8. Clone the provider(s) you want to deploy locally as well

## Getting started

### Create a kind cluster
A script to create a KIND cluster along with a local Docker registry and the correct mounts to run CAPD is included in the hack/ folder.

To create a pre-configured cluster run:

```bash
./hack/kind-install-for-capd.sh
```

You can see the status of the cluster with:

```bash
kubectl cluster-info --context kind-capi-test
```

### Create a tilt-settings file

Next, create a `tilt-settings.yaml` file and place it in your local copy of `cluster-api`. Here is an example that uses the components from the CAPI repo:

```yaml
default_registry: gcr.io/your-project-name-here
enable_providers:
- docker
- kubeadm-bootstrap
- kubeadm-control-plane
```

To use tilt to launch a provider with its own repo, using Cluster API Provider AWS here, `tilt-settings.yaml` should look like: 

```yaml
default_registry: gcr.io/your-project-name-here
provider_repos:
- ../cluster-api-provider-aws
enable_providers:
- aws
- kubeadm-bootstrap
- kubeadm-control-plane
```

<aside class="note">

If you prefer JSON, you can create a `tilt-settings.json` file instead. YAML will be preferred if both files are present.

</aside>

#### tilt-settings fields

**allowed_contexts** (Array, default=[]): A list of kubeconfig contexts Tilt is allowed to use. See the Tilt documentation on
[allow_k8s_contexts](https://docs.tilt.dev/api.html#api.allow_k8s_contexts) for more details.

**default_registry** (String, default=[]): The image registry to use if you need to push images. See the [Tilt
documentation](https://docs.tilt.dev/api.html#api.default_registry) for more details.
Please note that, in case you are not using a local registry, this value is required; additionally, the Cluster API
Tiltfile protects you from accidental push on `gcr.io/k8s-staging-cluster-api`.

**build_engine** (String, default="docker"): The engine used to build images. Can either be `docker` or `podman`.
NB: the default is dynamic and will be "podman" if the string "Podman Engine" is found in `docker version` (or in `podman version` if the command fails).

**kind_cluster_name** (String, default="capi-test"): The name of the kind cluster to use when preloading images.

**provider_repos** (Array[]String, default=[]): A list of paths to all the providers you want to use. Each provider must have a
`tilt-provider.yaml` or `tilt-provider.json` file describing how to build the provider.

**enable_providers** (Array[]String, default=['docker']): A list of the providers to enable. See [available providers](#available-providers)
for more details.

**template_dirs** (Map{String: Array[]String}, default={"docker": [
"./test/infrastructure/docker/templates"]}): A map of providers to directories containing cluster templates. An example of the field is given below. See [Deploying a workload cluster](#deploying-a-workload-cluster) for how this is used.

```yaml
template_dirs:
  docker:
  - ./test/infrastructure/docker/templates
  - <other-template-dir>
  azure:
  - <azure-template-dir>
  aws:
  - <aws-template-dir>
  gcp:
  - <gcp-template-dir>
```

**kustomize_substitutions** (Map{String: String}, default={}): An optional map of substitutions for `${}`-style placeholders in the
provider's yaml. These substitutions are also used when deploying cluster templates. See [Deploying a workload cluster](#deploying-a-workload-cluster).

**Note**: When running E2E tests locally using an existing cluster managed by Tilt, the following substitutions are required for successful tests:
```yaml
kustomize_substitutions:
  CLUSTER_TOPOLOGY: "true"
  EXP_MACHINE_POOL: "true"
  EXP_CLUSTER_RESOURCE_SET: "true"
  EXP_KUBEADM_BOOTSTRAP_FORMAT_IGNITION: "true"
  EXP_RUNTIME_SDK: "true"
  EXP_MACHINE_SET_PREFLIGHT_CHECKS: "true"
```

{{#tabs name:"tab-tilt-kustomize-substitution" tabs:"AWS,Azure,DigitalOcean,GCP,vSphere"}}
{{#tab AWS}}

For example, if the yaml contains `${AWS_B64ENCODED_CREDENTIALS}`, you could do the following:

```yaml
kustomize_substitutions:
  AWS_B64ENCODED_CREDENTIALS: "your credentials here"
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

Add the output of the following as a section in your `tilt-settings.yaml`:

```bash
  cat <<EOF
  kustomize_substitutions:
     AZURE_SUBSCRIPTION_ID_B64: "$(echo "${AZURE_SUBSCRIPTION_ID}" | tr -d '\n' | base64 | tr -d '\n')"
     AZURE_TENANT_ID_B64: "$(echo "${AZURE_TENANT_ID}" | tr -d '\n' | base64 | tr -d '\n')"
     AZURE_CLIENT_SECRET_B64: "$(echo "${AZURE_CLIENT_SECRET}" | tr -d '\n' | base64 | tr -d '\n')"
     AZURE_CLIENT_ID_B64: "$(echo "${AZURE_CLIENT_ID}" | tr -d '\n' | base64 | tr -d '\n')"
  EOF
```

{{#/tab }}
{{#tab DigitalOcean}}

```yaml
kustomize_substitutions:
  DO_B64ENCODED_CREDENTIALS: "your credentials here"
```

{{#/tab }}
{{#tab GCP}}

You can generate a base64 version of your GCP json credentials file using:
```bash
base64 -i ~/path/to/gcp/credentials.json
```

```yaml
kustomize_substitutions:
  GCP_B64ENCODED_CREDENTIALS: "your credentials here"
```

{{#/tab }}
{{#tab vSphere}}

```yaml
kustomize_substitutions:
  VSPHERE_USERNAME: "administrator@vsphere.local"
  VSPHERE_PASSWORD: "Admin123"
```

{{#/tab }}
{{#/tabs }}

**deploy_observability** ([string], default=[]): If set, installs on the dev cluster one of more observability
tools.
Important! This feature requires the `helm` command to be available in the user's path.

Supported values are:

  * `grafana`*: To create dashboards and query `loki`, `prometheus` and `tempo`.
  * `kube-state-metrics`: For exposing metrics for Kubernetes and CAPI resources to `prometheus`.
  * `loki`: To receive and store logs.
  * `metrics-server`: To enable `kubectl top node/pod`.
  * `prometheus`*: For collecting metrics from Kubernetes.
  * `promtail`: For providing pod logs to `loki`.
  * `parca`*: For visualizing profiling data.
  * `tempo`: To store traces.
  * `visualizer`*: Visualize Cluster API resources for each cluster, provide quick access to the specs and status of any resource.

\*: Note: the UI will be accessible via a link in the tilt console

**additional_kustomizations** (map[string]string, default={}): If set, install the additional resources built using kustomize to the cluster.
Example:
```yaml
additional_kustomizations:
  capv-metrics: ../cluster-api-provider-vsphere/config/metrics
```

**debug** (Map{string: Map} default{}): A map of named configurations for the provider. The key is the name of the provider.

Supported settings:

  * **port** (int, default=0 (disabled)): If set to anything other than 0, then Tilt will run the provider with delve
  and port forward the delve server to localhost on the specified debug port. This can then be used with IDEs such as
  Visual Studio Code, Goland and IntelliJ.

  * **continue** (bool, default=true): By default, Tilt will run delve with `--continue`, such that any provider with
    debugging turned on will run normally unless specifically having a breakpoint entered. Change to false if you
    do not want the controller to start at all by default.

  * **profiler_port** (int, default=0 (disabled)): If set to anything other than 0, then Tilt will enable the profiler with
  `--profiler-address` and set up a port forward. A "profiler" link will be visible in the Tilt Web UI for the controller.

  * **metrics_port** (int, default=0 (disabled)): If set to anything other than 0, then Tilt will port forward to the
    default metrics port. A "metrics" link will be visible in the Tilt Web UI for the controller.

  * **race_detector** (bool, default=false) (Linux amd64 only): If enabled, Tilt will compile the specified controller with
    cgo and statically compile in the system glibc and enable the race detector. Currently, this is only supported when
    building on Linux amd64 systems. You must install glibc-static or have libc.a available for this to work.

    Example: Using the configuration below:

    ```yaml
      debug:
        core:
          continue: false
          port: 30000
          profiler_port: 40000
          metrics_port: 40001
    ```

    ##### Wiring up debuggers
    ###### Visual Studio
    When using the example above, the core CAPI controller can be debugged in Visual Studio Code using the following launch configuration:

    ```json
    {
      "version": "0.2.0",
      "configurations": [
        {
          "name": "Core CAPI Controller",
          "type": "go",
          "request": "attach",
          "mode": "remote",
          "remotePath": "",
          "port": 30000,
          "host": "127.0.0.1",
          "showLog": true,
          "trace": "log",
          "logOutput": "rpc"
        }
      ]
    }
    ```

    ###### Goland / IntelliJ
    With the above example, you can configure [a Go Remote run/debug
    configuration](https://www.jetbrains.com/help/go/attach-to-running-go-processes-with-debugger.html#step-3-create-the-remote-run-debug-configuration-on-the-client-computer)
    pointing at port 30000.

<br/>

**deploy_cert_manager** (Boolean, default=`true`): Deploys cert-manager into the cluster for use for webhook registration.

**trigger_mode** (String, default=`auto`): Optional setting to configure if tilt should automatically rebuild on changes.
Set to `manual` to disable auto-rebuilding and require users to trigger rebuilds of individual changed components through the UI.

**extra_args** (Object, default={}): A mapping of provider to additional arguments to pass to the main binary configured
for this provider. Each item in the array will be passed in to the manager for the given provider.

Example:

```yaml
extra_args:
  kubeadm-bootstrap:
  - --logging-format=json
```

With this config, the respective managers will be invoked with:

```bash
manager --logging-format=json
```

### Create a kind cluster and run Tilt!

To create a pre-configured kind cluster (if you have not already done so) and launch your development environment, run

```bash
make tilt-up
```

This will open the command-line HUD as well as a web browser interface. You can monitor Tilt's status in either
location. After a brief amount of time, you should have a running development environment, and you should now be able to
create a cluster. There are [example worker cluster
configs](https://github.com/kubernetes-sigs/cluster-api/tree/main/test/infrastructure/docker/examples) available.
These can be customized for your specific needs.

### Deploying a workload cluster

After your kind management cluster is up and running with Tilt, you can deploy a workload clusters in the Tilt web UI based off of YAML templates from the directories specified in
the `template_dirs` field from the [tilt-settings.yaml](#tilt-settings-fields) file (default `./test/infrastructure/docker/templates`).

Templates should be named according to clusterctl conventions:

- template files must be named `cluster-template-{name}.yaml`; those files will be accessible in the Tilt web UI under the label grouping `{provider-label}.templates`, i.e. `CAPD.templates`.
- cluster class files must be named `clusterclass-{name}.yaml`; those file will be accessible in the Tilt web UI under the label grouping `{provider-label}.clusterclasses`, i.e. `CAPD.clusterclasses`.

By selecting one of those items in the Tilt web UI set of buttons will appear, allowing to create - with a dropdown for customizing variable substitutions - or delete clusters.
Custom values for variable substitutions can be set using `kustomize_substitutions` in `tilt-settings.yaml`, e.g.

```yaml
kustomize_substitutions:
  NAMESPACE: "default"
  KUBERNETES_VERSION: "v1.29.0"
  CONTROL_PLANE_MACHINE_COUNT: "1"
  WORKER_MACHINE_COUNT: "3"
# Note: kustomize substitutions expects the values to be strings. This can be achieved by wrapping the values in quotation marks.
```

### Cleaning up your kind cluster and development environment

After stopping Tilt, you can clean up your kind cluster and development environment by running

```bash
make clean-kind
```

To remove all generated files, run

```bash
make clean
```

Note that you must run `make clean` or `make clean-charts` to fetch new versions of charts deployed using `deploy_observability` in `tilt-settings.yaml`.

<h1>Use of clusterctl</h1>

When the worker cluster has been created using tilt, `clusterctl` should not be used for management
operations; this is because tilt doesn't initialize providers on the management cluster like clusterctl init does, so
some of the clusterctl commands like clusterctl config won't work.

This limitation is an acceptable trade-off while executing fast dev-test iterations on controllers logic. If instead
you are interested in testing clusterctl workflows, you should refer to the
[clusterctl developer instructions](../clusterctl/developers.md).

## Available providers

The following providers are currently defined in the Tiltfile:

* **core**: cluster-api itself
* **kubeadm-bootstrap**: kubeadm bootstrap provider
* **kubeadm-control-plane**: kubeadm control-plane provider
* **docker**: Docker infrastructure provider
* **in-memory**: In-memory infrastructure provider
* **test-extension**: Runtime extension used by CAPI E2E tests

Additional providers can be added by following the procedure described in following paragraphs:

### tilt-provider configuration

A provider must supply a `tilt-provider.yaml` file describing how to build it. Here is an example:

```yaml
name: aws
config:
  image: "gcr.io/k8s-staging-cluster-api-aws/cluster-api-aws-controller"
  live_reload_deps: ["main.go", "go.mod", "go.sum", "api", "cmd", "controllers", "pkg"]
  label: CAPA
```


<aside class="note">

If you prefer JSON, you can create a `tilt-provider.json` file instead. YAML will be preferred if both files are present.

</aside>

#### config fields

**image**: the image for this provider, as referenced in the kustomize files. This must match; otherwise, Tilt won't
build it.

**live_reload_deps**: a list of files/directories to watch. If any of them changes, Tilt rebuilds the manager binary
for the provider and performs a live update of the running container.

**version**: allows to define the version to be used for the Provider CR. If empty, a default version will 
be used.

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

**kustomize_folder** (String, default=config/default): The folder where the kustomize file for a provider
is defined; the path is relative to the provider root folder.

**kustomize_options** ([]String, default=[]): Options to be applied when running kustomize for generating the
yaml manifest for a provider. e.g. `"kustomize_options": [ "--load-restrictor=LoadRestrictionsNone" ]`

**apply_provider_yaml** (Bool, default=true): Whether to apply the provider yaml.
Set to `false` if your provider does not have a ./config folder or you do not want it to be applied in the cluster.

**go_main** (String, default="main.go"): The go main file if not located at the root of the folder

**label** (String, default=provider name): The label to be used to group provider components in the tilt UI
in tilt version >= v0.22.2 (see https://blog.tilt.dev/2021/08/09/resource-grouping.html); as a convention,
provider abbreviation should be used (CAPD, KCP etc.).

**additional_resources** ([]string, default=[]): A list of paths to yaml file to be loaded into the tilt cluster;
e.g. use this to deploy an ExtensionConfig object for a RuntimeExtension provider.

**resource_deps** ([]string, default=[]): A list of tilt resource names to be installed before the current provider;
e.g. set this to ["capi_controller"] to ensure that this provider gets installed after Cluster API.

## Customizing Tilt

If you need to customize Tilt's behavior, you can create files in cluster-api's `tilt.d` directory. This file is ignored
by git so you can be assured that any files you place here will never be checked in to source control.

These files are included after the `providers` map has been defined and after all the helper function definitions. This
is immediately before the "real work" happens.

## Under the covers, a.k.a "the real work"

At a high level, the Tiltfile performs the following actions:

1. Read `tilt-settings.yaml`
1. Configure the allowed Kubernetes contexts
1. Set the default registry
1. Define the `providers` map
1. Include user-defined Tilt files
1. Deploy cert-manager
1. Enable providers (`core` + what is listed in `tilt-settings.yaml`)
    1. Build the manager binary locally as a `local_resource`
    1. Invoke `docker_build` for the provider
    1. Invoke `kustomize` for the provider's `config/` directory

### Live updates

Each provider in the `providers` map has a `live_reload_deps` list. This defines the files and/or directories that Tilt
should monitor for changes. When a dependency is modified, Tilt rebuilds the provider's manager binary **on your local
machine**, copies the binary to the running container, and executes a restart script. This is significantly faster
than rebuilding the container image for each change. It also helps keep the size of each development image as small as
possible (the container images do not need the entire go toolchain, source code, module dependencies, etc.).

## IDE support for Tiltfile

For IntelliJ, Syntax highlighting for the Tiltfile can be configured with a TextMate Bundle. For instructions, please see:
[Tiltfile TextMate Bundle](https://github.com/tilt-dev/tiltfile.tmbundle).

For VSCode the [Bazel plugin](https://marketplace.visualstudio.com/items?itemName=BazelBuild.vscode-bazel) can be used, it provides
syntax highlighting and auto-formatting. To enable it for Tiltfile a file association has to be configured via user settings:
```json
"files.associations": {
  "Tiltfile": "starlark",
},
```

## Using Podman

[Podman](https://podman.io) can be used instead of Docker by following these actions:

1. Enable the podman unix socket:
   - on Linux/systemd: `systemctl --user enable --now podman.socket`
   - on macOS: create a podman machine with `podman machine init`
1. Set `build_engine` to `podman` in `tilt-settings.yaml` (optional, only if both Docker & podman are installed)
1. Define the env variable `DOCKER_HOST` to the right socket:
   - on Linux/systemd: `export DOCKER_HOST=unix:///run/user/$(id -u)/podman/podman.sock`
   - on macOS: `export DOCKER_HOST=$(podman machine inspect <machine> | jq -r '.[0].ConnectionInfo.PodmanSocket.Path')` where `<machine>` is the podman machine name
1. Run `tilt up`

NB: The socket defined by `DOCKER_HOST` is used only for the `hack/tools/internal/tilt-prepare` command, the image build is running the `podman build`/`podman push` commands.

## Troubleshooting Tilt

### Tilt is stuck

Sometimes tilt looks stuck when it's waiting on connections.

Ensure that docker/podman is up and running and your kubernetes cluster is reachable.

### Errors running tilt-prepare

#### `failed to get current context from the KubeConfig file`

- Ensure the cluster in the default context is reachable by running `kubectl cluster-info`
- Switch to the right context with `kubectl config use-context`
- Ensure the context is allowed, see [**allowed_contexts** field](#tilt-settings-fields)

#### `Cannot connect to the Docker daemon`

- Ensure the docker daemon is running ;) or for podman see [Using Podman](#using-podman)
- If a DOCKER_HOST is specified:
  - check that the DOCKER_HOST has the correct prefix (usually `unix://`)
  - ensure docker/podman is listening on $DOCKER_HOST using `fuser` / `lsof` / `netstat -u`

### Errors pulling/pushing to the registry

#### `connection refused` / `denied` / `not found`

Ensure the [**default_registry** field](#tilt-settings-fields) is a valid registry where you can pull and push images.

#### `server gave HTTP response to HTTPS client`

By default all registries except localhost:5000 are accessed via HTTPS.

If you run a HTTP registry you may have to configure the registry in docker/podman.

For example, in podman a `localhost:5001` registry configuration should be declared in `/etc/containers/registries.conf.d` with this content:
````
[[registry]]
location = "localhost:5001"
insecure = true
````

NB: on macOS this configuration should be done **in the podman machine** by running `podman machine ssh <machine>`.

### Errors loading images in kind

You may try manually to load images in kind by running:
````
kind load docker-image --name=<kind_cluster> <image>
````

#### `image: "..." not present locally`

If you are running podman, you may have hit this bug: https://github.com/kubernetes-sigs/kind/issues/2760

The workaround is to create a `docker` symlink to your `podman` executable and try to load the images again.
