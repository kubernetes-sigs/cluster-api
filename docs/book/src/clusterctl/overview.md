# Overview of clusterctl

The `clusterctl` CLI tool handles the lifecycle of a Cluster API management cluster.

## Day 1

The `clusterctl` user interface is specifically designed for providing a simple "day 1 experience" and a
quick start with Cluster API.

### Prerequisites

* Cluster API requires an existing Kubernetes cluster accessible via kubectl;

{{#tabs name:"tab-create-cluster" tabs:"Development,Production"}}
{{#tab Development}}

{{#tabs name:"tab-create-development-cluster" tabs:"kind,kind for docker provider,Minikube"}}
{{#tab kind}}

  ```bash
  kind create cluster --name=clusterapi
  kubectl cluster-info --context kind-clusterapi
  ```

See the [kind documentation](https://kind.sigs.k8s.io/) for more details.
{{#/tab }}
{{#tab kind for docker provider}}

If you are planning to use the Docker infrastructure provider, a custom kind cluster configuration is required
because the provider needs to access Docker on the host:

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
  ```

{{#/tab }}
{{#tab Minikube}}

 ```bash
 minikube start
 ```

See the [Minikube documentation](https://minikube.sigs.k8s.io/) for more details.
{{#/tab }}
{{#/tabs }}

{{#/tab }}
{{#tab Production}}

{{#tabs name:"tab-create-production-cluster" tabs:"Pre-Existing cluster"}}
{{#tab Pre-Existing cluster}}

For production use-cases a "real" kubernetes cluster should be used with appropriate backup and DR policies and procedures in place.

```bash
export KUBECONFIG=<...>
```
{{#/tab }}
{{#/tabs }}

{{#/tab }}
{{#/tabs }}

* If the provider of your choice expects some preliminary steps to be executed, users should take care of those in advance;
* If the provider of your choice expects some environment variables, e.g. `AWS_CREDENTIALS` for the `aws` 
infrastructure provider, user should ensure those variables are set in advance.

{{#tabs name:"tab-installation-infrastructure" tabs:"AWS,Azure,Docker,GCP,vSphere,OpenStack"}}
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

```bash
# Create the base64 encoded credentials
export AZURE_SUBSCRIPTION_ID_B64="$(echo -n "$AZURE_SUBSCRIPTION_ID" | base64 | tr -d '\n')"
export AZURE_TENANT_ID_B64="$(echo -n "$AZURE_TENANT_ID" | base64 | tr -d '\n')"
export AZURE_CLIENT_ID_B64="$(echo -n "$AZURE_CLIENT_ID" | base64 | tr -d '\n')"
export AZURE_CLIENT_SECRET_B64="$(echo -n "$AZURE_CLIENT_SECRET" | base64 | tr -d '\n')"
```
 
For more information about authorization, AAD, or requirements for Azure, visit the [Azure Provider Prerequisites](https://github.com/kubernetes-sigs/cluster-api-provider-azure/blob/master/docs/getting-started.md#prerequisites) document.

{{#/tab }}
{{#tab Docker}}

No additional pre-requisites.

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

Please visit the [getting started guide](https://github.com/kubernetes-sigs/cluster-api-provider-openstack/blob/master/docs/getting-started.md).

{{#/tab }}
{{#/tabs }}

### 1. Initialize the management cluster

The `clusterctl init` command installs the Cluster API components and transforms the Kubernetes cluster
into a management cluster.

The command accepts as input a list of providers to install; when executed for the first time, `clusterctl init` 
automatically adds to the list the `cluster-api` core provider, and if not specified, it
also adds the `kubeadm-bootstrap` and `kubeadm-controlplane` providers.

<aside class="note warning">

<h1>Action Required</h1>

If the component specification defined by one of the selected providers expects some environment variables, user 
should ensure those variables are set in advance.

</aside>

```shell
clusterctl init --infrastructure aws
```

The output of `clusterctl init` is similar to this:

```shell
performing init...
 - cluster-api CoreProvider installed (v0.2.8)
 - aws InfrastructureProvider installed (v0.4.1)

Your Cluster API management cluster has been initialized successfully!

You can now create your first workload cluster by running the following:

  clusterctl config cluster [name] --kubernetes-version [version] | kubectl apply -f -
```

See [`clusterctl init`](commands/init.md) for more details.

### 2. Create the first workload cluster

Once the management cluster is ready, you can create the first workload cluster.
 
The `clusterctl create config` command returns a YAML template for creating a workload cluster.
Store it locally, eventually customize it, and the apply it to start provisioning the workload cluster.


<aside class="note">

<h1> Which provider will be used for my cluster? </h1>

The `clusterctl config cluster` command uses smart defaults in order to simplify the user experience; in this example,
it detects that there is only an `aws` infrastructure provider and so it uses that when creating the cluster. 

</aside>


<aside class="note">

<h1> What topology will be used for my cluster? </h1>

The `clusterctl config cluster` uses cluster templates which are provided by the infrastructure providers.
See the provider's documentation for more information.

See [`clusterctl config cluster`](commands/config-cluster.md) for details about how to use alternative sources
for cluster templates.

</aside>

<aside class="note warning">

<h1>Action Required</h1>

If the cluster template defined by the infrastructure provider expects some environment variables, user 
should ensure those variables are set in advance.

See [`clusterctl config cluster`](commands/config-cluster.md) for details about how to discover the list of
variables required by a cluster templates.

</aside>

For example

```
clusterctl config cluster my-cluster --kubernetes-version v1.16.3 > my-cluster.yaml
```

Creates a YAML file named `my-cluster.yaml` with a predefined list of Cluster API objects; Cluster, Machines,
Machine Deployments, etc.

The file can be eventually modified using your editor of choice; when ready, run the following command
to apply the cluster manifest.

```
kubectl apply -f my-cluster.yaml
```

The output is similar to this:

```
kubeadmconfig.bootstrap.cluster.x-k8s.io/my-cluster-controlplane-0 created
kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io/my-cluster-worker created
cluster.cluster.x-k8s.io/my-cluster created
machine.cluster.x-k8s.io/my-cluster-controlplane-0 created
machinedeployment.cluster.x-k8s.io/my-cluster-worker created
awscluster.infrastructure.cluster.x-k8s.io/my-cluster created
awsmachine.infrastructure.cluster.x-k8s.io/my-cluster-controlplane-0 created
awsmachinetemplate.infrastructure.cluster.x-k8s.io/my-cluster-worker created
```

See [`clusterctl config cluster`](commands/config-cluster.md) for more details.

## Day 2 operations

The `clusterctl` command supports also day 2 operations:

* use [`clusterctl init`](commands/init.md) to install additional Cluster API providers
* use [`clusterctl upgrade`](commands/upgrade.md) to upgrade Cluster API providers
* use [`clusterctl delete`](commands/delete.md) to delete Cluster API providers

* use [`clusterctl config cluster`](commands/config-cluster.md) to spec out additional workload clusters
* use [`clusterctl move`](commands/move.md) to migrate objects defining a workload clusters (e.g. Cluster, Machines) from a management cluster to another management cluster
