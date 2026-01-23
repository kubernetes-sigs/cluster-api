# Concepts

Similar to how you can use StatefulSets or Deployments in Kubernetes to manage a group of Pods, in Cluster API you can use custom resources like KubeadmControlPlane (a control plane implementation) to manage a set of control plane Machines, or you can use MachineDeployments to manage a group of worker Machines, each one of them representing a host server and the corresponding Kubernetes Node.

Extensibility is at the core of Cluster API and Cluster API providers like Cluster API provider VSphere, AWS, GCP etc. can be used
to deploy Cluster API managed Clusters to your preferred infrastructure, as well as to configure many other parts of the system.
![](../images/management-cluster.svg)

See also [Quick start](quick-start.md).

## Management cluster

A Kubernetes cluster where Cluster API and one or more Cluster API providers run, and that can be used to manage the lifecycle of your Kubernetes Cluster via a set of custom resources such as [Cluster](#cluster) or [Machines](#machine).

### Cluster

A "Cluster" is a custom resource that represent a Kubernetes cluster whose lifecycle is managed by Cluster API, usually also referred to as workload cluster.

Common properties such as network CIDRs are modeled as fields on the Cluster's spec. Any information that is provider-specific is part of the custom resources
referenced via `infrastructureRef` or `controlPlaneRef` and is not portable between different providers.

```yaml 
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
      - 192.168.0.0/16
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: VSphereCluster
    name: my-cluster-infrastructure
  controlPlaneRef:
    apiGroup: controlplane.cluster.x-k8s.io
    kind: KubeadmControlPlane
    name: my-control-plane
```

In most recent versions of Cluster API, the Cluster object can be used as a single point of control for the entire cluster.
See [ClusterClass](../tasks/experimental-features/cluster-class)

### Machine

A "Machine" is a custom resource providing the declarative spec for infrastructure hosting a Kubernetes Node (for example, a VM).

```yaml 
apiVersion: cluster.x-k8s.io/v1beta2
kind: Machine
metadata:
  name: my-machine
spec:
  clusterName: my-cluster
  version: v1.35.0
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: VSphereMachineTemplate
    name: my-machine-infrastructure
  bootstrap:
    configRef:
      apiGroup: bootstrap.cluster.x-k8s.io
      kind: KubeadmConfigTemplate
      name: my-bootstrap-config
status:
  nodeRef:
    name: the-node-running-on-my-machine
```

Common fields such as the Kubernetes version are modeled as fields on the Machine's spec. Any information that is provider-specific is part of the custom resources
referenced via `infrastructureRef` or `bootstrap.configRef` and is not portable between different providers.

If a new Machine object is created, a provider-specific controller will provision and install a new host to register as a new Node matching the Machine spec. If a Machine object is deleted, its underlying infrastructure and corresponding Node will be deleted.

Like for Pods in Kubernetes, also for Machines in Cluster API it is more convenient to not manage single Machines directly. Instead you should use resources like KubeadmControlPlane (a control plane implementation), [MachineDeployments](#machinedeployment) or [MachinePools](#machinepool) to manage a group of Machines.

#### Machine Immutability (In-place update vs. Replace)

From the perspective of Cluster API, all Machines are immutable: once they are created, they are never updated (except for labels, annotations and status), only deleted.

For this reason, MachineDeployments are preferable. MachineDeployments handle changes to machines by replacing them, in the same way core Deployments handle changes to Pod specifications.

Over time several improvement have been applied to Cluster API in oder to perform machine rollout only when necessary and
for minimizing risks and impact of this operation on users workloads.

Starting from Cluster API v1.12, users can intentionally trade off some of the benefits that they get of Machine immutability by
using Cluster API extensions points to add the capability to perform in-place updates under well-defined circumstances.

Notably, the Cluster API user experience will remain the same no matter of the in-place update feature is enabled
or not, because ultimately users should care ONLY about the desired state.

Cluster API is responsible to choose the best strategy to achieve desired state, and with the introduction of
update extensions, Cluster API is expanding the set of tools that can be used to achieve the desired state.

## Infrastructure provider

A component responsible for the provisioning of infrastructure/computational resources required by the Cluster or by Machines (e.g. VMs, networking, etc.). 
For example, cloud Infrastructure Providers include AWS, Azure, and Google, and bare metal Infrastructure Providers include VMware, MAAS, and metal3.io.

When there is more than one way to obtain resources from the same Infrastructure Provider (such as AWS offering both EC2 and EKS), each way is referred to as a variant.

## Control plane provider

A component responsible for the provisioning and for the management of the control plane of your Kubernetes Cluster, like e.g. the KubeadmControlPlane provider.

Control plane providers can take different approach on how to manage the control plane;

* __Self-provisioned__: A Kubernetes control plane consisting of pods or machines wholly managed by a single Cluster API deployment.
  e.g kubeadm uses [static pods](https://kubernetes.io/docs/tasks/configure-pod-container/static-pod/) for running components such as [kube-apiserver](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/), [kube-controller-manager](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-controller-manager/) and [kube-scheduler](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-scheduler/)
  on control plane machines.

* __Pod-based__ deployments require an external hosting cluster. The control plane components are deployed using standard *Deployment* and *StatefulSet* objects and the API is exposed using a *Service*.

* __External__  or __Managed__ control planes are offered and controlled by some system other than Cluster API, such as GKE, AKS, EKS, or IKS.

## Bootstrap provider

A component responsible for turning a server into a Kubernetes node as well as for:

1. Generating the cluster certificates, if not otherwise specified
2. Initializing the control plane, and gating the creation of other nodes until it is complete
3. Joining control plane and worker nodes to the cluster

Boostrap provider achieve this goal by generating BootstrapData, which contains the Machine or Node role-specific initialization data (usually cloud-init). The bootstrap data is used by the Infrastructure Provider to bootstrap a Machine into a Node.

## KubeadmControlPlane

The KubeadmControlPlane is a custom resource that is provided by the Kubeadm provider, and that allows to manage a set of Machines hosting control plane Nodes created with kubeadm.

Other control plane providers implement similar resources as well.

### MachineDeployment

A MachineDeployment provides declarative updates for Machines and MachineSets.

A MachineDeployment works similarly to a core Kubernetes [Deployment](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/). A MachineDeployment reconciles changes to a Machine spec by rolling out changes to 2 MachineSets, the old and the newly updated.

### MachinePool

A MachinePool is a declarative spec for a group of Machines. It is similar to a MachineDeployment, but is specific to a particular Infrastructure Provider. For more information, please check out [MachinePool](../tasks/experimental-features/machine-pools.md).

### MachineSet

A MachineSet's purpose is to maintain a stable set of Machines running at any given time.

A MachineSet works similarly to a core Kubernetes [ReplicaSet](https://kubernetes.io/docs/concepts/workloads/controllers/replicaset/). MachineSets are not meant to be used directly, but are the mechanism MachineDeployments use to reconcile desired state.

### MachineHealthCheck

A MachineHealthCheck defines the conditions when a Node should be considered missing or unhealthy.

If the Node matches these unhealthy conditions for a given user-configured time, the MachineHealthCheck initiates remediation of the Node. Remediation of Nodes is performed by replacing the corresponding Machine.

MachineHealthChecks will only remediate Nodes if they are owned by a MachineSet. This ensures that the Kubernetes cluster does not lose capacity, since the MachineSet will create a new Machine to replace the failed Machine.

## Custom Resource Definitions (CRDs)

A [CustomResourceDefinition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) is a built-in resource that lets you extend the Kubernetes API. Each CustomResourceDefinition represents a customization of a Kubernetes installation. The Cluster API provides and relies on several CustomResourceDefinitions:
