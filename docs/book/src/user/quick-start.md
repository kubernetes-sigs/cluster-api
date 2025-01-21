# Quick Start

In this tutorial we'll cover the basics of how to use Cluster API to create one or more Kubernetes clusters.

<aside class="note warning">

<h1>Warning</h1>

If using a [provider] that does not support v1beta1 or v1alpha4 yet, please follow the [release 0.3](https://release-0-3.cluster-api.sigs.k8s.io/user/quick-start.html) or [release 0.4](https://release-0-4.cluster-api.sigs.k8s.io/user/quick-start.html) quickstart instructions instead.

</aside>

## Installation

There are two major quickstart paths:  Using clusterctl or the Cluster API Operator.

 This article describes a path that uses the `clusterctl` CLI tool to handle the lifecycle of a Cluster API [management cluster](https://cluster-api.sigs.k8s.io/reference/glossary#management-cluster).

The clusterctl command line interface is specifically designed for providing a simple “day 1 experience” and a quick start with Cluster API. It automates fetching the YAML files defining [provider components](https://cluster-api.sigs.k8s.io/reference/glossary#provider-components) and installing them.

Additionally it encodes a set of best practices in managing providers, that helps the user in avoiding mis-configurations or in managing day 2 operations such as upgrades.

The Cluster API Operator is a Kubernetes Operator built on top of clusterctl and designed to empower cluster administrators to handle the lifecycle of Cluster API providers within a management cluster using a declarative approach. It aims to improve user experience in deploying and managing Cluster API, making it easier to handle day-to-day tasks and automate workflows with GitOps. Visit the [CAPI Operator quickstart] if you want to experiment with this tool.

### Common Prerequisites

- Install and setup [kubectl] in your local environment
- Install [kind] and [Docker]
- Install [Helm]

### Install and/or configure a Kubernetes cluster

Cluster API requires an existing Kubernetes cluster accessible via kubectl. During the installation process the
Kubernetes cluster will be transformed into a [management cluster] by installing the Cluster API [provider components], so it
is recommended to keep it separated from any application workload.

It is a common practice to create a temporary, local bootstrap cluster which is then used to provision
a target [management cluster] on the selected [infrastructure provider].

**Choose one of the options below:**

1. **Existing Management Cluster**

   For production use-cases a "real" Kubernetes cluster should be used with appropriate backup and disaster recovery policies and procedures in place. The Kubernetes cluster must be at least v1.20.0.

   ```bash
   export KUBECONFIG=<...>
   ```
**OR**

2. **Kind**

   <aside class="note warning">

   <h1>Warning</h1>

   [kind] is not designed for production use.

   **Minimum [kind] supported version**: v0.25.0

   **Help with common issues can be found in the [Troubleshooting Guide](./troubleshooting.md).**

   Note for macOS users: you may need to [increase the memory available](https://docs.docker.com/docker-for-mac/#resources) for containers (recommend 6 GB for CAPD).

   Note for Linux users: you may need to [increase `ulimit` and `inotify` when using Docker (CAPD)](./troubleshooting.md#cluster-api-with-docker----too-many-open-files).

   </aside>

   [kind] can be used for creating a local Kubernetes cluster for development environments or for
   the creation of a temporary [bootstrap cluster] used to provision a target [management cluster] on the selected infrastructure provider.

   The installation procedure depends on the version of kind; if you are planning to use the Docker infrastructure provider,
   please follow the additional instructions in the dedicated tab:

   {{#tabs name:"install-kind" tabs:"Default,Docker,KubeVirt"}}
   {{#tab Default}}

   Create the kind cluster:
   ```bash
   kind create cluster
   ```
   Test to ensure the local kind cluster is ready:
   ```bash
   kubectl cluster-info
   ```

   {{#/tab }}
   {{#tab Docker}}

   Run the following command to create a kind config file for allowing the Docker provider to access Docker on the host:

   ```bash
   cat > kind-cluster-with-extramounts.yaml <<EOF
   kind: Cluster
   apiVersion: kind.x-k8s.io/v1alpha4
   networking:
     ipFamily: dual
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
   {{#tab KubeVirt}}

   #### Create the Kind Cluster
   [KubeVirt][KubeVirt] is a cloud native virtualization solution. The virtual machines we're going to create and use for
   the workload cluster's nodes, are actually running within pods in the management cluster. In order to communicate with
   the workload cluster's API server, we'll need to expose it. We are using Kind which is a limited environment. The
   easiest way to expose the workload cluster's API server (a pod within a node running in a VM that is itself running
   within a pod in the management cluster, that is running inside a Docker container), is to use a LoadBalancer service.

   To allow using a LoadBalancer service, we can't use the kind's default CNI (kindnet), but we'll need to install
   another CNI, like Calico. In order to do that, we'll need first to initiate the kind cluster with two modifications:
   1. Disable the default CNI
   2. Add the Docker credentials to the cluster, to avoid the Docker Hub pull rate limit of the calico images; read more
      about it in the [docker documentation](https://docs.docker.com/docker-hub/download-rate-limit/), and in the
      [kind documentation](https://kind.sigs.k8s.io/docs/user/private-registries/#mount-a-config-file-to-each-node).

   Create a configuration file for kind. Please notice the Docker config file path, and adjust it to your local setting:
   ```bash
   cat <<EOF > kind-config.yaml
   kind: Cluster
   apiVersion: kind.x-k8s.io/v1alpha4
   networking:
   # the default CNI will not be installed
     disableDefaultCNI: true
   nodes:
   - role: control-plane
     extraMounts:
      - containerPath: /var/lib/kubelet/config.json
        hostPath: <YOUR DOCKER CONFIG FILE PATH>
   EOF
   ```
   Now, create the kind cluster with the configuration file:
   ```bash
   kind create cluster --config=kind-config.yaml
   ```
   Test to ensure the local kind cluster is ready:
   ```bash
   kubectl cluster-info
   ```

   #### Install the Calico CNI
   Now we'll need to install a CNI. In this example, we're using calico, but other CNIs should work as well. Please see
   [calico installation guide](https://projectcalico.docs.tigera.io/getting-started/kubernetes/self-managed-onprem/onpremises#install-calico)
   for more details (use the "Manifest" tab). Below is an example of how to install calico version v3.24.4.

   Use the Calico manifest to create the required resources; e.g.:
   ```bash
   kubectl create -f  https://raw.githubusercontent.com/projectcalico/calico/v3.24.4/manifests/calico.yaml
   ```

   {{#/tab }}
   {{#/tabs }}

### Install clusterctl
The clusterctl CLI tool handles the lifecycle of a Cluster API management cluster.

{{#tabs name:"install-clusterctl" tabs:"Linux,macOS,homebrew,Windows"}}
{{#tab Linux}}

#### Install clusterctl binary with curl on Linux
If you are unsure you can determine your computers architecture by running `uname -a`

Download for AMD64:
```bash
curl -L {{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api" gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-linux-amd64" version:"1.9.x"}} -o clusterctl
```

Download for ARM64:
```bash
curl -L {{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api" gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-linux-arm64" version:"1.9.x"}} -o clusterctl
```

Download for PPC64LE:
```bash
curl -L {{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api" gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-linux-ppc64le" version:"1.9.x"}} -o clusterctl
```

Install clusterctl:
```bash
sudo install -o root -g root -m 0755 clusterctl /usr/local/bin/clusterctl
```
Test to ensure the version you installed is up-to-date:
```bash
clusterctl version
```

{{#/tab }}
{{#tab macOS}}

#### Install clusterctl binary with curl on macOS
If you are unsure you can determine your computers architecture by running `uname -a`

Download for AMD64:
```bash
curl -L {{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api" gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-darwin-amd64" version:"1.9.x"}} -o clusterctl
```

Download for M1 CPU ("Apple Silicon") / ARM64:
```bash
curl -L {{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api" gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-darwin-arm64" version:"1.9.x"}} -o clusterctl
```

Make the clusterctl binary executable.
```bash
chmod +x ./clusterctl
```
Move the binary in to your PATH.
```bash
sudo mv ./clusterctl /usr/local/bin/clusterctl
```
Test to ensure the version you installed is up-to-date:
```bash
clusterctl version
```
{{#/tab }}
{{#tab homebrew}}

#### Install clusterctl with homebrew on macOS and Linux

Install the latest release using homebrew:

```bash
brew install clusterctl
```

Test to ensure the version you installed is up-to-date:
```bash
clusterctl version
```

{{#/tab }}
{{#tab windows}}

#### Install clusterctl binary with curl on Windows using PowerShell
Go to the working directory where you want clusterctl downloaded.

Download the latest release; on Windows, type:
```powershell
curl.exe -L {{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api" gomodule:"sigs.k8s.io/cluster-api" asset:"clusterctl-windows-amd64.exe" version:"1.9.x"}} -o clusterctl.exe
```
Append or prepend the path of that directory to the `PATH` environment variable.

Test to ensure the version you installed is up-to-date:
```powershell
clusterctl.exe version
```

{{#/tab }}
{{#/tabs }}

### Initialize the management cluster

Now that we've got clusterctl installed and all the prerequisites in place, let's transform the Kubernetes cluster
into a management cluster by using `clusterctl init`.

The command accepts as input a list of providers to install; when executed for the first time, `clusterctl init`
automatically adds to the list the `cluster-api` core provider, and if unspecified, it also adds the `kubeadm` bootstrap
and `kubeadm` control-plane providers.

#### Enabling Feature Gates

Feature gates can be enabled by exporting environment variables before executing `clusterctl init`.
For example, the `ClusterTopology` feature, which is required to enable support for managed topologies and ClusterClass,
can be enabled via:
```bash
export CLUSTER_TOPOLOGY=true
```
Additional documentation about experimental features can be found in [Experimental Features].

#### Initialization for common providers

Depending on the infrastructure provider you are planning to use, some additional prerequisites should be satisfied
before getting started with Cluster API. See below for the expected settings for common providers.

{{#tabs name:"tab-installation-infrastructure" tabs:"Akamai (Linode),AWS,Azure,CloudStack,DigitalOcean,Docker,Equinix Metal,GCP,Harvester,Hetzner,Hivelocity,IBM Cloud,IONOS Cloud,K0smotron,KubeKey,KubeVirt,Metal3,Nutanix,OCI,OpenStack,Outscale,Proxmox,VCD,vcluster,Virtink,vSphere,Vultr"}}
{{#tab Akamai (Linode)}}

```bash
export LINODE_TOKEN=<your-access-token>

# Initialize the management cluster
clusterctl init --infrastructure linode-linode
```
{{#/tab }}
{{#tab AWS}}

Download the latest binary of `clusterawsadm` from the [AWS provider releases]. The [clusterawsadm] command line utility assists with identity and access management (IAM) for [Cluster API Provider AWS][capa].

{{#tabs name:"install-clusterawsadm" tabs:"Linux,macOS,homebrew,Windows"}}
{{#tab Linux}}

Download the latest release; on Linux, type:
```
curl -L {{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api-provider-aws" gomodule:"sigs.k8s.io/cluster-api-provider-aws" asset:"clusterawsadm-linux-amd64" version:">=2.0.0"}} -o clusterawsadm
```

Make it executable
```
chmod +x clusterawsadm
```

Move the binary to a directory present in your PATH
```
sudo mv clusterawsadm /usr/local/bin
```

Check version to confirm installation
```
clusterawsadm version
```

**Example Usage**
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

{{#/tab }}
{{#tab macOS}}

Download the latest release; on macOs, type:
```
curl -L {{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api-provider-aws" gomodule:"sigs.k8s.io/cluster-api-provider-aws" asset:"clusterawsadm-darwin-amd64" version:">=2.0.0"}} -o clusterawsadm
```

Or if your Mac has an M1 CPU (”Apple Silicon”):
```
curl -L {{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api-provider-aws" gomodule:"sigs.k8s.io/cluster-api-provider-aws" asset:"clusterawsadm-darwin-arm64" version:">=2.0.0"}} -o clusterawsadm
```

Make it executable
```
chmod +x clusterawsadm
```

Move the binary to a directory present in your PATH
```
sudo mv clusterawsadm /usr/local/bin
```

Check version to confirm installation
```
clusterawsadm version
```

**Example Usage**
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
{{#/tab }}
{{#tab homebrew}}

Install the latest release using homebrew:
```
brew install clusterawsadm
```

Check version to confirm installation
```
clusterawsadm version
```

**Example Usage**
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

{{#/tab }}
{{#tab Windows}}

Download the latest release; on Windows, type:
```
curl.exe -L {{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api-provider-aws" gomodule:"sigs.k8s.io/cluster-api-provider-aws" asset:"clusterawsadm-windows-amd64.exe" version:">=2.0.0"}} -o clusterawsadm.exe
```

Append or prepend the path of that directory to the `PATH` environment variable.
Check version to confirm installation
```
clusterawsadm.exe version
```

**Example Usage in Powershell**
```bash
$Env:AWS_REGION="us-east-1" # This is used to help encode your environment variables
$Env:AWS_ACCESS_KEY_ID="<your-access-key>"
$Env:AWS_SECRET_ACCESS_KEY="<your-secret-access-key>"
$Env:AWS_SESSION_TOKEN="<session-token>" # If you are using Multi-Factor Auth.

# The clusterawsadm utility takes the credentials that you set as environment
# variables and uses them to create a CloudFormation stack in your AWS account
# with the correct IAM resources.
clusterawsadm bootstrap iam create-cloudformation-stack

# Create the base64 encoded credentials using clusterawsadm.
# This command uses your environment variables and encodes
# them in a value to be stored in a Kubernetes Secret.
$Env:AWS_B64ENCODED_CREDENTIALS=$(clusterawsadm bootstrap credentials encode-as-profile)

# Finally, initialize the management cluster
clusterctl init --infrastructure aws
```
{{#/tab }}
{{#/tabs }}

See the [AWS provider prerequisites] document for more details.

{{#/tab }}
{{#tab Azure}}

For more information about authorization, AAD, or requirements for Azure, visit the [Azure provider prerequisites] document.

```bash
export AZURE_SUBSCRIPTION_ID="<SubscriptionId>"

# Create an Azure Service Principal and paste the output here
export AZURE_TENANT_ID="<Tenant>"
export AZURE_CLIENT_ID="<AppId>"
export AZURE_CLIENT_ID_USER_ASSIGNED_IDENTITY=$AZURE_CLIENT_ID # for compatibility with CAPZ v1.16 templates
export AZURE_CLIENT_SECRET="<Password>"

# Settings needed for AzureClusterIdentity used by the AzureCluster
export AZURE_CLUSTER_IDENTITY_SECRET_NAME="cluster-identity-secret"
export CLUSTER_IDENTITY_NAME="cluster-identity"
export AZURE_CLUSTER_IDENTITY_SECRET_NAMESPACE="default"

# Create a secret to include the password of the Service Principal identity created in Azure
# This secret will be referenced by the AzureClusterIdentity used by the AzureCluster
kubectl create secret generic "${AZURE_CLUSTER_IDENTITY_SECRET_NAME}" --from-literal=clientSecret="${AZURE_CLIENT_SECRET}" --namespace "${AZURE_CLUSTER_IDENTITY_SECRET_NAMESPACE}"

# Finally, initialize the management cluster
clusterctl init --infrastructure azure
```

{{#/tab }}
{{#tab CloudStack}}

Create a file named cloud-config in the repo's root directory, substituting in your own environment's values
```bash
[Global]
api-url = <cloudstackApiUrl>
api-key = <cloudstackApiKey>
secret-key = <cloudstackSecretKey>
```

Create the base64 encoded credentials by catting your credentials file.
This command uses your environment variables and encodes
them in a value to be stored in a Kubernetes Secret.

```bash
export CLOUDSTACK_B64ENCODED_SECRET=`cat cloud-config | base64 | tr -d '\n'`
```

Finally, initialize the management cluster
```bash
clusterctl init --infrastructure cloudstack
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

The Docker provider requires the `ClusterTopology` and `MachinePool` features to deploy ClusterClass-based clusters.
We are only supporting ClusterClass-based cluster-templates in this quickstart as ClusterClass makes it possible to
adapt configuration based on Kubernetes version. This is required to install Kubernetes clusters < v1.24 and
for the upgrade from v1.23 to v1.24 as we have to use different cgroupDrivers depending on Kubernetes version.

```bash
# Enable the experimental Cluster topology feature.
export CLUSTER_TOPOLOGY=true

# Initialize the management cluster
clusterctl init --infrastructure docker
```

{{#/tab }}
{{#tab Equinix Metal}}

In order to initialize the Equinix Metal Provider (formerly Packet) you have to expose the environment
variable `PACKET_API_KEY`. This variable is used to authorize the infrastructure
provider manager against the Equinix Metal API. You can retrieve your token directly
from the Equinix Metal Console.

```bash
export PACKET_API_KEY="34ts3g4s5g45gd45dhdh"

clusterctl init --infrastructure packet
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
{{#tab Harvester}}

```bash
clusterctl init --infrastructure harvester-harvester
```

For more information, please visit the [Harvester project][Harvester provider].
{{#/tab }}
{{#tab Hetzner}}

Please visit the [Hetzner project][Hetzner provider].

{{#/tab }}
{{#tab Hivelocity}}

Please visit the [Hivelocity project][Hivelocity provider].

{{#/tab }}
{{#tab IBM Cloud}}

In order to initialize the IBM Cloud Provider you have to expose the environment
variable `IBMCLOUD_API_KEY`. This variable is used to authorize the infrastructure
provider manager against the IBM Cloud API. To create one from the UI, refer [here](https://cloud.ibm.com/docs/account?topic=account-userapikey&interface=ui#create_user_key).

```bash
export IBMCLOUD_API_KEY=<you_api_key>

# Finally, initialize the management cluster
clusterctl init --infrastructure ibmcloud
```

{{#/tab }}
{{#tab IONOS Cloud}}

The IONOS Cloud credentials are configured in the `IONOSCloudCluster`.
Therefore, there is no need to specify them during the provider initialization.

```bash
clusterctl init --infrastructure ionoscloud-ionoscloud
```

For more information, please visit the [IONOS Cloud project][ionoscloud provider].

{{#/tab }}
{{#tab K0smotron}}

```bash
# Initialize the management cluster
clusterctl init --infrastructure k0sproject-k0smotron
```

{{#/tab }}
{{#tab KubeKey}}

```bash
# Initialize the management cluster
clusterctl init --infrastructure kubekey
```

{{#/tab }}
{{#tab KubeVirt}}

Please visit the [KubeVirt project][KubeVirt provider] for more information.

As described above, we want to use a LoadBalancer service in order to expose the workload cluster's API server. In the
example below, we will use [MetalLB](https://metallb.universe.tf/) solution to implement load balancing to our kind
cluster. Other solution should work as well.

#### Install MetalLB for load balancing
Install MetalLB, as described [here](https://metallb.universe.tf/installation/#installation-by-manifest); for example:
```bash
METALLB_VER=$(curl "https://api.github.com/repos/metallb/metallb/releases/latest" | jq -r ".tag_name")
kubectl apply -f "https://raw.githubusercontent.com/metallb/metallb/${METALLB_VER}/config/manifests/metallb-native.yaml"
kubectl wait pods -n metallb-system -l app=metallb,component=controller --for=condition=Ready --timeout=10m
kubectl wait pods -n metallb-system -l app=metallb,component=speaker --for=condition=Ready --timeout=2m
```

Now, we'll create the `IPAddressPool` and the `L2Advertisement` custom resources. The script below creates the CRs with
the right addresses, that match to the kind cluster addresses:
```bash
GW_IP=$(docker network inspect -f '{{range .IPAM.Config}}{{.Gateway}}{{end}}' kind)
NET_IP=$(echo ${GW_IP} | sed -E 's|^([0-9]+\.[0-9]+)\..*$|\1|g')
cat <<EOF | sed -E "s|172.19|${NET_IP}|g" | kubectl apply -f -
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: capi-ip-pool
  namespace: metallb-system
spec:
  addresses:
  - 172.19.255.200-172.19.255.250
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: empty
  namespace: metallb-system
EOF
```

#### Install KubeVirt on the kind cluster
```bash
# get KubeVirt version
KV_VER=$(curl "https://api.github.com/repos/kubevirt/kubevirt/releases/latest" | jq -r ".tag_name")
# deploy required CRDs
kubectl apply -f "https://github.com/kubevirt/kubevirt/releases/download/${KV_VER}/kubevirt-operator.yaml"
# deploy the KubeVirt custom resource
kubectl apply -f "https://github.com/kubevirt/kubevirt/releases/download/${KV_VER}/kubevirt-cr.yaml"
kubectl wait -n kubevirt kv kubevirt --for=condition=Available --timeout=10m
```

#### Initialize the management cluster with the KubeVirt Provider
```bash
clusterctl init --infrastructure kubevirt
```

{{#/tab }}
{{#tab Metal3}}

Please visit the [Metal3 project][Metal3 provider].

{{#/tab }}
{{#tab Nutanix}}

Please follow the Cluster API Provider for [Nutanix Getting Started Guide](https://opendocs.nutanix.com/capx/latest/getting_started/)

{{#/tab }}
{{#tab OCI}}

Please follow the Cluster API Provider for [Oracle Cloud Infrastructure (OCI) Getting Started Guide][oci-provider]

{{#/tab }}
{{#tab OpenStack}}

Cluster API Provider OpenStack depends on [openstack-resource-controller] since v0.12.

```bash
# Install ORC (needed for CAPO >=v0.12)
kubectl apply -f https://github.com/k-orc/openstack-resource-controller/releases/latest/download/install.yaml
# Initialize the management cluster
clusterctl init --infrastructure openstack
```

{{#/tab }}

{{#tab Outscale}}

```bash
export OSC_SECRET_KEY=<your-secret-key>
export OSC_ACCESS_KEY=<your-access-key>
export OSC_REGION=<you-region>
# Create namespace
kubectl create namespace cluster-api-provider-outscale-system
# Create secret
kubectl create secret generic cluster-api-provider-outscale --from-literal=access_key=${OSC_ACCESS_KEY} --from-literal=secret_key=${OSC_SECRET_KEY} --from-literal=region=${OSC_REGION}  -n cluster-api-provider-outscale-system
# Initialize the management cluster
clusterctl init --infrastructure outscale
```

{{#/tab }}

{{#tab Proxmox}}

The Proxmox credentials are optional, when creating a cluster they can be set in the `ProxmoxCluster` resource,
if you do not set them here.

```bash
# The host for the Proxmox cluster
export PROXMOX_URL="https://pve.example:8006"
# The Proxmox token ID to access the remote Proxmox endpoint
export PROXMOX_TOKEN='root@pam!capi'
# The secret associated with the token ID
# You may want to set this in `$XDG_CONFIG_HOME/cluster-api/clusterctl.yaml` so your password is not in
# bash history
export PROXMOX_SECRET="1234-1234-1234-1234"


# Finally, initialize the management cluster
clusterctl init --infrastructure proxmox --ipam in-cluster
```

For more information about the CAPI provider for Proxmox, see the [Proxmox
project][Proxmox getting started guide].

{{#/tab }}

{{#tab VCD}}

Please follow the Cluster API Provider for [Cloud Director Getting Started Guide](https://github.com/vmware/cluster-api-provider-cloud-director/blob/main/README.md)

```bash
# Initialize the management cluster
clusterctl init --infrastructure vcd
```

{{#/tab }}
{{#tab vcluster}}

```bash
clusterctl init --infrastructure vcluster
```

Please follow the Cluster API Provider for [vcluster Quick Start Guide](https://github.com/loft-sh/cluster-api-provider-vcluster/blob/main/docs/quick-start.md)

{{#/tab }}
{{#tab Virtink}}

```bash
# Initialize the management cluster
clusterctl init --infrastructure virtink
```

{{#/tab }}
{{#tab vSphere}}

```bash
# The username used to access the remote vSphere endpoint
export VSPHERE_USERNAME="vi-admin@vsphere.local"
# The password used to access the remote vSphere endpoint
# You may want to set this in `$XDG_CONFIG_HOME/cluster-api/clusterctl.yaml` so your password is not in
# bash history
export VSPHERE_PASSWORD="admin!23"

# Finally, initialize the management cluster
clusterctl init --infrastructure vsphere
```

For more information about prerequisites, credentials management, or permissions for vSphere, see the [vSphere
project][vSphere getting started guide].

{{#/tab }}
{{#tab Vultr}}

```bash
export VULTR_API_KEY=<your_api_key>

# initialize the management cluster
clusterctl init --infrastructure vultr
```
{{#/tab }}
{{#/tabs }}

The output of `clusterctl init` is similar to this:

```bash
Fetching providers
Installing cert-manager Version="v1.11.0"
Waiting for cert-manager to be available...
Installing Provider="cluster-api" Version="v1.0.0" TargetNamespace="capi-system"
Installing Provider="bootstrap-kubeadm" Version="v1.0.0" TargetNamespace="capi-kubeadm-bootstrap-system"
Installing Provider="control-plane-kubeadm" Version="v1.0.0" TargetNamespace="capi-kubeadm-control-plane-system"
Installing Provider="infrastructure-docker" Version="v1.0.0" TargetNamespace="capd-system"

Your management cluster has been initialized successfully!

You can now create your first workload cluster by running the following:

  clusterctl generate cluster [name] --kubernetes-version [version] | kubectl apply -f -
```

<aside class="note">

<h1>Alternatives to environment variables</h1>

Throughout this quickstart guide we've given instructions on setting parameters using environment variables. For most
environment variables in the rest of the guide, you can also set them in `$XDG_CONFIG_HOME/cluster-api/clusterctl.yaml`

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

{{#tabs name:"tab-configuration-infrastructure" tabs:"Akamai (Linode),AWS,Azure,CloudStack,DigitalOcean,Docker,Equinix Metal,GCP,Harvester,IBM Cloud,IONOS Cloud,K0smotron,KubeKey,KubeVirt,Metal3,Nutanix,OpenStack,Outscale,Proxmox,Tinkerbell,VCD,vcluster,Virtink,vSphere,Vultr"}}
{{#tab Akamai (Linode)}}

```bash
export LINODE_REGION=us-ord
export LINODE_TOKEN=<your linode PAT>
export LINODE_CONTROL_PLANE_MACHINE_TYPE=g6-standard-2
export LINODE_MACHINE_TYPE=g6-standard-2
```

See the [Akamai (Linode) provider] for more information.

{{#/tab }}
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

# [Optional] Select resource group. The default value is ${CLUSTER_NAME}.
export AZURE_RESOURCE_GROUP="<ResourceGroupName>"
```

{{#/tab }}
{{#tab CloudStack}}

A Cluster API compatible image must be available in your CloudStack installation. For instructions on how to build a compatible image
see [image-builder (CloudStack)](https://image-builder.sigs.k8s.io/capi/providers/cloudstack.html)

Prebuilt images can be found [here](http://packages.shapeblue.com/cluster-api-provider-cloudstack/images/)

To see all required CloudStack environment variables execute:
```bash
clusterctl generate cluster --infrastructure cloudstack --list-variables capi-quickstart
```

Apart from the script, the following CloudStack environment variables are required.
```bash
# Set this to the name of the zone in which to deploy the cluster
export CLOUDSTACK_ZONE_NAME=<zone name>
# The name of the network on which the VMs will reside
export CLOUDSTACK_NETWORK_NAME=<network name>
# The endpoint of the workload cluster
export CLUSTER_ENDPOINT_IP=<cluster endpoint address>
export CLUSTER_ENDPOINT_PORT=<cluster endpoint port>
# The service offering of the control plane nodes
export CLOUDSTACK_CONTROL_PLANE_MACHINE_OFFERING=<control plane service offering name>
# The service offering of the worker nodes
export CLOUDSTACK_WORKER_MACHINE_OFFERING=<worker node service offering name>
# The capi compatible template to use
export CLOUDSTACK_TEMPLATE_NAME=<template name>
# The ssh key to use to log into the nodes
export CLOUDSTACK_SSH_KEY_NAME=<ssh key name>

```

A full configuration reference can be found in [configuration.md](https://github.com/kubernetes-sigs/cluster-api-provider-cloudstack/blob/master/docs/book/src/clustercloudstack/configuration.md).

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

It is also possible but **not recommended** to disable the per-default enabled [Pod Security Standard](../security/pod-security-standards.md):
```bash
export POD_SECURITY_STANDARD_ENABLED="false"
```

{{#/tab }}
{{#tab Equinix Metal}}

There are several required variables you need to set to create a cluster. There
are also a few optional tunables if you'd like to change the OS or CIDRs used.

```bash
# Required (made up examples shown)
# The project where your cluster will be placed to.
# You have to get one from the Equinix Metal Console if you don't have one already.
export PROJECT_ID="2b59569f-10d1-49a6-a000-c2fb95a959a1"
# This can help to take advantage of automated, interconnected bare metal across our global metros.
export METRO="da"
# What plan to use for your control plane nodes
export CONTROLPLANE_NODE_TYPE="m3.small.x86"
# What plan to use for your worker nodes
export WORKER_NODE_TYPE="m3.small.x86"
# The ssh key you would like to have access to the nodes
export SSH_KEY="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDvMgVEubPLztrvVKgNPnRe9sZSjAqaYj9nmCkgr4PdK username@computer"
export CLUSTER_NAME="my-cluster"

# Optional (defaults shown)
export NODE_OS="ubuntu_18_04"
export POD_CIDR="192.168.0.0/16"
export SERVICE_CIDR="172.26.0.0/16"
# Only relevant if using the kube-vip flavor
export KUBE_VIP_VERSION="v0.5.0"
```

{{#/tab }}
{{#tab GCP}}


```bash
# Name of the GCP datacenter location. Change this value to your desired location
export GCP_REGION="<GCP_REGION>"
export GCP_PROJECT="<GCP_PROJECT>"
# Make sure to use same Kubernetes version here as building the GCE image
export KUBERNETES_VERSION=1.23.3
# This is the image you built. See https://github.com/kubernetes-sigs/image-builder
export IMAGE_ID=projects/$GCP_PROJECT/global/images/<built image>
export GCP_CONTROL_PLANE_MACHINE_TYPE=n1-standard-2
export GCP_NODE_MACHINE_TYPE=n1-standard-2
export GCP_NETWORK_NAME=<GCP_NETWORK_NAME or default>
export CLUSTER_NAME="<CLUSTER_NAME>"
```

See the [GCP provider] for more information.

{{#/tab }}
{{#tab Harvester}}


```bash
# Cloud Provider credentials, which are a Kubeconfig generated using this process: https://docs.harvesterhci.io/v1.3/rancher/cloud-provider/#deploying-to-the-rke2-custom-cluster-experimental
# Since v0.1.5, this can be left "", because the controller can update it automatically
export CLOUD_CONFIG_KUBECONFIG_B64=""
# Name of the CAPI Cluster
export CLUSTER_NAME="<CLUSTER_NAME>"
# Number of Control Plane machines
export CONTROL_PLANE_MACHINE_COUNT=3
# URL to access the Harvester Cluster, this will be overriden by the controller
export HARVESTER_ENDPOINT=""
# Base64-Encoded Kubeconfig to access Harvester, which can be downloaded from Harvester's UI or from a Harvester Manager Node.
export HARVESTER_KUBECONFIG_B64="<HARVESTER_KUBECONFIG_ENCODED_IN_BASE64>"
# Namespace for all resources in the Management Cluster
export NAMESPACE="test"
# Pod CIDR for the Workload Cluster, it should have the format: 192.168.0.0/16
export POD_CIDR="10.42.0.0/16"
# Service CIDR for the Workload Cluster, it should have the format : 192.168.0.0/16 and be different from POD_CIDR
export SERVICE_CIDR="10.43.0.0/16"
# Reference to SSH Keypair in Harvester. It should follow the format <NAMESPACE>/<NAME>
export SSH_KEYPAIR="default/ssk-key-pair"
# Namespace in Harvester where the VMs will be created.
export TARGET_HARVESTER_NAMESPACE="default"
# Disk Size to be used by the VMs
export VM_DISK_SIZE="50Gi"
# Reference to OS Image in Harvester which will be used for creating VMs, It must follow the format <NAMESPACE>/<NAME>
export VM_IMAGE_NAME="default/jammy-server"
# Reference to VM Network in Harvester. It must follow the format <NAMESPACE>/<NAME>
export VM_NETWORK="default/untagged"
# Linux Username for the VMs
export VM_SSH_USER="ubuntu"
# Number of Worker nodes in the target Workload cluster
export WORKER_MACHINE_COUNT=2
```

See the [Harvester provider] for more information.

{{#/tab }}
{{#tab IBM Cloud}}

```bash
# Required environment variables for VPC
# VPC region
export IBMVPC_REGION=us-south
# VPC zone within the region
export IBMVPC_ZONE=us-south-1
# ID of the resource group in which the VPC will be created
export IBMVPC_RESOURCEGROUP=<your-resource-group-id>
# Name of the VPC
export IBMVPC_NAME=ibm-vpc-0
export IBMVPC_IMAGE_ID=<you-image-id>
# Profile for the virtual server instances
export IBMVPC_PROFILE=bx2-4x16
export IBMVPC_SSHKEY_ID=<your-sshkey-id>

# Required environment variables for PowerVS
export IBMPOWERVS_SSHKEY_NAME=<your-ssh-key>
# Internal and external IP of the network
export IBMPOWERVS_VIP=<internal-ip>
export IBMPOWERVS_VIP_EXTERNAL=<external-ip>
export IBMPOWERVS_VIP_CIDR=29
export IBMPOWERVS_IMAGE_NAME=<your-capi-image-name>
# ID of the PowerVS service instance
export IBMPOWERVS_SERVICE_INSTANCE_ID=<service-instance-id>
export IBMPOWERVS_NETWORK_NAME=<your-capi-network-name>
```

Please visit the [IBM Cloud provider] for more information.

{{#/tab }}
{{#tab IONOS Cloud}}

A ClusterAPI compatible image must be available in your IONOS Cloud contract.
For instructions on how to build a compatible Image, see [our docs](https://github.com/ionos-cloud/cluster-api-provider-ionoscloud/blob/main/docs/custom-image.md).

```bash
# The token which is used to authenticate against the IONOS Cloud API
export IONOS_TOKEN=<your-token>
# The datacenter ID where the cluster will be deployed
export IONOSCLOUD_DATACENTER_ID="<your-datacenter-id>"
# The IP of the control plane endpoint
export CONTROL_PLANE_ENDPOINT_IP=10.10.10.4
# The location of the data center where the cluster will be deployed
export CONTROL_PLANE_ENDPOINT_LOCATION=de/txl
# The image ID of the custom image that will be used for the VMs
export IONOSCLOUD_MACHINE_IMAGE_ID="<your-image-id>"
# The SSH key that will be used to access the VMs
export IONOSCLOUD_MACHINE_SSH_KEYS="<your-ssh-key>"
```

For more configuration options check our list of [available variables](https://github.com/ionos-cloud/cluster-api-provider-ionoscloud/blob/main/docs/quickstart.md#environment-variables)

{{#/tab }}
{{#tab K0smotron}}

Please visit the [K0smotron provider] for more information.

{{#/tab }}
{{#tab KubeKey}}

```bash
# Required environment variables
# The KKZONE is used to specify where to download the binaries. (e.g. "", "cn")
export KKZONE=""
# The ssh name of the all instance Linux user. (e.g. root, ubuntu)
export USER_NAME=<your-linux-user>
# The ssh password of the all instance Linux user.
export PASSWORD=<your-linux-user-password>
# The ssh IP address of the all instance. (e.g. "[{address: 192.168.100.3}, {address: 192.168.100.4}]")
export INSTANCES=<your-linux-ip-address>
# The cluster control plane VIP. (e.g. "192.168.100.100")
export CONTROL_PLANE_ENDPOINT_IP=<your-control-plane-virtual-ip>
```

Please visit the [KubeKey provider] for more information.

{{#/tab }}
{{#tab KubeVirt}}

```bash
export CAPK_GUEST_K8S_VERSION="v1.23.10"
export CRI_PATH="/var/run/containerd/containerd.sock"
export NODE_VM_IMAGE_TEMPLATE="quay.io/capk/ubuntu-2004-container-disk:${CAPK_GUEST_K8S_VERSION}"
```
Please visit the [KubeVirt project][KubeVirt provider] for more information.

{{#/tab }}
{{#tab Metal3}}

**Note**: If you are running CAPM3 release prior to v0.5.0, make sure to export the following
environment variables. However, you don't need them to be exported if you use
CAPM3 release v0.5.0 or higher.

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
{{#tab Nutanix}}

A ClusterAPI compatible image must be available in your Nutanix image library. For instructions on how to build a compatible image
see [image-builder](https://image-builder.sigs.k8s.io/capi/capi.html).

To see all required Nutanix environment variables execute:
```bash
clusterctl generate cluster --infrastructure nutanix --list-variables capi-quickstart
```

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
# The external network
export OPENSTACK_EXTERNAL_NETWORK_ID=<external network ID>
```

A full configuration reference can be found in [configuration.md](https://github.com/kubernetes-sigs/cluster-api-provider-openstack/blob/master/docs/book/src/clusteropenstack/configuration.md).

{{#/tab }}
{{#tab Outscale}}

A ClusterAPI compatible image must be available in your Outscale account. For instructions on how to build a compatible image
see [image-builder](https://image-builder.sigs.k8s.io/capi/capi.html).

```bash
# The outscale root disk iops
export OSC_IOPS="<IOPS>"
# The outscale root disk size
export OSC_VOLUME_SIZE="<VOLUME_SIZE>"
# The outscale root disk volumeType
export OSC_VOLUME_TYPE="<VOLUME_TYPE>"
# The outscale key pair
export OSC_KEYPAIR_NAME="<KEYPAIR_NAME>"
# The outscale subregion name
export OSC_SUBREGION_NAME="<SUBREGION_NAME>"
# The outscale vm type
export OSC_VM_TYPE="<VM_TYPE>"
# The outscale image name
export OSC_IMAGE_NAME="<IMAGE_NAME>"
```

{{#/tab }}
{{#tab Proxmox}}

A ClusterAPI compatible image must be available in your Proxmox cluster. For instructions on how to build a compatible VM template
see [image-builder](https://image-builder.sigs.k8s.io/capi/capi.html).

```bash
# The node that hosts the VM template to be used to provision VMs
export PROXMOX_SOURCENODE="pve"
# The template VM ID used for cloning VMs
export TEMPLATE_VMID=100
# The ssh authorized keys used to ssh to the machines.
export VM_SSH_KEYS="ssh-ed25519 ..., ssh-ed25519 ..."
# The IP address used for the control plane endpoint
export CONTROL_PLANE_ENDPOINT_IP=10.10.10.4
# The IP ranges for Cluster nodes
export NODE_IP_RANGES="[10.10.10.5-10.10.10.50, 10.10.10.55-10.10.10.70]"
# The gateway for the machines network-config.
export GATEWAY="10.10.10.1"
# Subnet Mask in CIDR notation for your node IP ranges
export IP_PREFIX=24
# The Proxmox network device for VMs
export BRIDGE="vmbr1"
# The dns nameservers for the machines network-config.
export DNS_SERVERS="[8.8.8.8,8.8.4.4]"
# The Proxmox nodes used for VM deployments
export ALLOWED_NODES="[pve1,pve2,pve3]"
```

For more information about prerequisites and advanced setups for Proxmox, see the [Proxmox getting started guide].

{{#/tab }}
{{#tab Tinkerbell}}

```bash
export TINKERBELL_IP=<hegel ip>
```
For more information please visit [Tinkerbell getting started guide].

{{#/tab }}
{{#tab VCD}}

A ClusterAPI compatible image must be available in your VCD catalog. For instructions on how to build and upload a compatible image
see [CAPVCD](https://github.com/vmware/cluster-api-provider-cloud-director)

To see all required VCD environment variables execute:
```bash
clusterctl generate cluster --infrastructure vcd --list-variables capi-quickstart
```


{{#/tab }}
{{#tab vcluster}}

```bash
export CLUSTER_NAME=kind
export CLUSTER_NAMESPACE=vcluster
export KUBERNETES_VERSION=1.23.4
export HELM_VALUES="service:\n  type: NodePort"
```

Please see the [vcluster installation instructions](https://github.com/loft-sh/cluster-api-provider-vcluster#installation-instructions) for more details.

{{#/tab }}
{{#tab Virtink}}

To see all required Virtink environment variables execute:
```bash
clusterctl generate cluster --infrastructure virtink --list-variables capi-quickstart
```

See the [Virtink provider](https://github.com/smartxworks/cluster-api-provider-virtink) document for more details.

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
# The public ssh authorized key on all machines
export VSPHERE_SSH_AUTHORIZED_KEY="ssh-rsa AAAAB3N..."
# The certificate thumbprint for the vCenter server
export VSPHERE_TLS_THUMBPRINT="97:48:03:8D:78:A9..."
# The storage policy to be used (optional). Set to "" if not required
export VSPHERE_STORAGE_POLICY="policy-one"
# The IP address used for the control plane endpoint
export CONTROL_PLANE_ENDPOINT_IP="1.2.3.4"
```

For more information about prerequisites, credentials management, or permissions for vSphere, see the [vSphere getting started guide].

{{#/tab }}
{{#tab Vultr}}

A Cluster API compatible image must be available in your Vultr account. For instructions on how to build a compatible image see image-builder for [Vultr](https://github.com/vultr/cluster-api-provider-vultr/blob/main/docs/getting-started.md)

```bash
export CLUSTER_NAME=<clustername>
export KUBERNETES_VERSION=v1.28.9
export CONTROL_PLANE_MACHINE_COUNT=1
export CONTROL_PLANE_PLANID=<plan_id>
export WORKER_MACHINE_COUNT=1
export WORKER_PLANID=<plan_id>
export MACHINE_IMAGE=<snapshot_id>
export REGION=<region>
export PLANID=<plan_id>
export VPCID=<vpc_id>
export SSHKEY_ID=<sshKey_id>
```

{{#/tab }}
{{#/tabs }}

#### Generating the cluster configuration

For the purpose of this tutorial, we'll name our cluster capi-quickstart.

{{#tabs name:"tab-clusterctl-config-cluster" tabs:"Docker, vcluster, KubeVirt, Azure, Other providers..."}}
{{#tab Docker}}

<aside class="note warning">

<h1>Warning</h1>

The Docker provider is not designed for production use and is intended for development environments only.

</aside>

```bash
clusterctl generate cluster capi-quickstart --flavor development \
  --kubernetes-version v1.32.0 \
  --control-plane-machine-count=3 \
  --worker-machine-count=3 \
  > capi-quickstart.yaml
```

{{#/tab }}
{{#tab vcluster}}

```bash
export CLUSTER_NAME=kind
export CLUSTER_NAMESPACE=vcluster
export KUBERNETES_VERSION=1.31.2
export HELM_VALUES="service:\n  type: NodePort"

kubectl create namespace ${CLUSTER_NAMESPACE}
clusterctl generate cluster ${CLUSTER_NAME} \
    --infrastructure vcluster \
    --kubernetes-version ${KUBERNETES_VERSION} \
    --target-namespace ${CLUSTER_NAMESPACE} | kubectl apply -f -
```

{{#/tab }}
{{#tab KubeVirt}}

As we described above, in this tutorial, we will use a LoadBalancer service in order to expose the API server of the
workload cluster, so we want to use the load balancer (lb) template (rather than the default one). We'll use the
clusterctl's `--flavor` flag for that:
```bash
clusterctl generate cluster capi-quickstart \
  --infrastructure="kubevirt" \
  --flavor lb \
  --kubernetes-version ${CAPK_GUEST_K8S_VERSION} \
  --control-plane-machine-count=1 \
  --worker-machine-count=1 \
  > capi-quickstart.yaml
```

{{#/tab }}
{{#tab Azure}}

```bash
clusterctl generate cluster capi-quickstart \
  --infrastructure azure \
  --kubernetes-version v1.32.0 \
  --control-plane-machine-count=3 \
  --worker-machine-count=3 \
  > capi-quickstart.yaml

# Cluster templates authenticate with Workload Identity by default. Modify the AzureClusterIdentity for ServicePrincipal authentication.
# See https://capz.sigs.k8s.io/topics/identities for more details.
yq -i "with(. | select(.kind == \"AzureClusterIdentity\"); .spec.type |= \"ServicePrincipal\" | .spec.clientSecret.name |= \"${AZURE_CLUSTER_IDENTITY_SECRET_NAME}\" | .spec.clientSecret.namespace |= \"${AZURE_CLUSTER_IDENTITY_SECRET_NAMESPACE}\")" capi-quickstart.yaml
```

{{#/tab }}
{{#tab Other providers...}}

```bash
clusterctl generate cluster capi-quickstart \
  --kubernetes-version v1.32.0 \
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
dockercluster.infrastructure.cluster.x-k8s.io/capi-quickstart created
kubeadmcontrolplane.controlplane.cluster.x-k8s.io/capi-quickstart-control-plane created
dockermachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-control-plane created
machinedeployment.cluster.x-k8s.io/capi-quickstart-md-0 created
dockermachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-md-0 created
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

and see an output similar to this:

```bash
NAME              PHASE         AGE   VERSION
capi-quickstart   Provisioned   8s    v1.32.0
```

To verify the first control plane is up:

```bash
kubectl get kubeadmcontrolplane
```

You should see an output is similar to this:

```bash
NAME                    CLUSTER           INITIALIZED   API SERVER AVAILABLE   REPLICAS   READY   UPDATED   UNAVAILABLE   AGE    VERSION
capi-quickstart-g2trk   capi-quickstart   true                                 3                  3         3             4m7s   v1.32.0
```

<aside class="note warning">

<h1> Warning </h1>

The control plane won't be `Ready` until we install a CNI in the next step.

</aside>

After the first control plane node is up and running, we can retrieve the [workload cluster] Kubeconfig.

{{#tabs name:"tab-get-kubeconfig" tabs:"Default,Docker"}}

{{#/tab }}
{{#tab Default}}

```bash
clusterctl get kubeconfig capi-quickstart > capi-quickstart.kubeconfig
```

{{#/tab }}

{{#tab Docker}}
For Docker Desktop on macOS, Linux or Windows use kind to retrieve the kubeconfig. Docker Engine for Linux works with the default clusterctl approach.

```bash
kind get kubeconfig --name capi-quickstart > capi-quickstart.kubeconfig
```

<aside class="note warning">

Note: To use the default clusterctl method to retrieve kubeconfig for a workload cluster created with the Docker provider when using Docker Desktop see [Additional Notes for the Docker provider](../clusterctl/developers.md#additional-notes-for-the-docker-provider).

</aside>

{{#/tab }}
{{#/tabs }}

### Install a Cloud Provider

The Kubernetes in-tree cloud provider implementations are being [removed](https://github.com/kubernetes/enhancements/tree/master/keps/sig-cloud-provider/2395-removing-in-tree-cloud-providers) in favor of external cloud providers (also referred to as "out-of-tree"). This requires deploying a new component called the cloud-controller-manager which is responsible for running all the cloud specific controllers that were previously run in the kube-controller-manager. To learn more, see [this blog post](https://kubernetes.io/blog/2019/04/17/the-future-of-cloud-providers-in-kubernetes/).

{{#tabs name:"tab-install-cloud-provider" tabs:"Azure,OpenStack"}}
{{#tab Azure}}

Install the official cloud-provider-azure Helm chart on the workload cluster:

```bash
helm install --kubeconfig=./capi-quickstart.kubeconfig --repo https://raw.githubusercontent.com/kubernetes-sigs/cloud-provider-azure/master/helm/repo cloud-provider-azure --generate-name --set infra.clusterName=capi-quickstart --set cloudControllerManager.clusterCIDR="192.168.0.0/16"
```

For more information, see the [CAPZ book](https://capz.sigs.k8s.io/self-managed/addons.html).

{{#/tab }}
{{#tab OpenStack}}

Before deploying the OpenStack external cloud provider, configure the `cloud.conf` file for integration with your OpenStack environment:

```bash
cat > cloud.conf <<EOF
[Global]
auth-url=<your_auth_url>
application-credential-id=<your_credential_id>
application-credential-secret=<your_credential_secret>
region=<your_region>
domain-name=<your_domain_name>
EOF
```

For more detailed information on configuring the `cloud.conf` file, see the [OpenStack Cloud Controller Manager documentation](https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/openstack-cloud-controller-manager/using-openstack-cloud-controller-manager.md#config-openstack-cloud-controller-manager).

Next, create a Kubernetes secret using this configuration to securely store your cloud environment details.
You can create this secret for example with:

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig -n kube-system create secret generic cloud-config --from-file=cloud.conf
```

Now, you are ready to deploy the external cloud provider!

```bash
kubectl apply --kubeconfig=./capi-quickstart.kubeconfig -f https://raw.githubusercontent.com/kubernetes/cloud-provider-openstack/master/manifests/controller-manager/cloud-controller-manager-roles.yaml
kubectl apply --kubeconfig=./capi-quickstart.kubeconfig -f https://raw.githubusercontent.com/kubernetes/cloud-provider-openstack/master/manifests/controller-manager/cloud-controller-manager-role-bindings.yaml
kubectl apply --kubeconfig=./capi-quickstart.kubeconfig -f https://raw.githubusercontent.com/kubernetes/cloud-provider-openstack/master/manifests/controller-manager/openstack-cloud-controller-manager-ds.yaml
```

Alternatively, refer to the [helm chart](https://github.com/kubernetes/cloud-provider-openstack/tree/master/charts/openstack-cloud-controller-manager).

{{#/tab }}
{{#/tabs }}

### Deploy a CNI solution

Calico is used here as an example.

{{#tabs name:"tab-deploy-cni" tabs:"Azure,vcluster,KubeVirt,Other providers..."}}
{{#tab Azure}}

Install the official Calico Helm chart on the workload cluster:

```bash
helm repo add projectcalico https://docs.tigera.io/calico/charts --kubeconfig=./capi-quickstart.kubeconfig && \
helm install calico projectcalico/tigera-operator --kubeconfig=./capi-quickstart.kubeconfig -f https://raw.githubusercontent.com/kubernetes-sigs/cluster-api-provider-azure/main/templates/addons/calico/values.yaml --namespace tigera-operator --create-namespace
```

After a short while, our nodes should be running and in `Ready` state,
let's check the status using `kubectl get nodes`:

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig get nodes
```

{{#/tab }}
{{#tab vcluster}}

Calico not required for vcluster.

{{#/tab }}
{{#tab KubeVirt}}

Before deploying the Calico CNI, make sure the VMs are running:
```bash
kubectl get vm
```

If our new VMs are running, we should see a response similar to this:

```text
NAME                                  AGE    STATUS    READY
capi-quickstart-control-plane-7s945   167m   Running   True
capi-quickstart-md-0-zht5j            164m   Running   True
```

We can also read the virtual machine instances:
```bash
kubectl get vmi
```
The output will be similar to:
```text
NAME                                  AGE    PHASE     IP             NODENAME             READY
capi-quickstart-control-plane-7s945   167m   Running   10.244.82.16   kind-control-plane   True
capi-quickstart-md-0-zht5j            164m   Running   10.244.82.17   kind-control-plane   True
```

Since our workload cluster is running within the kind cluster, we need to prevent conflicts between the kind
(management) cluster's CNI, and the workload cluster CNI. The following modifications in the default Calico settings
are enough for these two CNI to work on (actually) the same environment.

* Change the CIDR to a non-conflicting range
* Change the value of the `CLUSTER_TYPE` environment variable to `k8s`
* Change the value of the `CALICO_IPV4POOL_IPIP` environment variable to `Never`
* Change the value of the `CALICO_IPV4POOL_VXLAN` environment variable to `Always`
* Add the `FELIX_VXLANPORT` environment variable with the value of a non-conflicting port, e.g. `"6789"`.

The following script downloads the Calico manifest and modifies the required field. The CIDR and the port values are examples.
```bash
curl https://raw.githubusercontent.com/projectcalico/calico/v3.24.4/manifests/calico.yaml -o calico-workload.yaml

sed -i -E 's|^( +)# (- name: CALICO_IPV4POOL_CIDR)$|\1\2|g;'\
's|^( +)# (  value: )"192.168.0.0/16"|\1\2"10.243.0.0/16"|g;'\
'/- name: CLUSTER_TYPE/{ n; s/( +value: ").+/\1k8s"/g };'\
'/- name: CALICO_IPV4POOL_IPIP/{ n; s/value: "Always"/value: "Never"/ };'\
'/- name: CALICO_IPV4POOL_VXLAN/{ n; s/value: "Never"/value: "Always"/};'\
'/# Set Felix endpoint to host default action to ACCEPT./a\            - name: FELIX_VXLANPORT\n              value: "6789"' \
calico-workload.yaml
```
Now, deploy the Calico CNI on the workload cluster:
```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig create -f calico-workload.yaml
```

After a short while, our nodes should be running and in `Ready` state, let’s check the status using `kubectl get nodes`:

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig get nodes
```

<aside class="note">

<h1>Troubleshooting</h1>

If the nodes don't become ready after a long period, read the pods in the `kube-system` namespace
```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig get pod -n kube-system
```

If the Calico pods are in image pull error state (`ErrImagePull`), it's probably because of the Docker Hub pull rate limit.
We can try to fix that by adding a secret with our Docker Hub credentials, and use it;
see [here](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/#registry-secret-existing-credentials)
for details.

First, create the secret. Please notice the Docker config file path, and adjust it to your local setting.
```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig create secret generic docker-creds \
    --from-file=.dockerconfigjson=<YOUR DOCKER CONFIG FILE PATH> \
    --type=kubernetes.io/dockerconfigjson \
    -n kube-system
```

Now, if the `calico-node` pods are with status of `ErrImagePull`, patch their DaemonSet to make them use the new secret to pull images:
```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig patch daemonset \
    -n kube-system calico-node \
    -p '{"spec":{"template":{"spec":{"imagePullSecrets":[{"name":"docker-creds"}]}}}}'
```

After a short while, the calico-node pods will be with `Running` status. Now, if the calico-kube-controllers pod is also
in `ErrImagePull` status, patch its deployment to fix the problem:
```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig patch deployment \
    -n kube-system calico-kube-controllers \
    -p '{"spec":{"template":{"spec":{"imagePullSecrets":[{"name":"docker-creds"}]}}}}'
```

Read the pods again
```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig get pod -n kube-system
```

Eventually, all the pods in the kube-system namespace will run, and the result should be similar to this:
```text
NAME                                                          READY   STATUS    RESTARTS   AGE
calico-kube-controllers-c969cf844-dgld6                       1/1     Running   0          50s
calico-node-7zz7c                                             1/1     Running   0          54s
calico-node-jmjd6                                             1/1     Running   0          54s
coredns-64897985d-dspjm                                       1/1     Running   0          3m49s
coredns-64897985d-pgtgz                                       1/1     Running   0          3m49s
etcd-capi-quickstart-control-plane-kjjbb                      1/1     Running   0          3m57s
kube-apiserver-capi-quickstart-control-plane-kjjbb            1/1     Running   0          3m57s
kube-controller-manager-capi-quickstart-control-plane-kjjbb   1/1     Running   0          3m57s
kube-proxy-b9g5m                                              1/1     Running   0          3m12s
kube-proxy-p6xx8                                              1/1     Running   0          3m49s
kube-scheduler-capi-quickstart-control-plane-kjjbb            1/1     Running   0          3m57s
```

</aside>

{{#/tab }}
{{#tab Other providers...}}

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig \
  apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.26.1/manifests/calico.yaml
```

After a short while, our nodes should be running and in `Ready` state,
let's check the status using `kubectl get nodes`:

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig get nodes
```
```bash
NAME                                          STATUS   ROLES           AGE    VERSION
capi-quickstart-vs89t-gmbld                   Ready    control-plane   5m33s  v1.32.0
capi-quickstart-vs89t-kf9l5                   Ready    control-plane   6m20s  v1.32.0
capi-quickstart-vs89t-t8cfn                   Ready    control-plane   7m10s  v1.32.0
capi-quickstart-md-0-55x6t-5649968bd7-8tq9v   Ready    <none>          6m5s   v1.32.0
capi-quickstart-md-0-55x6t-5649968bd7-glnjd   Ready    <none>          6m9s   v1.32.0
capi-quickstart-md-0-55x6t-5649968bd7-sfzp6   Ready    <none>          6m9s   v1.32.0
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

- Create a second workload cluster. Simply follow the steps outlined above, but remember to provide a different name for your second workload cluster.
- Deploy applications to your workload cluster. Use the [CNI deployment steps](#deploy-a-cni-solution) for pointers.
- See the [clusterctl] documentation for more detail about clusterctl supported actions.

<!-- links -->
[Experimental Features]: ../tasks/experimental-features/experimental-features.md
[Akamai (Linode) provider]: https://linode.github.io/cluster-api-provider-linode/introduction.html
[AWS provider prerequisites]: https://cluster-api-aws.sigs.k8s.io/topics/using-clusterawsadm-to-fulfill-prerequisites.html
[AWS provider releases]: https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases
[Azure Provider Prerequisites]: https://capz.sigs.k8s.io/getting-started.html#prerequisites
[bootstrap cluster]: ../reference/glossary.md#bootstrap-cluster
[capa]: https://cluster-api-aws.sigs.k8s.io
[capv-upload-images]: https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/docs/getting_started.md#uploading-the-machine-images
[clusterawsadm]: https://cluster-api-aws.sigs.k8s.io/clusterawsadm/clusterawsadm.html
[clusterctl generate cluster]: ../clusterctl/commands/generate-cluster.md
[clusterctl get kubeconfig]: ../clusterctl/commands/get-kubeconfig.md
[clusterctl]: ../clusterctl/overview.md
[Docker]: https://www.docker.com/
[GCP provider]: https://cluster-api-gcp.sigs.k8s.io/
[Helm]: https://helm.sh/docs/intro/install/
[Harvester provider]: https://github.com/rancher-sandbox/cluster-api-provider-harvester
[Hetzner provider]: https://github.com/syself/cluster-api-provider-hetzner
[Hivelocity provider]: https://github.com/hivelocity/cluster-api-provider-hivelocity
[IBM Cloud provider]: https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud
[infrastructure provider]: ../reference/glossary.md#infrastructure-provider
[ionoscloud provider]: https://github.com/ionos-cloud/cluster-api-provider-ionoscloud
[kind]: https://kind.sigs.k8s.io/
[KubeadmControlPlane]: ../developer/core/controllers/control-plane.md
[kubectl]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[management cluster]: ../reference/glossary.md#management-cluster
[Metal3 getting started guide]: https://github.com/metal3-io/cluster-api-provider-metal3/blob/master/docs/getting-started.md
[Metal3 provider]: https://github.com/metal3-io/cluster-api-provider-metal3/
[K0smotron provider]: https://github.com/k0sproject/k0smotron
[KubeKey provider]: https://github.com/kubesphere/kubekey
[KubeVirt provider]: https://github.com/kubernetes-sigs/cluster-api-provider-kubevirt/
[KubeVirt]: https://kubevirt.io/
[oci-provider]: https://oracle.github.io/cluster-api-provider-oci/#getting-started
[openstack-resource-controller]: https://k-orc.cloud/
[Equinix Metal getting started guide]: https://github.com/kubernetes-sigs/cluster-api-provider-packet#using
[provider]:../reference/providers.md
[provider components]: ../reference/glossary.md#provider-components
[vSphere getting started guide]: https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/docs/getting_started.md
[workload cluster]: ../reference/glossary.md#workload-cluster
[CAPI Operator quickstart]: ./quick-start-operator.md
[Proxmox getting started guide]: https://github.com/ionos-cloud/cluster-api-provider-proxmox/blob/main/docs/Usage.md
[Tinkerbell getting started guide]: https://github.com/tinkerbell/cluster-api-provider-tinkerbell/blob/main/docs/QUICK-START.md
