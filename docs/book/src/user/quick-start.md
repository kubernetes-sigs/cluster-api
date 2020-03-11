# Quick Start

In this tutorial we'll cover the basics of how to use Cluster API to create one or more Kubernetes clusters.

{{#include ../tasks/installation.md}}

# Usage

## Initialize the management cluster

Now that we've got clusterctl installed and all the prerequisites in places, 
let's transforms the Kubernetes cluster into a management cluster by using the  `clusterctl init`. 

The command accepts as input a list of providers to install; when executed for the first time, `clusterctl init`
automatically adds to the list the `cluster-api` core provider, and if unspecified, it
also adds the `kubeadm` bootstrap and `kubeadm` control-plane providers.

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

See [`clusterctl init`](../clusterctl/commands/init.md) for more details.

## Create the first workload cluster

Once the management cluster is ready, you can create the first workload cluster.

### Preparing the Workload cluster configuration

The `clusterctl config cluster` command returns a YAML template for creating a [workload cluster].

<aside class="note">

<h1> Which provider will be used for my cluster? </h1>

The `clusterctl config cluster` command uses smart defaults in order to simplify the user experience; in this example,
it detects that there is only an `aws` infrastructure provider and so it uses that when creating the cluster.

</aside>


<aside class="note">

<h1> What topology will be used for my cluster? </h1>

The `clusterctl config cluster` by default uses cluster templates which are provided by the infrastructure providers.
See the provider's documentation for more information.

See [`clusterctl config cluster`](../clusterctl/commands/config-cluster.md) for details about how to use alternative sources.
for cluster templates.

</aside>

<aside class="note warning">

<h1>Action Required</h1>

If the cluster template defined by the infrastructure provider expects some environment variables, user
should ensure those variables are set in advance.

If you have followed [provided specific requirements](#provider-specific-prerequisites), everything should be already set
at this stage. Otherwise, you can look at [`clusterctl config cluster`](../clusterctl/commands/config-cluster.md) for details about how to discover the list of
variables required by a cluster templates.

</aside>

For the purpose of this tutorial, we’ll name our cluster capi-quickstart.

```
clusterctl config cluster capi-quickstart --kubernetes-version v1.17.0 --control-plane-machine-count=3 --worker-machine-count=3 > capi-quickstart.yaml
```

Creates a YAML file named `capi-quickstart.yaml` with a predefined list of Cluster API objects; Cluster, Machines,
Machine Deployments, etc.

The file can be eventually modified using your editor of choice.

See [`clusterctl config cluster`](../clusterctl/commands/config-cluster.md) for more details.
 
### Apply the Workload cluster

When ready, run the following command to apply the cluster manifest.

```
kubectl apply -f capi-quickstart.yaml
```

The output is similar to this:

```
cluster.cluster.x-k8s.io/capi-quickstart created
awscluster.infrastructure.cluster.x-k8s.io/capi-quickstart created
kubeadmcontrolplane.controlplane.cluster.x-k8s.io/capi-quickstart-control-plane created
awsmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-control-plane created
machinedeployment.cluster.x-k8s.io/capi-quickstart-md-0 created
awsmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-md-0 created
kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io/capi-quickstart-md-0 created
```

### Accessing Workload cluster

To verify the control plane is up, check if the control plane machine
has a ProviderID.
```
kubectl get machines --selector cluster.x-k8s.io/control-plane --all-namespaces
```

If the control plane is deployed using a control plane provider, such as
[KubeadmControlPlane][control-plane-controller] ensure the control plane is
ready.
```
kubectl get clusters --output jsonpath="{range .items[*]} [{.metadata.name} {.status.ControlPlaneInitialized} {.status.ControlPlaneReady}] {end}"
```

After the control plane node is up and ready, we can retrieve the [workload cluster] Kubeconfig:

```bash
kubectl --namespace=default get secret/capi-quickstart-kubeconfig -o jsonpath={.data.value} \
  | base64 --decode \
  > ./capi-quickstart.kubeconfig
```

### Deploy a CNI solution

Calico is used here as an example.

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig \
  apply -f https://docs.projectcalico.org/v3.12/manifests/calico.yaml
```

After a short while, our nodes should be running and in `Ready` state,
let's check the status using `kubectl get nodes`:

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig get nodes
```

# Next Steps

See the [clusterctl](../clusterctl/overview.md) documentation for more detail about clusterctl supported actions.

<!-- links -->
[control-plane-controller]: ../developer/architecture/controllers/control-plane.md
[workload cluster]: ../reference/glossary.md#workload-cluster
