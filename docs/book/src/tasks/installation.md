# Installation

## Prerequisites

- Install and setup [kubectl] in your local environment.
- Install and/or configure a [management cluster]

## Setup Management Cluster

Cluster API requires an existing kubernetes cluster accessible via kubectl, choose one of the options below:

1. **Kind**

{{#tabs name:"kind-cluster" tabs:"AWS|Azure|GCP|vSphere|OpenStack,Docker"}}
{{#tab AWS|Azure|GCP|vSphere|OpenStack}}

<aside class="note warning">

<h1>Warning</h1>

**Minimum [kind] supported version**: v0.6.x

[kind] is not designed for production use, and is intended for development environments only.

</aside>

  ```bash
  kind create cluster --name=clusterapi
  kubectl cluster-info --context kind-clusterapi
  ```
{{#/tab }}
{{#tab Docker}}

<aside class="note warning">

<h1>Warning</h1>

**Minimum [kind] supported version**: v0.6.x

[kind] is not designed for production use, and is intended for development environments only.
</aside>

<aside class="note warning">

<h1>Warning</h1>

The Docker provider is not designed for production use and is intended for development environments only.

</aside>

  Because the Docker provider needs to access Docker on the host, a custom kind cluster configuration is required:

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
  kind create cluster --config ./kind-cluster-with-extramounts.yaml --name clusterapi
  kubectl cluster-info --context kind-clusterapi
  ```
{{#/tab }}
{{#/tabs }}


2. **Existing Management Cluster**

For production use-cases a "real" kubernetes cluster should be used with appropriate backup and DR policies and procedures in place.

```bash
export KUBECONFIG=<...>
```

3. **Pivoting**

Pivoting is the process of taking an initial kind cluster to create a new workload cluster, and then converting the workload cluster into a management cluster by migrating the Cluster API CRD's.


## Installation

Using [kubectl], create the components on the [management cluster]:

#### Install Cluster API

```bash
kubectl create -f {{#releaselink gomodule:"sigs.k8s.io/cluster-api" asset:"cluster-api-components.yaml" version:"0.2.x"}}
```

#### Install the Bootstrap Provider

{{#tabs name:"tab-installation-bootstrap" tabs:"Kubeadm"}}
{{#tab Kubeadm}}

Check the [Kubeadm provider releases](https://github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm/releases) for an up-to-date components file.

```bash
kubectl create -f {{#releaselink gomodule:"sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm" asset:"bootstrap-components.yaml" version:"0.1.x"}}
```

{{#/tab }}
{{#/tabs }}


#### Install Infrastructure Provider

{{#tabs name:"tab-installation-infrastructure" tabs:"AWS,Azure,Docker,GCP,vSphere,OpenStack"}}
{{#tab AWS}}

<aside class="note warning">

<h1>Action Required</h1>

For more information about credentials management, IAM, or requirements for AWS, visit the [AWS Provider Prerequisites](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/master/docs/prerequisites.md) document.

</aside>

#### Install clusterawsadm

Download the latest binary of `clusterawsadm` from the [AWS provider releases] and make sure to place it in your path.

##### Create the components

Check the [AWS provider releases] for an up-to-date components file.

```bash
# Create the base64 encoded credentials using clusterawsadm.
# This command uses your environment variables and encodes
# them in a value to be stored in a Kubernetes Secret.
export AWS_B64ENCODED_CREDENTIALS=$(clusterawsadm alpha bootstrap encode-aws-credentials)

The `envsubst` application is provided by the `gettext` package on Linux and via Brew on MacOS.

# Create the components.
curl -L {{#releaselink gomodule:"sigs.k8s.io/cluster-api-provider-aws" asset:"infrastructure-components.yaml" version:"0.4.x"}} \
  | envsubst \
  | kubectl create -f -
```

{{#/tab }}
{{#tab Azure}}

<aside class="note warning">

<h1>Action Required</h1>

For more information about authorization, AAD, or requirements for Azure, visit the [Azure Provider Prerequisites](https://github.com/kubernetes-sigs/cluster-api-provider-azure/blob/master/docs/getting-started.md#prerequisites) document.

</aside>

Check the [Azure provider releases](https://github.com/kubernetes-sigs/cluster-api-provider-azure/releases) for an up-to-date components file.

```bash
# Create the base64 encoded credentials
export AZURE_SUBSCRIPTION_ID_B64="$(echo -n "$AZURE_SUBSCRIPTION_ID" | base64 | tr -d '\n')"
export AZURE_TENANT_ID_B64="$(echo -n "$AZURE_TENANT_ID" | base64 | tr -d '\n')"
export AZURE_CLIENT_ID_B64="$(echo -n "$AZURE_CLIENT_ID" | base64 | tr -d '\n')"
export AZURE_CLIENT_SECRET_B64="$(echo -n "$AZURE_CLIENT_SECRET" | base64 | tr -d '\n')"
```

```bash
curl -L {{#releaselink gomodule:"sigs.k8s.io/cluster-api-provider-azure" asset:"infrastructure-components.yaml" version:"0.3.x"}} \
  | envsubst \
  | kubectl create -f -
```

{{#/tab }}
{{#tab Docker}}

Check the [Docker provider releases](https://github.com/kubernetes-sigs/cluster-api-provider-docker/releases) for an up-to-date components file.

```bash
kubectl create -f {{#releaselink gomodule:"sigs.k8s.io/cluster-api-provider-docker" asset:"provider-components.yaml" version:"0.2.x"}}
```

{{#/tab }}
{{#tab GCP}}

<aside class="note warning">

<h1>Action Required</h1>

Update the path to your GCP credentials file below.

</aside>

##### Create the components

Check the [GCP provider releases](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/releases) for an up-to-date components file.

```bash
# Create the base64 encoded credentials by catting your credentials json.
# This command uses your environment variables and encodes
# them in a value to be stored in a Kubernetes Secret.
export GCP_B64ENCODED_CREDENTIALS=$( cat /path/to/gcp-credentials.json | base64 | tr -d '\n' )

# Create the components.
curl -L {{#releaselink gomodule:"sigs.k8s.io/cluster-api-provider-gcp" asset:"infrastructure-components.yaml" version:"0.3.x"}} \
  | envsubst \
  | kubectl create -f -
```

{{#/tab }}
{{#tab vSphere}}

It is required to use an official CAPV machine image for your vSphere VM templates. See [Uploading CAPV Machine Images](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/docs/getting_started.md#uploading-the-capv-machine-image) for instructions on how to do this.

```bash
# Upload vCenter credentials as a Kubernetes secret
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

$ kubectl create -f {{#releaselink gomodule:"sigs.k8s.io/cluster-api-provider-vsphere" asset:"infrastructure-components.yaml" version:"0.5.x"}}
```

Check the [vSphere provider releases](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/releases) for an up-to-date components file.

For more information about prerequisites, credentials management, or permissions for vSphere, visit the [getting started guide](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/docs/getting_started.md).

{{#/tab }}
{{#tab OpenStack}}

Check the [OpenStack provider releases](https://github.com/kubernetes-sigs/cluster-api-provider-openstack/releases) for an up-to-date components file.

For more detailed information, e.g. about prerequisites visit the [getting started guide](https://github.com/kubernetes-sigs/cluster-api-provider-openstack/blob/master/docs/getting-started.md).

```bash
kubectl create -f {{#releaselink gomodule:"sigs.k8s.io/cluster-api-provider-openstack" asset:"infrastructure-components.yaml" version:"0.2.x"}}
```

{{#/tab }}
{{#/tabs }}


<!-- links -->
[kubectl]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[components]: ../reference/glossary.md#provider-components
[kind]: https://sigs.k8s.io/kind
[management cluster]: ../reference/glossary.md#management-cluster
[target cluster]: ../reference/glossary.md#target-cluster
[AWS provider releases]: https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases
