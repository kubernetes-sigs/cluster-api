# ClusterClass

The ClusterClass feature introduces a new way to create clusters which reduces boilerplate and enables flexible and powerful customization of clusters.
ClusterClass is a powerful abstraction implemented on top of existing interfaces and offering a set of tools and operations to streamline cluster lifecycle management while maintaining the same underlying API.

This tutorial covers using Cluster API and ClusterClass to create a Kubernetes cluster and how to perform a one-touch Kubernetes version upgrade.

<aside class="note warning">

<h1>Note</h1>

This guide only covers creating a ClusterClass cluster with the CAPD provider.
</aside>

## Installation

### Prerequisites

#### Install tools

This guide requires the following tools are installed: 
- Install and setup [kubectl] and [clusterctl]
- Install [Kind] and [Docker]

#### Enable experimental features

ClusterClass is currently behind a feature gate that needs to be enabled.
This tutorial will also use another experimental gated feature - Cluster Resource Set. This is not required for ClusterClass to work but is used in this tutorial to set up networking.

To enable these features set the respective environment variables by running:
```bash
export EXP_CLUSTER_RESOURCE_SET=true
export CLUSTER_TOPOLOGY=true
```
This ensures that the Cluster Topology and Cluster Resource Set features are enabled when the providers are initialized.


#### Create a CAPI management cluster

A script to set up a Kind cluster pre-configured for CAPD (the docker infrastructure provider) can be found in the [hack folder of the core repo](https://raw.githubusercontent.com/kubernetes-sigs/cluster-api/main/hack/kind-install-for-capd.sh). 

To set up the cluster from the root of the repo run:
```bash
./hack/kind-install-for-capd.sh
clusterctl init --infrastructure docker
````

### Create a new Cluster using ClusterClass

#### Create the ClusterClass and templates

With a management Cluster with CAPD initialized and the Cluster Topology feature gate enabled, the next step is to create the ClusterClass and its referenced templates.
The ClusterClass - first in the yaml below - contains references to the templates needed to build a full cluster, defining a shape that can be re-used for any number of clusters.
* ClusterClass
* For the InfrastructureCluster:
  * DockerClusterTemplate
* For the ControlPlane:
  * KubeadmControlPlaneTemplate
  * DockerMachineTemplate 
* For the worker nodes:
  * DockerMachineTemplate
  * KubeadmConfigTemplate

The full ClusterClass definition can also be found in the [CAPI repo](https://raw.githubusercontent.com/kubernetes-sigs/cluster-api/main/docs/book/src/tasks/experimental-features/yamls/clusterclass.yaml).

<details><summary>ClusterClass</summary>

```yaml
{{#include ./yamls/clusterclass.yaml}}
```

</details>


To create the objects on your local cluster run:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/cluster-api/main/docs/book/src/tasks/experimental-features/yamls/clusterclass.yaml 
```

#### Enable networking for workload clusters

To make sure workload clusters come up with a functioning network a Kindnet ConfigMap with a Kindnet ClusterResourceSet is required. Kindnet only offers networking for Clusters built with Kind and CAPD. This can be substituted for any other networking solution for Kubernetes e.g. Calico as used in the Quickstart guide.

The kindnet configuration file can be found in the [CAPI repo](https://raw.githubusercontent.com/kubernetes-sigs/cluster-api/main/docs/book/src/tasks/experimental-features/yamls/kindnet-clusterresourceset.yaml).

To create the resources run:
```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/cluster-api/main/docs/book/src/tasks/experimental-features/yamls/kindnet-clusterresourceset.yaml 
```

#### Create the workload cluster

This is a Cluster definition that leverages the ClusterClass created above to define its shape.

<details><summary>Cluster</summary>

```yaml
{{#include ./yamls/clusterclass-quickstart.yaml}}
```
</details>

Create the Cluster object from the file in [the CAPI repo](https://raw.githubusercontent.com/kubernetes-sigs/cluster-api/main/docs/book/src/tasks/experimental-features/yamls/clusterclass-quickstart.yaml) with:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/cluster-api/main/docs/book/src/tasks/experimental-features/yamls/clusterclass-quickstart.yaml
```

#### Verify the workload cluster is running

The cluster will now start provisioning. You can check status with:

```bash
kubectl get cluster
```

Get a view of the cluster and its resources as they are created by running:

```bash
clusterctl describe cluster clusterclass-quickstart
```

To verify the control plane is up:

```bash
kubectl get kubeadmcontrolplane
```

The output should be similar to:

```bash
NAME                                    INITIALIZED   API SERVER AVAILABLE   VERSION   REPLICAS   READY   UPDATED   UNAVAILABLE
clusterclass-quickstart                 true                                 v1.21.2   1                  1         1
```


## Upgrade a Cluster using Managed Topology

The `spec.topology` field added to the Cluster object as part of ClusterClass allows changes made on the Cluster to be propagated across all relevant objects. This turns a Kubernetes cluster upgrade into a one-touch operation.
Looking at the newly-created cluster, the version of the control plane and the machine deployments is v1.21.2.

```bash
> kubectl get kubeadmcontrolplane,machinedeployments

NAME                                                                              CLUSTER                   INITIALIZED   API SERVER AVAILABLE   REPLICAS   READY   UPDATED   UNAVAILABLE   AGE     VERSION
kubeadmcontrolplane.controlplane.cluster.x-k8s.io/clusterclass-quickstart-XXXX    clusterclass-quickstart   true          true                   1          1       1         0             2m21s   v1.21.2

NAME                                                                             CLUSTER                   REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE     AGE     VERSION
machinedeployment.cluster.x-k8s.io/clusterclass-quickstart-linux-workers-XXXX    clusterclass-quickstart   1          1       1         0             Running   2m21s   v1.21.2

```

To update the Cluster the only change needed is to the `version` field under `spec.topology` in the Cluster object.


Change `1.21.2` to `1.22.0` as below. 
```bash
kubectl patch cluster clusterclass-quickstart --type json --patch '[{"op": "replace", "path": "/spec/topology/version", "value": "v1.22.0"}]'
```

The upgrade will take some time to roll out as it will take place machine by machine with older versions of the machines only being removed after healthy newer versions come online.

To watch the update progress run:
```bash
watch kubectl get kubeadmcontrolplane,machinedeployments
```
After a few minutes the upgrade will be complete and the output will be similar to:
```bash
NAME                                                                              CLUSTER                   INITIALIZED   API SERVER AVAILABLE   REPLICAS   READY   UPDATED   UNAVAILABLE   AGE     VERSION
kubeadmcontrolplane.controlplane.cluster.x-k8s.io/clusterclass-quickstart-XXXX    clusterclass-quickstart   true          true                   1          1       1         0             7m29s   v1.22.0

NAME                                                                             CLUSTER                   REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE     AGE     VERSION
machinedeployment.cluster.x-k8s.io/clusterclass-quickstart-linux-workers-XXXX    clusterclass-quickstart   1          1       1         0             Running   7m29s   v1.22.0
```

## Clean Up

Delete workload cluster.
```bash
kubectl delete cluster clusterclass-quickstart
```
<aside class="note warning">

IMPORTANT: In order to ensure a proper cleanup of your infrastructure you must always delete the cluster object. Deleting the entire cluster template with `kubectl delete -f clusterclass-quickstart.yaml` might lead to pending resources to be cleaned up manually.
</aside>

Delete management cluster
```bash
kind delete clusters capi-test
```

## Next steps

To see what else is made possible by ClusterClasses see the [ClusterClass operations guide].


<!-- links -->
[quick start guide]: ../../user/quick-start.md
[bootstrap cluster]: ../../reference/glossary.md#bootstrap-cluster
[clusterctl]: ../../user/quick-start.md#install-clusterctl
[Docker]: https://www.docker.com/
[infrastructure provider]: ../../reference/glossary.md#infrastructure-provider
[kind]: https://kind.sigs.k8s.io/
[KubeadmControlPlane]: ../../developer/architecture/controllers/control-plane.md
[kubectl]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[management cluster]: ../../reference/glossary.md#management-cluster
[provider]:../../reference/providers.md
[provider components]: ../../reference/glossary.md#provider-components
[workload cluster]: ../../reference/glossary.md#workload-cluster
[ClusterClass operations guide]: ./cluster-class-operations.md