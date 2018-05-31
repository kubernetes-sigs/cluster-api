# Cluster API vSphere Deployer

The Cluster API vSphere prototype implements the [Cluster API](https://github.com/kubernetes-sigs/cluster-api/blob/master/README.md) for vSphere 6.5.

## Getting Started

### Prerequisites

Follow the steps listed at [CONTRIBUTING.md](https://github.com/kubernetes-sigs/cluster-api/blob/master/vsphere-deployer/CONTRIBUTING.md) to:

1. Build the `vsphere-deployer` tool
   ```
   cd $GOPATH/src/sigs.k8s.io/cluster-api/vsphere-deployer/
   go build
   ```
1. Create a vSphere template following [this guide](https://blog.inkubate.io/deploy-a-vmware-vsphere-virtual-machine-with-terraform/) up to **Create the vSphere Ubuntu 16.04 template** section.
1. Create a resource pool for your cluster.
1. Create a pair of ssh keys that will be used to connect to the VMs. Put your
   keys in `~/.ssh/vsphere_tmp` and `~/.ssh/vsphere_tmp.pub`.
   ```
   ssh-keygen -b 2048 -t rsa -f ~/.ssh/vsphere_tmp -q -N ""
   ```
1. Create a `machines.yaml` file configured for your cluster. See the provided template
   for an example, if you use it, make sure to fill in all missing `terraformVariables`
   in `providerConfig`. You'll also need to create a `cluster.yaml` (look at the provided `cluster.yaml.template`).

### Limitation

See [here](https://github.com/karan/kube-deploy/issues?utf8=%E2%9C%93&q=is%3Aissue+is%3Aopen+vsphere).

### Creating a cluster

1. Create a cluster:
  ```
  ./vsphere-deployer create -c cluster.yaml -m machines.yaml -n vsphere_named_machines.yaml
  ```

During cluster creation, you can watch the machine resources get created in Kubernetes,
see the corresponding virtual machines created in your provider, and then finally see nodes
join the cluster:

```bash
$ watch -n 5 "kubectl get machines"
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
