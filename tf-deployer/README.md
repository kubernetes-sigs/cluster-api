# Cluster API Terraform Prototype for vSphere 6.5

The Cluster API Terraform prototype implements the [Cluster API](https://github.com/kubernetes/kube-deploy/blob/master/cluster-api/README.md) for Terraform. The target provider for this prototype is vSphere 6.5.

## Getting Started

### Prerequisites

Follow the steps listed at [CONTRIBUTING.md](https://github.com/kubernetes/kube-deploy/blob/master/cluster-api/tf-deployer/CONTRIBUTING.md) to:

1. Build the `tf-deployer` tool
2. Create a vSphere template following [this guide](https://blog.inkubate.io/deploy-a-vmware-vsphere-virtual-machine-with-terraform/), and make sure you have sshd running, and that it had a public key (for which you have the private key). Additionally, make sure a VM running that image is capable of networking (`etc/networking/interfaces`). 
3. Create a `machines.yaml` file configured for your cluster. See the provided template for an example.

### Limitation

See [here](https://github.com/karan/kube-deploy/issues?utf8=%E2%9C%93&q=is%3Aissue+is%3Aopen+vsphere).

### Creating a cluster

1. Create a cluster: `./tf-deployer create -c cluster.yaml -m machines.yaml`

During cluster creation, you can watch the machine resources get created in Kubernetes,
see the corresponding virtual machines created in your provider, and then finally see nodes
join the cluster:

```bash
$ watch -n 5 "kubectl get machines"
$ watch -n 5 "gcloud compute instances list"
$ watch -n 5 "kubectl get nodes"
```


### Interacting with your cluster

Once you have created a cluster, you can interact with the cluster and machine
resources using kubectl:

```
$ kubectl get clusters
$ kubectl get machines
$ kubectl get machines -o yaml
```

#### Scaling your cluster

**NOT YET SUPPORTED!**

#### Upgrading your cluster

**NOT YET SUPPORTED!**

#### Node repair

**NOT YET SUPPORTED!**

### Deleting a cluster

**NOT YET SUPPORTED!**

### How does the prototype work?

Right now, the Cluster and Machine objects are stored as resources in an extension apiserver, which
connected with main apiserver through api aggregation. We deploy the extension API server and
controller manager as a pod inside the cluster. Like other resources in Kubernetes, machine
controller as part of controller manager is responsible to reconcile the actual vs. desired machine
state. Bootstrapping and in-place upgrading is handled by
[kubeadm](https://kubernetes.io/docs/setup/independent/create-cluster-kubeadm/).