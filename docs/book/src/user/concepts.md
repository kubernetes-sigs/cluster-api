# Concepts

![](../images/management-cluster.svg)

## Management cluster

A Kubernetes cluster that manages the lifecycle of Workload Clusters. A Management Cluster is also where one or more Infrastructure Providers run, and where resources such as Machines are stored.

### Workload cluster

A Kubernetes cluster whose lifecycle is managed by a Management Cluster.

## Infrastructure provider

A source of computational resources, such as compute and networking. For example, cloud Infrastructure Providers include AWS, Azure, and Google, and bare metal Infrastructure Providers include VMware, MAAS, and metal3.io.

When there is more than one way to obtain resources from the same Infrastructure Provider (such as AWS offering both EC2 and EKS), each way is referred to as a variant.

## Bootstrap provider

The Bootstrap Provider is responsible for:

1. Generating the cluster certificates, if not otherwise specified
1. Initializing the control plane, and gating the creation of other nodes until it is complete
1. Joining control plane and worker nodes to the cluster

## Control plane

The [control plane](https://kubernetes.io/docs/concepts/overview/components/) is a set of components that serve the Kubernetes API and continuously reconcile desired state using [control loops](https://kubernetes.io/docs/concepts/architecture/controller/).

* __Machine-based__ control planes are the most common type. Dedicated machines are provisioned, running [static pods](https://kubernetes.io/docs/tasks/configure-pod-container/static-pod/) for components such as [kube-apiserver](https://kubernetes.io/docs/admin/kube-apiserver/), [kube-controller-manager](https://kubernetes.io/docs/admin/kube-controller-manager/) and [kube-scheduler](https://kubernetes.io/docs/admin/kube-scheduler/).

* __Pod-based__ deployments require an external hosting cluster. The control plane components are deployed using standard *Deployment* and *StatefulSet* objects and the API is exposed using a *Service*.

* __External__ control planes are offered and controlled by some system other than Cluster API, such as GKE, AKS, EKS, or IKS.

The default provider uses kubeadm to bootstrap the control plane. As of v1alpha3, it exposes the configuration via the `KubeadmControlPlane` object. The controller, `capi-kubeadm-control-plane-controller-manager`, can then create Machine and BootstrapConfig objects based on the requested replicas in the `KubeadmControlPlane` object.

## Custom Resource Definitions (CRDs)

A [CustomResourceDefinition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) is a built-in resource that lets you extend the Kubernetes API. Each CustomResourceDefinition represents a customization of a Kubernetes installation. The Cluster API provides and relies on several CustomResourceDefinitions:

### Machine

A "Machine" is the declarative spec for an infrastructure component hosting a Kubernetes Node (for example, a VM). If a new Machine object is created, a provider-specific controller will provision and install a new host to register as a new Node matching the Machine spec. If the Machine's spec is updated, the controller replaces the host with a new one matching the updated spec. If a Machine object is deleted, its underlying infrastructure and corresponding Node will be deleted by the controller.

Common fields such as Kubernetes version are modeled as fields on the Machine's spec. Any information that is provider-specific is part of the `InfrastructureRef` and is not portable between different providers.

#### Machine Immutability (In-place Upgrade vs. Replace)

From the perspective of Cluster API, all Machines are immutable: once they are created, they are never updated (except for labels, annotations and status), only deleted.

For this reason, MachineDeployments are preferable. MachineDeployments handle changes to machines by replacing them, in the same way core Deployments handle changes to Pod specifications.

### MachineDeployment

A MachineDeployment provides declarative updates for Machines and MachineSets.

A MachineDeployment works similarly to a core Kubernetes [Deployment](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/). A MachineDeployment reconciles changes to a Machine spec by rolling out changes to 2 MachineSets, the old and the newly updated.

### MachineSet

A MachineSet's purpose is to maintain a stable set of Machines running at any given time.

A MachineSet works similarly to a core Kubernetes [ReplicaSet](https://kubernetes.io/docs/concepts/workloads/controllers/replicaset/). MachineSets are not meant to be used directly, but are the mechanism MachineDeployments use to reconcile desired state.

### MachineHealthCheck

A MachineHealthCheck defines the conditions when a Node should be considered unhealthy.

If the Node matches these unhealthy conditions for a given user-configured time, the MachineHealthCheck initiates remediation of the Node. Remediation of Nodes is performed by deleting the corresponding Machine.

MachineHealthChecks will only remediate Nodes if they are owned by a MachineSet. This ensures that the Kubernetes cluster does not lose capacity, since the MachineSet will create a new Machine to replace the failed Machine.

### BootstrapData

BootstrapData contains the Machine or Node role-specific initialization data (usually cloud-init) used by the Infrastructure Provider to bootstrap a Machine into a Node.
