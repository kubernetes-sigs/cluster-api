# Installation

## Common Prerequisites

- Install and setup [kubectl] in your local environment

## Install and/or configure a kubernetes cluster

Cluster API requires an existing Kubernetes cluster accessible via kubectl; during the installation process the 
Kubernetes cluster will be transformed into a [management cluster] by installing the Cluster API [provider components], so it 
is recommended to keep it separated from any application workload.

Please note that it is a common practice the creation of a temporary [bootstrap cluster] to be used to provision 
a target [management cluster] on the selected [infrastructure provider].

Choose one of the options below:

1. **Existing Management Cluster**

For production use-cases a "real" Kubernetes cluster should be used with appropriate backup and DR policies and procedures in place.

```bash
export KUBECONFIG=<...>
```

2. **Kind**

<aside class="note warning">

<h1>Warning</h1>

[kind] is not designed for production use.

**Minimum [kind] supported version**: v0.6.x

</aside>

[kind] can be used for creating a local Kubernetes cluster for development environments or for 
the creation of a temporary [bootstrap cluster] used to provision a target [management cluster] on the selected infrastructure provider.

  ```bash
  kind create cluster
  ```
Test to ensure the local kind cluster is ready:
```
kubectl cluster-info
```

## Provider Specific Prerequisites
Depending on the infrastructure provider you are planning to use, some additional prerequisites should be satisfied
before getting started with Cluster API.

{{#tabs name:"tab-installation-infrastructure" tabs:"AWS,Azure,Docker,GCP,vSphere,OpenStack,Metal3"}}
{{#tab AWS}}

Download the latest binary of `clusterawsadm` from the [AWS provider releases] and make sure to place it in your path.

```bash
# Create the base64 encoded credentials using clusterawsadm.
# This command uses your environment variables and encodes
# them in a value to be stored in a Kubernetes Secret.
export AWS_B64ENCODED_CREDENTIALS=$(clusterawsadm alpha bootstrap encode-aws-credentials)
```

See the [AWS Provider Prerequisites](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/master/docs/prerequisites.md) document for more details.

{{#/tab }}
{{#tab Azure}}

For more information about authorization, AAD, or requirements for Azure, visit the [Azure Provider Prerequisites](https://github.com/kubernetes-sigs/cluster-api-provider-azure/blob/master/docs/getting-started.md#prerequisites) document.

```bash
# Create the base64 encoded credentials
export AZURE_SUBSCRIPTION_ID_B64="$(echo -n "$AZURE_SUBSCRIPTION_ID" | base64 | tr -d '\n')"
export AZURE_TENANT_ID_B64="$(echo -n "$AZURE_TENANT_ID" | base64 | tr -d '\n')"
export AZURE_CLIENT_ID_B64="$(echo -n "$AZURE_CLIENT_ID" | base64 | tr -d '\n')"
export AZURE_CLIENT_SECRET_B64="$(echo -n "$AZURE_CLIENT_SECRET" | base64 | tr -d '\n')"
```

{{#/tab }}
{{#tab Docker}}

If you are planning to use to test locally Cluster API using the Docker infrastructure provider, please follow additional
steps described in the [developer instruction](../clusterctl/developers.md#additional-steps-in-order-to-use-the-docker-provider) page.

{{#/tab }}
{{#tab GCP}}

```bash
# Create the base64 encoded credentials by catting your credentials json.
# This command uses your environment variables and encodes
# them in a value to be stored in a Kubernetes Secret.
export GCP_B64ENCODED_CREDENTIALS=$( cat /path/to/gcp-credentials.json | base64 | tr -d '\n' )
```

{{#/tab }}
{{#tab vSphere}}

It is required to use an official CAPV machine image for your vSphere VM templates. See [Uploading CAPV Machine Images](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/docs/getting_started.md#uploading-the-capv-machine-image) for instructions on how to do this.

Then, it is required Upload vCenter credentials as a Kubernetes secret:

```bash
$ cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  labels:
    control-plane: controller-manager
  name: capv-system
---
apiVersion: v1
kind: Secret
metadata:
  name: capv-manager-bootstrap-credentials
  namespace: capv-system
type: Opaque
stringData:
  username: "<my vCenter username>"
  password: "<my vCenter password>"
EOF
```

For more information about prerequisites, credentials management, or permissions for vSphere, visit the [getting started guide](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/docs/getting_started.md).

{{#/tab }}
{{#tab OpenStack}}

Please visit the [OpenStack getting started guide](https://github.com/kubernetes-sigs/cluster-api-provider-openstack/blob/master/docs/getting-started.md).

{{#/tab }}
{{#tab Metal3}}

Please visit the [Metal3 getting started guide](https://github.com/metal3-io/cluster-api-provider-metal3/blob/master/docs/getting-started.md).

{{#/tab }}
{{#/tabs }}

## Install clusterctl
The clusterctl CLI tool handles the lifecycle of a Cluster API management cluster.

{{#tabs name:"install-clusterctl" tabs:"linux,macOS"}}
{{#tab linux}}

#### Install clusterctl binary with curl on linux
Download the latest release; for example, to download version v0.3.0 on linux, type:
```
curl -L {{#releaselink gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-linux-amd64" version:"0.3.x"}} -o clusterctl
```
Make the kubectl binary executable.
```
chmod +x ./clusterctl
```
Move the binary in to your PATH.
```
sudo mv ./clusterctl /usr/local/bin/clusterctl
```
Test to ensure the version you installed is up-to-date:
```
clusterctl version
```

{{#/tab }}
{{#tab macOS}}

#### Install clusterctl binary with curl on macOS
Download the latest release; for example, to download version v0.3.0 on macOS, type:
```
curl -L {{#releaselink gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-darwin-amd64" version:"0.3.x"}} -o clusterctl
```
Make the kubectl binary executable.
```
chmod +x ./clusterctl
```
Move the binary in to your PATH.
```
sudo mv ./clusterctl /usr/local/bin/clusterctl
```
Test to ensure the version you installed is up-to-date:
```
clusterctl version
```
{{#/tab }}
{{#/tabs }}

<!-- links -->
[AWS provider releases]: https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases
[bootstrap cluster]: ../reference/glossary.md#bootstrap-cluster
[infrastructure provider]: ../reference/glossary.md#infrastructure-provider
[kubectl]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[kind]: https://kind.sigs.k8s.io/
[management cluster]: ../reference/glossary.md#management-cluster
[provider components]: ../reference/glossary.md#provider-components