# Concepts

![](../images/management-cluster.svg)


### Management cluster

The cluster where one or more Infrastructure Providers run, and where resources (e.g. Machines) are stored.  Typically referred to when you are provisioning multiple clusters.

### Workload/Target Cluster

A cluster whose lifecycle is managed by the Management cluster.

### Infrastructure provider

A source of computational resources (e.g. machines, networking, etc.). Examples for cloud include AWS, Azure, Google, etc.; for bare metal include VMware, MAAS, metal3.io, etc. When there is more than one way to obtain resources from the same infrastructure provider (e.g. EC2 vs. EKS) each way is referred to as a variant.

### Bootstrap provider

The bootstrap provider is responsible for (usually by generating cloud-init or similar):

1. Generating the cluster certificates, if not otherwise specified
1. Initializing the control plane, and gating the creation of other nodes until it is complete
1. Joining master and worker nodes to the cluster

### Control plane

The control plane (sometimes referred to as master nodes) is a set of [services](https://kubernetes.io/docs/concepts/#kubernetes-control-plane) that serve the Kubernetes API and reconcile desired state through the control-loops.

* __Machine Based__ based control planes are the most common type deployment model and is used by tools like kubeadm and kubespray. Dedicated machines are provisioned running [*static pods*](https://kubernetes.io/docs/tasks/configure-pod-container/static-pod/) for the control plane components such as  [*kube-apiserver*](https://kubernetes.io/docs/admin/kube-apiserver/), [*kube-controller-manager*](https://kubernetes.io/docs/admin/kube-controller-manager/) and [*kube-scheduler*](https://kubernetes.io/docs/admin/kube-scheduler/).

* __Pod Based__  deployments require an external hosting cluster, the control plane is deployed using standard *Deployment* and *StatefulSet* objects and then the API exposed using a *Service*.

* __External__ control planes are offered and controlled by some system other than Cluster API (e.g., GKE, AKS, EKS, IKS).

As of v1alpha2 __Machine Based__ is the only supported Cluster API control plane type.
## Custom Resource Definitions (CRDs)

### Machine

A "Machine" is the declarative spec for a Node, as represented in Kubernetes core. If a new Machine object is created, a provider-specific controller will handle provisioning and installing a new host to register as a new Node matching the Machine spec. If the Machine's spec is updated, a provider-specific controller is responsible for updating the Node in-place or replacing the host with a new one matching the updated spec. If a Machine object is deleted, the corresponding Node should have its external resources released by the provider-specific controller, and should be deleted as well.

Fields like the kubelet version are modeled as fields on the Machine's spec. Any other information that is provider-specific, though, is part of the InfraProviderRef and is not portable between different providers.

#### Machine Immutability (In-place Upgrade vs. Replace)

From the perspective of Cluster API all machines are immutable, once they are created they are never updated (except for maybe labels, annotations and status) - only deleted.

For this reason, it is recommended to use MachineDeployments which handles changes to machines by replacing them in the same way regular Deployments handle changes to the podSpec.

### MachineDeployment

MachineDeployment work similar to regular POD [Deployments](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/) reconciling changes to a machine spec by rolling out changes to 2 MachineSets, the old and newly updated.

<!--TODO-->

### MachineSet

MachineSets work similar to regular POD [ReplicaSets](https://kubernetes.io/docs/concepts/workloads/controllers/replicaset/), MachineSets are not meant to be used directly, but are rather the mechanism MachineDeployments use to reconcile desired state.

<!--TODO-->

### BootstrapData

BootstrapData contains the machine or node role specific initialization data (usually cloud-init) used by the infrastructure provider to bootstrap a machine into a node.

<!--TODO-->

