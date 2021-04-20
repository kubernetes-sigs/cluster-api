# Quick Start

In this tutorial we'll cover the basics of how to use Cluster API to create one or more Kubernetes clusters.

## Installation

### Common Prerequisites

- Install and setup [kubectl] in your local environment
- Install [Kind] and [Docker]

### Install and/or configure a kubernetes cluster

Cluster API requires an existing Kubernetes cluster accessible via kubectl; during the installation process the
Kubernetes cluster will be transformed into a [management cluster] by installing the Cluster API [provider components], so it
is recommended to keep it separated from any application workload.

It is a common practice to create a temporary, local bootstrap cluster which is then used to provision
a target [management cluster] on the selected [infrastructure provider].

Choose one of the options below:

1. **Existing Management Cluster**

For production use-cases a "real" Kubernetes cluster should be used with appropriate backup and DR policies and procedures in place. The Kubernetes cluster must be at least v1.16+.

```bash
export KUBECONFIG=<...>
```

2. **Kind**

<aside class="note warning">

<h1>Warning</h1>

[kind] is not designed for production use.

**Minimum [kind] supported version**: v0.9.0

</aside>

[kind] can be used for creating a local Kubernetes cluster for development environments or for
the creation of a temporary [bootstrap cluster] used to provision a target [management cluster] on the selected infrastructure provider.

The installation procedure depends on the version of kind; if you are planning to user the docker infrastructure provider (CAPD),
please follow the additional instructions in the dedicated tab:

{{#tabs name:"install-kind" tabs:">=v0.9.x, Docker infrastructure provider - CAPD"}}
{{#tab >=v0.9.x}}

Create the kind cluster:
```bash
kind create cluster
```
Test to ensure the local kind cluster is ready:
```
kubectl cluster-info
```

{{#/tab }}
{{#tab Docker infrastructure provider - CAPD}}

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

If you are planning to use a CAPD version v0.3.15 or older, export the variable 
**KIND_EXPERIMENTAL_DOCKER_NETWORK=bridge** to let kind run in the **bridge** network, the same used
by older versions of the Docker infrastructure provider:

```bash
export KIND_EXPERIMENTAL_DOCKER_NETWORK=bridge
```

Then follow the instruction for your kind version using  `kind create cluster --config kind-cluster-with-extramounts.yaml` 
to create the management cluster using the above file.

{{#/tab }}
{{#/tabs }}

### Install clusterctl
The clusterctl CLI tool handles the lifecycle of a Cluster API management cluster.

{{#tabs name:"install-clusterctl" tabs:"linux,macOS"}}
{{#tab linux}}

#### Install clusterctl binary with curl on linux
Download the latest release; for example, to download version v0.3.0 on linux, type:
```
curl -L {{#releaselink gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-linux-amd64" version:"0.3.x"}} -o clusterctl
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

##### Install clusterctl binary with curl on macOS
Download the latest release; for example, to download version v0.3.0 on macOS, type:
```
curl -L {{#releaselink gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-darwin-amd64" version:"0.3.x"}} -o clusterctl
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

{{#tabs name:"tab-installation-infrastructure" tabs:"AWS,Azure,Docker,GCP,vSphere,OpenStack,Metal3,Packet"}}
{{#tab AWS}}

Download the latest binary of `clusterawsadm` from the [AWS provider releases] and make sure to place it in your path. You need at least version v0.5.5 for these instructions.
Instructions for older versions of clusterawsadm are available in [Github][legacy-clusterawsadm].

The clusterawsadm command line utility assists with identity and access management (IAM) for Cluster API Provider AWS.

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

# Azure cloud settings
# To use the default public cloud, otherwise set to AzureChinaCloud|AzureGermanCloud|AzureUSGovernmentCloud
export AZURE_ENVIRONMENT="AzurePublicCloud"

export AZURE_SUBSCRIPTION_ID_B64="$(echo -n "$AZURE_SUBSCRIPTION_ID" | base64 | tr -d '\n')"
export AZURE_TENANT_ID_B64="$(echo -n "$AZURE_TENANT_ID" | base64 | tr -d '\n')"
export AZURE_CLIENT_ID_B64="$(echo -n "$AZURE_CLIENT_ID" | base64 | tr -d '\n')"
export AZURE_CLIENT_SECRET_B64="$(echo -n "$AZURE_CLIENT_SECRET" | base64 | tr -d '\n')"

# Finally, initialize the management cluster
clusterctl init --infrastructure azure
```

{{#/tab }}
{{#tab Docker}}

<aside class="note warning">

<h1>Warning</h1>

The Docker provider is not designed for production use and is intended for development environments only.

</aside>

The docker provider does not require additional prerequisites.
You can run 
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
Installing cert-manager
Waiting for cert-manager to be available...
Installing Provider="cluster-api" Version="v0.3.0" TargetNamespace="capi-system"
Installing Provider="bootstrap-kubeadm" Version="v0.3.0" TargetNamespace="capi-kubeadm-bootstrap-system"
Installing Provider="control-plane-kubeadm" Version="v0.3.0" TargetNamespace="capi-kubeadm-control-plane-system"
Installing Provider="infrastructure-aws" Version="v0.5.0" TargetNamespace="capa-system"

Your management cluster has been initialized successfully!

You can now create your first workload cluster by running the following:

  clusterctl config cluster [name] --kubernetes-version [version] | kubectl apply -f -
```

<aside class="note">

<h1>Alternatives to environment variables</h1>

Throughout this quickstart guide, we've given instructions on setting parameters using environment variables. For most
environment variables in the rest of the guide, you can also set them in ~/.cluster-api/clusterctl.yaml

See [`clusterctl init`](../clusterctl/commands/init.md) for more details.

</aside>

### Create your first workload cluster

Once the management cluster is ready, you can create your first workload cluster.

#### Preparing the workload cluster configuration

The `clusterctl config cluster` command returns a YAML template for creating a [workload cluster].

<aside class="note">

<h1> Which provider will be used for my cluster? </h1>

The `clusterctl config cluster` command uses smart defaults in order to simplify the user experience; in this example,
it detects that there is only an `aws` infrastructure provider and so it uses that when creating the cluster.

</aside>

<aside class="note">

<h1> What topology will be used for my cluster? </h1>

The `clusterctl config cluster` command by default uses cluster templates which are provided by the infrastructure
providers. See the provider's documentation for more information.

See the `clusterctl config cluster` [command][clusterctl config cluster] documentation for
details about how to use alternative sources. for cluster templates.

</aside>

#### Required configuration for common providers

Depending on the infrastructure provider you are planning to use, some additional prerequisites should be satisfied
before configuring a cluster with Cluster API. Instructions are provided for common providers below.

Otherwise, you can look at the `clusterctl config cluster` [command][clusterctl config cluster] documentation for details about how to
discover the list of variables required by a cluster templates.

{{#tabs name:"tab-configuration-infrastructure" tabs:"AWS,Azure,Docker,GCP,vSphere,OpenStack,Metal3,Packet"}}
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
{{#tab Docker}}

<aside class="note warning">

<h1>Warning</h1>

The Docker provider is not designed for production use and is intended for development environments only.

</aside>

The docker provider does not require additional configurations for cluster templates.

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
clusterctl config cluster --infrastructure openstack --list-variables capi-quickstart
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

A full configuration reference can be found in [configuration.md](https://github.com/kubernetes-sigs/cluster-api-provider-openstack/blob/master/docs/configuration.md).

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
# You have to get out from Packet Portal if you don't have one already.
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

{{#tabs name:"tab-clusterctl-config-cluster" tabs:"Azure|AWS|GCP|vSphere|OpenStack|Metal3|Packet,Docker"}}
{{#tab Azure|AWS|GCP|vSphere|OpenStack|Metal3|Packet}}

```bash
clusterctl config cluster capi-quickstart \
  --kubernetes-version v1.18.16 \
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
clusterctl config cluster capi-quickstart --flavor development \
  --kubernetes-version v1.18.16 \
  --control-plane-machine-count=3 \
  --worker-machine-count=3 \
  > capi-quickstart.yaml
```

{{#/tab }}
{{#/tabs }}

This creates a YAML file named `capi-quickstart.yaml` with a predefined list of Cluster API objects; Cluster, Machines,
Machine Deployments, etc.

The file can be eventually modified using your editor of choice.

See [clusterctl config cluster] for more details.

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
kubectl get cluster --all-namespaces
```

You can also get an "at glance" view of the cluster and its resources by running:

```bash
clusterctl describe cluster capi-quickstart
```

To verify the first control plane is up:

```bash
kubectl get kubeadmcontrolplane --all-namespaces
```

You should see an output is similar to this:

```bash
NAME                            INITIALIZED   API SERVER AVAILABLE   VERSION    REPLICAS   READY   UPDATED   UNAVAILABLE
capi-quickstart-control-plane   true                                 v1.18.16   3                  3         3
```

<aside class="note warning">

<h1> Warning </h1>

The control planes won't be `Ready` until we install a CNI in the next step.

</aside>

After the first control plane node is up and running, we can retrieve the [workload cluster] Kubeconfig:

```bash
clusterctl get kubeconfig capi-quickstart > capi-quickstart.kubeconfig
```

<aside class="note warning">

<h1>Warning</h1>

The `clusterctl get kubeconfig` command is available on for clusterctl v0.3.9 or newer. See [clusterctl get kubeconfig] for more details. If you are running older
version you can use the following command: 

```bash
kubectl --namespace=default get secret capi-quickstart-kubeconfig \
   -o jsonpath={.data.value} | base64 --decode \
   > capi-quickstart.kubeconfig
```

If you are using docker on MacOS, you will need to do a couple of additional
steps to get the correct kubeconfig for a workload cluster created with the docker provider.
See [Additional Notes for the Docker Provider](../clusterctl/developers.md#additional-notes-for-the-docker-provider). 

</aside>

### Deploy a CNI solution

Calico is used here as an example.

{{#tabs name:"tab-deploy-cni" tabs:"AWS|Docker|GCP|vSphere|OpenStack|Metal3|Packet,Azure"}}
{{#tab AWS|Docker|GCP|vSphere|OpenStack|Metal3|Packet}}

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig \
  apply -f https://docs.projectcalico.org/v3.15/manifests/calico.yaml
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
  apply -f https://raw.githubusercontent.com/kubernetes-sigs/cluster-api-provider-azure/master/templates/addons/calico.yaml
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
[capv-upload-images]: https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/docs/getting_started.md#uploading-the-machine-images
[clusterctl config cluster]: ../clusterctl/commands/config-cluster.md
[clusterctl get kubeconfig]: ../clusterctl/commands/get-kubeconfig.md
[clusterctl]: ../clusterctl/overview.md
[Docker]: https://www.docker.com/
[docker-provider]: ../clusterctl/developers.md#additional-steps-for-the-docker-provider
[GCP provider]: https://github.com/kubernetes-sigs/cluster-api-provider-gcp
[infrastructure provider]: ../reference/glossary.md#infrastructure-provider
[kind]: https://kind.sigs.k8s.io/
[KubeadmControlPlane]: ../developer/architecture/controllers/control-plane.md
[kubectl]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[management cluster]: ../reference/glossary.md#management-cluster
[Metal3 provider]: https://github.com/metal3-io/cluster-api-provider-metal3/
[Metal3 getting started guide]: https://github.com/metal3-io/cluster-api-provider-metal3/blob/master/docs/getting-started.md
[Packet getting started guide]: https://github.com/kubernetes-sigs/cluster-api-provider-packet#using
[provider components]: ../reference/glossary.md#provider-components
[vSphere getting started guide]: https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/
[workload cluster]: ../reference/glossary.md#workload-cluster
[legacy-clusterawsadm]: https://github.com/kubernetes-sigs/cluster-api/blob/v0.3.6/docs/book/src/user/quick-start.md#initialization-for-common-providers
