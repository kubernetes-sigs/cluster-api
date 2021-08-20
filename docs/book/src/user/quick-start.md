# Quick Start

In this tutorial we'll cover the basics of how to use Cluster API to create one or more Kubernetes clusters.

<aside class="note warning">

<h1>Warning</h1>

If using a [provider] that does not yet support v1alpha4, please follow [the release 0.3 quickstart instructions](https://release-0-3.cluster-api.sigs.k8s.io/user/quick-start.html) instead.

</aside>

## Installation

### Common Prerequisites

- Install and setup [kubectl] in your local environment
- Install [Kind] and [Docker]

### Install and/or configure a Kubernetes cluster

Cluster API requires an existing Kubernetes cluster accessible via kubectl. During the installation process the
Kubernetes cluster will be transformed into a [management cluster] by installing the Cluster API [provider components], so it
is recommended to keep it separated from any application workload.

It is a common practice to create a temporary, local bootstrap cluster which is then used to provision
a target [management cluster] on the selected [infrastructure provider].

Choose one of the options below:

1. **Existing Management Cluster**

   For production use-cases a "real" Kubernetes cluster should be used with appropriate backup and DR policies and procedures in place. The Kubernetes cluster must be at least v1.19.1.

   ```bash
   export KUBECONFIG=<...>
   ```

2. **Kind**

   <aside class="note warning">

   <h1>Warning</h1>

   [kind] is not designed for production use.

   **Minimum [kind] supported version**: v0.9.0

   Note for macOS users: you may need to [increase the memory available](https://docs.docker.com/docker-for-mac/#resources) for containers (recommend 6Gb for CAPD).

   </aside>

   [kind] can be used for creating a local Kubernetes cluster for development environments or for
   the creation of a temporary [bootstrap cluster] used to provision a target [management cluster] on the selected infrastructure provider.

   The installation procedure depends on the version of kind; if you are planning to use the Docker infrastructure provider,
   please follow the additional instructions in the dedicated tab:

   {{#tabs name:"install-kind" tabs:"Default,Docker"}}
   {{#tab Default}}

   Create the kind cluster:
   ```bash
   kind create cluster
   ```
   Test to ensure the local kind cluster is ready:
   ```
   kubectl cluster-info
   ```

   {{#/tab }}
   {{#tab Docker}}

   Run the following command to create a kind config file for allowing the Docker provider to access Docker on the host:

   ```bash
   cat > kind-cluster-with-extramounts.yaml <<EOF
   kind: Cluster
   apiVersion: kind.x-k8s.io/v1alpha4
   nodes:
   - role: control-plane
     extraMounts:
       - hostPath: /var/run/docker.sock
         containerPath: /var/run/docker.sock
   EOF
   ```

   Then follow the instruction for your kind version using  `kind create cluster --config kind-cluster-with-extramounts.yaml`
   to create the management cluster using the above file.

   {{#/tab }}
   {{#/tabs }}

### Install clusterctl
The clusterctl CLI tool handles the lifecycle of a Cluster API management cluster.

{{#tabs name:"install-clusterctl" tabs:"linux,macOS,homebrew"}}
{{#tab linux}}

#### Install clusterctl binary with curl on linux
Download the latest release; on linux, type:
```
curl -L {{#releaselink gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-linux-amd64" version:"0.4.x"}} -o clusterctl
```
Make the clusterctl binary executable.
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
Download the latest release; on macOS, type:
```
curl -L {{#releaselink gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-darwin-amd64" version:"0.4.x"}} -o clusterctl
```

Or if your Mac has an M1 CPU ("Apple Silicon"):
```
curl -L {{#releaselink gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-darwin-arm64" version:"0.4.x"}} -o clusterctl
```

Make the clusterctl binary executable.
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
{{#tab homebrew}}

#### Install clusterctl with homebrew on macOS and linux

Install the latest release using homebrew:

```bash
brew install clusterctl
```

Test to ensure the version you installed is up-to-date:
```
clusterctl version
```

{{#/tab }}
{{#/tabs }}

### Initialize the management cluster

Now that we've got clusterctl installed and all the prerequisites in place, let's transform the Kubernetes cluster
into a management cluster by using `clusterctl init`.

The command accepts as input a list of providers to install; when executed for the first time, `clusterctl init`
automatically adds to the list the `cluster-api` core provider, and if unspecified, it also adds the `kubeadm` bootstrap
and `kubeadm` control-plane providers.

#### Initialization for common providers

Depending on the infrastructure provider you are planning to use, some additional prerequisites should be satisfied
before getting started with Cluster API. See below for the expected settings for common providers.

{{#tabs name:"tab-installation-infrastructure" tabs:"AWS,Azure,DigitalOcean,Docker,GCP,vSphere,OpenStack,Metal3,Packet"}}
{{#tab AWS}}

Download the latest binary of `clusterawsadm` from the [AWS provider releases] and make sure to place it in your path.

The [clusterawsadm] command line utility assists with identity and access management (IAM) for [Cluster API Provider AWS][capa].

```bash
export AWS_REGION=us-east-1 # This is used to help encode your environment variables
export AWS_ACCESS_KEY_ID=<your-access-key>
export AWS_SECRET_ACCESS_KEY=<your-secret-access-key>
export AWS_SESSION_TOKEN=<session-token> # If you are using Multi-Factor Auth.

# The clusterawsadm utility takes the credentials that you set as environment
# variables and uses them to create a CloudFormation stack in your AWS account
# with the correct IAM resources.
clusterawsadm bootstrap iam create-cloudformation-stack

# Create the base64 encoded credentials using clusterawsadm.
# This command uses your environment variables and encodes
# them in a value to be stored in a Kubernetes Secret.
export AWS_B64ENCODED_CREDENTIALS=$(clusterawsadm bootstrap credentials encode-as-profile)

# Finally, initialize the management cluster
clusterctl init --infrastructure aws
```

See the [AWS provider prerequisites] document for more details.

{{#/tab }}
{{#tab Azure}}

For more information about authorization, AAD, or requirements for Azure, visit the [Azure provider prerequisites] document.

```bash
export AZURE_SUBSCRIPTION_ID="<SubscriptionId>"

# Create an Azure Service Principal and paste the output here
export AZURE_TENANT_ID="<Tenant>"
export AZURE_CLIENT_ID="<AppId>"
export AZURE_CLIENT_SECRET="<Password>"

# Base64 encode the variables
export AZURE_SUBSCRIPTION_ID_B64="$(echo -n "$AZURE_SUBSCRIPTION_ID" | base64 | tr -d '\n')"
export AZURE_TENANT_ID_B64="$(echo -n "$AZURE_TENANT_ID" | base64 | tr -d '\n')"
export AZURE_CLIENT_ID_B64="$(echo -n "$AZURE_CLIENT_ID" | base64 | tr -d '\n')"
export AZURE_CLIENT_SECRET_B64="$(echo -n "$AZURE_CLIENT_SECRET" | base64 | tr -d '\n')"

# Settings needed for AzureClusterIdentity used by the AzureCluster
export AZURE_CLUSTER_IDENTITY_SECRET_NAME="cluster-identity-secret"
export CLUSTER_IDENTITY_NAME="cluster-identity"
export AZURE_CLUSTER_IDENTITY_SECRET_NAMESPACE="default"

# Create a secret to include the password of the Service Principal identity created in Azure
# This secret will be referenced by the AzureClusterIdentity used by the AzureCluster
kubectl create secret generic "${AZURE_CLUSTER_IDENTITY_SECRET_NAME}" --from-literal=clientSecret="${AZURE_CLIENT_SECRET}"

# Finally, initialize the management cluster
clusterctl init --infrastructure azure
```

{{#/tab }}
{{#tab DigitalOcean}}

```bash
export DIGITALOCEAN_ACCESS_TOKEN=<your-access-token>
export DO_B64ENCODED_CREDENTIALS="$(echo -n "${DIGITALOCEAN_ACCESS_TOKEN}" | base64 | tr -d '\n')"

# Initialize the management cluster
clusterctl init --infrastructure digitalocean
```

{{#/tab }}
{{#tab Docker}}

<aside class="note warning">

<h1>Warning</h1>

The Docker provider is not designed for production use and is intended for development environments only.

</aside>

The Docker provider does not require additional prerequisites.
You can run:

```
clusterctl init --infrastructure docker
```

{{#/tab }}
{{#tab GCP}}

```bash
# Create the base64 encoded credentials by catting your credentials json.
# This command uses your environment variables and encodes
# them in a value to be stored in a Kubernetes Secret.
export GCP_B64ENCODED_CREDENTIALS=$( cat /path/to/gcp-credentials.json | base64 | tr -d '\n' )

# Finally, initialize the management cluster
clusterctl init --infrastructure gcp
```

{{#/tab }}
{{#tab vSphere}}

```bash
# The username used to access the remote vSphere endpoint
export VSPHERE_USERNAME="vi-admin@vsphere.local"
# The password used to access the remote vSphere endpoint
# You may want to set this in ~/.cluster-api/clusterctl.yaml so your password is not in
# bash history
export VSPHERE_PASSWORD="admin!23"

# Finally, initialize the management cluster
clusterctl init --infrastructure vsphere
```

For more information about prerequisites, credentials management, or permissions for vSphere, see the [vSphere
project][vSphere getting started guide].

{{#/tab }}
{{#tab OpenStack}}

```bash
# Initialize the management cluster
clusterctl init --infrastructure openstack
```

{{#/tab }}
{{#tab Metal3}}

Please visit the [Metal3 project][Metal3 provider].

{{#/tab }}
{{#tab Packet}}

In order to initialize the Packet Provider you have to expose the environment
variable `PACKET_API_KEY`. This variable is used to authorize the infrastructure
provider manager against the Packet API. You can retrieve your token directly
from the [Packet Portal](https://app.packet.net/).

```bash
export PACKET_API_KEY="34ts3g4s5g45gd45dhdh"

clusterctl init --infrastructure packet
```

{{#/tab }}

{{#/tabs }}

The output of `clusterctl init` is similar to this:

```shell
Fetching providers
Installing cert-manager Version="v1.5.0"
Waiting for cert-manager to be available...
Installing Provider="cluster-api" Version="v0.4.0" TargetNamespace="capi-system"
Installing Provider="bootstrap-kubeadm" Version="v0.4.0" TargetNamespace="capi-kubeadm-bootstrap-system"
Installing Provider="control-plane-kubeadm" Version="v0.4.0" TargetNamespace="capi-kubeadm-control-plane-system"
Installing Provider="infrastructure-docker" Version="v0.4.0" TargetNamespace="capd-system"

Your management cluster has been initialized successfully!

You can now create your first workload cluster by running the following:

  clusterctl generate cluster [name] --kubernetes-version [version] | kubectl apply -f -
```

<aside class="note">

<h1>Alternatives to environment variables</h1>

Throughout this quickstart guide we've given instructions on setting parameters using environment variables. For most
environment variables in the rest of the guide, you can also set them in ~/.cluster-api/clusterctl.yaml

See [`clusterctl init`](../clusterctl/commands/init.md) for more details.

</aside>

### Create your first workload cluster

Once the management cluster is ready, you can create your first workload cluster.

#### Preparing the workload cluster configuration

The `clusterctl generate cluster` command returns a YAML template for creating a [workload cluster].

<aside class="note">

<h1> Which provider will be used for my cluster? </h1>

The `clusterctl generate cluster` command uses smart defaults in order to simplify the user experience; for example,
if only the `aws` infrastructure provider is deployed, it detects and uses that when creating the cluster.

</aside>

<aside class="note">

<h1> What topology will be used for my cluster? </h1>

The `clusterctl generate cluster` command by default uses cluster templates which are provided by the infrastructure
providers. See the provider's documentation for more information.

See the `clusterctl generate cluster` [command][clusterctl generate cluster] documentation for
details about how to use alternative sources. for cluster templates.

</aside>

#### Required configuration for common providers

Depending on the infrastructure provider you are planning to use, some additional prerequisites should be satisfied
before configuring a cluster with Cluster API. Instructions are provided for common providers below.

Otherwise, you can look at the `clusterctl generate cluster` [command][clusterctl generate cluster] documentation for details about how to
discover the list of variables required by a cluster templates.

{{#tabs name:"tab-configuration-infrastructure" tabs:"AWS,Azure,DigitalOcean,Docker,GCP,vSphere,OpenStack,Metal3,Packet"}}
{{#tab AWS}}

```bash
export AWS_REGION=us-east-1
export AWS_SSH_KEY_NAME=default
# Select instance types
export AWS_CONTROL_PLANE_MACHINE_TYPE=t3.large
export AWS_NODE_MACHINE_TYPE=t3.large
```

See the [AWS provider prerequisites] document for more details.

{{#/tab }}
{{#tab Azure}}

<aside class="note warning">

<h1>Warning</h1>

Make sure you choose a VM size which is available in the desired location for your subscription. To see available SKUs, use `az vm list-skus -l <your_location> -r virtualMachines -o table`

</aside>

```bash
# Name of the Azure datacenter location. Change this value to your desired location.
export AZURE_LOCATION="centralus"

# Select VM types.
export AZURE_CONTROL_PLANE_MACHINE_TYPE="Standard_D2s_v3"
export AZURE_NODE_MACHINE_TYPE="Standard_D2s_v3"
```

{{#/tab }}
{{#tab DigitalOcean}}

A ClusterAPI compatible image must be available in your DigitalOcean account. For instructions on how to build a compatible image
see [image-builder](https://image-builder.sigs.k8s.io/capi/capi.html).

```bash
export DO_REGION=nyc1
export DO_SSH_KEY_FINGERPRINT=<your-ssh-key-fingerprint>
export DO_CONTROL_PLANE_MACHINE_TYPE=s-2vcpu-2gb
export DO_CONTROL_PLANE_MACHINE_IMAGE=<your-capi-image-id>
export DO_NODE_MACHINE_TYPE=s-2vcpu-2gb
export DO_NODE_MACHINE_IMAGE==<your-capi-image-id>
```

{{#/tab }}
{{#tab Docker}}

<aside class="note warning">

<h1>Warning</h1>

The Docker provider is not designed for production use and is intended for development environments only.

</aside>

The Docker provider does not require additional configurations for cluster templates.

However, if you require special network settings you can set the following environment variables:

```bash
# The list of service CIDR, default ["10.128.0.0/12"]
export SERVICE_CIDR=["10.96.0.0/12"]

# The list of pod CIDR, default ["192.168.0.0/16"]
export POD_CIDR=["192.168.0.0/16"]

# The service domain, default "cluster.local"
export SERVICE_DOMAIN="k8s.test"
```

{{#/tab }}
{{#tab GCP}}


```bash
# Name of the GCP datacenter location. Change this value to your desired location
export GCP_REGION="<GCP_REGION>"
export GCP_PROJECT="<GCP_PROJECT>"
# Make sure to use same kubernetes version here as building the GCE image
export KUBERNETES_VERSION=1.20.9
export GCP_CONTROL_PLANE_MACHINE_TYPE=n1-standard-2
export GCP_NODE_MACHINE_TYPE=n1-standard-2
export GCP_NETWORK_NAME=<GCP_NETWORK_NAME or default>
export CLUSTER_NAME="<CLUSTER_NAME>"
```

See the [GCP provider] for more information.

{{#/tab }}
{{#tab vSphere}}

It is required to use an official CAPV machine images for your vSphere VM templates. See [uploading CAPV machine images][capv-upload-images] for instructions on how to do this.

```bash
# The vCenter server IP or FQDN
export VSPHERE_SERVER="10.0.0.1"
# The vSphere datacenter to deploy the management cluster on
export VSPHERE_DATACENTER="SDDC-Datacenter"
# The vSphere datastore to deploy the management cluster on
export VSPHERE_DATASTORE="vsanDatastore"
# The VM network to deploy the management cluster on
export VSPHERE_NETWORK="VM Network"
# The vSphere resource pool for your VMs
export VSPHERE_RESOURCE_POOL="*/Resources"
# The VM folder for your VMs. Set to "" to use the root vSphere folder
export VSPHERE_FOLDER="vm"
# The VM template to use for your VMs
export VSPHERE_TEMPLATE="ubuntu-1804-kube-v1.17.3"
# The VM template to use for the HAProxy load balancer of the management cluster
export VSPHERE_HAPROXY_TEMPLATE="capv-haproxy-v0.6.0-rc.2"
# The public ssh authorized key on all machines
export VSPHERE_SSH_AUTHORIZED_KEY="ssh-rsa AAAAB3N..."

clusterctl init --infrastructure vsphere
```

For more information about prerequisites, credentials management, or permissions for vSphere, see the [vSphere getting started guide].

{{#/tab }}
{{#tab OpenStack}}

A ClusterAPI compatible image must be available in your OpenStack. For instructions on how to build a compatible image
see [image-builder](https://image-builder.sigs.k8s.io/capi/capi.html).
Depending on your OpenStack and underlying hypervisor the following options might be of interest:
* [image-builder (OpenStack)](https://image-builder.sigs.k8s.io/capi/providers/openstack.html)
* [image-builder (vSphere)](https://image-builder.sigs.k8s.io/capi/providers/vsphere.html)

To see all required OpenStack environment variables execute:
```bash
clusterctl generate cluster --infrastructure openstack --list-variables capi-quickstart
```

The following script can be used to export some of them:
```bash
wget https://raw.githubusercontent.com/kubernetes-sigs/cluster-api-provider-openstack/master/templates/env.rc -O /tmp/env.rc
source /tmp/env.rc <path/to/clouds.yaml> <cloud>
```

Apart from the script, the following OpenStack environment variables are required.
```bash
# The list of nameservers for OpenStack Subnet being created.
# Set this value when you need create a new network/subnet while the access through DNS is required.
export OPENSTACK_DNS_NAMESERVERS=<dns nameserver>
# FailureDomain is the failure domain the machine will be created in.
export OPENSTACK_FAILURE_DOMAIN=<availability zone name>
# The flavor reference for the flavor for your server instance.
export OPENSTACK_CONTROL_PLANE_MACHINE_FLAVOR=<flavor>
# The flavor reference for the flavor for your server instance.
export OPENSTACK_NODE_MACHINE_FLAVOR=<flavor>
# The name of the image to use for your server instance. If the RootVolume is specified, this will be ignored and use rootVolume directly.
export OPENSTACK_IMAGE_NAME=<image name>
# The SSH key pair name
export OPENSTACK_SSH_KEY_NAME=<ssh key pair name>
```

A full configuration reference can be found in [configuration.md](https://github.com/kubernetes-sigs/cluster-api-provider-openstack/blob/master/docs/book/src/clusteropenstack/configuration.md).

{{#/tab }}
{{#tab Metal3}}

```bash
# The URL of the kernel to deploy.
export DEPLOY_KERNEL_URL="http://172.22.0.1:6180/images/ironic-python-agent.kernel"
# The URL of the ramdisk to deploy.
export DEPLOY_RAMDISK_URL="http://172.22.0.1:6180/images/ironic-python-agent.initramfs"
# The URL of the Ironic endpoint.
export IRONIC_URL="http://172.22.0.1:6385/v1/"
# The URL of the Ironic inspector endpoint.
export IRONIC_INSPECTOR_URL="http://172.22.0.1:5050/v1/"
# Do not use a dedicated CA certificate for Ironic API. Any value provided in this variable disables additional CA certificate validation.
# To provide a CA certificate, leave this variable unset. If unset, then IRONIC_CA_CERT_B64 must be set.
export IRONIC_NO_CA_CERT=true
# Disables basic authentication for Ironic API. Any value provided in this variable disables authentication.
# To enable authentication, leave this variable unset. If unset, then IRONIC_USERNAME and IRONIC_PASSWORD must be set.
export IRONIC_NO_BASIC_AUTH=true
# Disables basic authentication for Ironic inspector API. Any value provided in this variable disables authentication.
# To enable authentication, leave this variable unset. If unset, then IRONIC_INSPECTOR_USERNAME and IRONIC_INSPECTOR_PASSWORD must be set.
export IRONIC_INSPECTOR_NO_BASIC_AUTH=true
```

Please visit the [Metal3 getting started guide] for more details.

{{#/tab }}
{{#tab Packet}}

There are a couple of required environment variables that you have to expose in
order to get a well tuned and function workload, they are all listed here:

```bash
# The project where your cluster will be placed to.
# You have to get out from Packet Portal if you do not have one already.
export PROJECT_ID="5yd4thd-5h35-5hwk-1111-125gjej40930"
# The facility where you want your cluster to be provisioned
export FACILITY="ewr1"
# The operatin system used to provision the device
export NODE_OS="ubuntu_18_04"
# The ssh key name you loaded in Packet Portal
export SSH_KEY="my-ssh"
export POD_CIDR="192.168.0.0/16"
export SERVICE_CIDR="172.26.0.0/16"
export CONTROLPLANE_NODE_TYPE="t1.small"
export WORKER_NODE_TYPE="t1.small"
```

{{#/tab }}
{{#/tabs }}

#### Generating the cluster configuration

For the purpose of this tutorial, we'll name our cluster capi-quickstart.

{{#tabs name:"tab-clusterctl-config-cluster" tabs:"Azure|AWS|DigitalOcean|GCP|vSphere|OpenStack|Metal3|Packet,Docker"}}
{{#tab Azure|AWS|DigitalOcean|GCP|vSphere|OpenStack|Metal3|Packet}}

```bash
clusterctl generate cluster capi-quickstart \
  --kubernetes-version v1.22.0 \
  --control-plane-machine-count=3 \
  --worker-machine-count=3 \
  > capi-quickstart.yaml
```

{{#/tab }}
{{#tab Docker}}

<aside class="note warning">

<h1>Warning</h1>

The Docker provider is not designed for production use and is intended for development environments only.

</aside>

```bash
clusterctl generate cluster capi-quickstart --flavor development \
  --kubernetes-version v1.22.0 \
  --control-plane-machine-count=3 \
  --worker-machine-count=3 \
  > capi-quickstart.yaml
```

{{#/tab }}
{{#/tabs }}

This creates a YAML file named `capi-quickstart.yaml` with a predefined list of Cluster API objects; Cluster, Machines,
Machine Deployments, etc.

The file can be eventually modified using your editor of choice.

See [clusterctl generate cluster] for more details.

#### Apply the workload cluster

When ready, run the following command to apply the cluster manifest.

```bash
kubectl apply -f capi-quickstart.yaml
```

The output is similar to this:

```bash
cluster.cluster.x-k8s.io/capi-quickstart created
awscluster.infrastructure.cluster.x-k8s.io/capi-quickstart created
kubeadmcontrolplane.controlplane.cluster.x-k8s.io/capi-quickstart-control-plane created
awsmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-control-plane created
machinedeployment.cluster.x-k8s.io/capi-quickstart-md-0 created
awsmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-md-0 created
kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io/capi-quickstart-md-0 created
```

#### Accessing the workload cluster

The cluster will now start provisioning. You can check status with:

```bash
kubectl get cluster
```

You can also get an "at glance" view of the cluster and its resources by running:

```bash
clusterctl describe cluster capi-quickstart
```

To verify the first control plane is up:

```bash
kubectl get kubeadmcontrolplane
```

You should see an output is similar to this:

```bash
NAME                            INITIALIZED   API SERVER AVAILABLE   VERSION   REPLICAS   READY   UPDATED   UNAVAILABLE
capi-quickstart-control-plane   true                                 v1.21.2   3                  3         3
```

<aside class="note warning">

<h1> Warning </h1>

The control plane won't be `Ready` until we install a CNI in the next step.

</aside>

After the first control plane node is up and running, we can retrieve the [workload cluster] Kubeconfig:

```bash
clusterctl get kubeconfig capi-quickstart > capi-quickstart.kubeconfig
```

<aside class="note warning">

<h1>Warning</h1>

If you are using Docker on MacOS, you will need to do a couple of additional
steps to get the correct kubeconfig for a workload cluster created with the Docker provider.
See [Additional Notes for the Docker Provider](../clusterctl/developers.md#additional-notes-for-the-docker-provider).

</aside>

### Deploy a CNI solution

Calico is used here as an example.

{{#tabs name:"tab-deploy-cni" tabs:"AWS|DigitalOcean|Docker|GCP|vSphere|OpenStack|Metal3|Packet,Azure"}}
{{#tab AWS|DigitalOcean|Docker|GCP|vSphere|OpenStack|Metal3|Packet}}

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig \
  apply -f https://docs.projectcalico.org/v3.18/manifests/calico.yaml
```

After a short while, our nodes should be running and in `Ready` state,
let's check the status using `kubectl get nodes`:

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig get nodes
```

{{#/tab }}
{{#tab Azure}}

Azure [does not currently support Calico networking](https://docs.projectcalico.org/reference/public-cloud/azure). As a workaround, it is recommended that Azure clusters use the Calico spec below that uses VXLAN.

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig \
  apply -f https://raw.githubusercontent.com/kubernetes-sigs/cluster-api-provider-azure/main/templates/addons/calico.yaml
```

After a short while, our nodes should be running and in `Ready` state,
let's check the status using `kubectl get nodes`:

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig get nodes
```

{{#/tab }}
{{#/tabs }}

### Clean Up

Delete workload cluster.
```bash
kubectl delete cluster capi-quickstart
```
<aside class="note warning">

IMPORTANT: In order to ensure a proper cleanup of your infrastructure you must always delete the cluster object. Deleting the entire cluster template with `kubectl delete -f capi-quickstart.yaml` might lead to pending resources to be cleaned up manually.
</aside>

Delete management cluster
```bash
kind delete cluster
```

## Next steps

See the [clusterctl] documentation for more detail about clusterctl supported actions.

<!-- links -->
[AWS provider prerequisites]: https://cluster-api-aws.sigs.k8s.io/topics/using-clusterawsadm-to-fulfill-prerequisites.html
[AWS provider releases]: https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases
[Azure Provider Prerequisites]: https://capz.sigs.k8s.io/topics/getting-started.html#prerequisites
[bootstrap cluster]: ../reference/glossary.md#bootstrap-cluster
[capa]: https://cluster-api-aws.sigs.k8s.io
[capv-upload-images]: https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/docs/getting_started.md#uploading-the-machine-images
[clusterawsadm]: https://cluster-api-aws.sigs.k8s.io/clusterawsadm/clusterawsadm.html
[clusterctl generate cluster]: ../clusterctl/commands/generate-cluster.md
[clusterctl get kubeconfig]: ../clusterctl/commands/get-kubeconfig.md
[clusterctl]: ../clusterctl/overview.md
[Docker]: https://www.docker.com/
[GCP provider]: https://github.com/kubernetes-sigs/cluster-api-provider-gcp
[infrastructure provider]: ../reference/glossary.md#infrastructure-provider
[kind]: https://kind.sigs.k8s.io/
[KubeadmControlPlane]: ../developer/architecture/controllers/control-plane.md
[kubectl]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[management cluster]: ../reference/glossary.md#management-cluster
[Metal3 getting started guide]: https://github.com/metal3-io/cluster-api-provider-metal3/blob/master/docs/getting-started.md
[Metal3 provider]: https://github.com/metal3-io/cluster-api-provider-metal3/
[Packet getting started guide]: https://github.com/kubernetes-sigs/cluster-api-provider-packet#using
[provider]:../reference/providers.md
[provider components]: ../reference/glossary.md#provider-components
[vSphere getting started guide]: https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/docs/getting_started.md
[workload cluster]: ../reference/glossary.md#workload-cluster
