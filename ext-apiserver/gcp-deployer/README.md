# Cluster API GCP Prototype

The Cluster API GCP prototype implements the [Cluster API](https://github.com/kubernetes/kube-deploy/blob/master/ext-apiserver/README.md) for GCP.

## Getting Started

### Prerequisites

Follow the steps listed at [CONTRIBUTING.md](https://github.com/kubernetes/kube-deploy/blob/master/ext-apiserver/gcp-deployer/CONTRIBUTING.md) to:
1. Build the `gcp-deployer` tool
2. Generate base `machines.yaml` file configured for your GCP project

### Limitation

gcp-deployer tool only supports Kubernetes version 1.8 or newer.

### Creating a cluster

1. *Optional* update `machines.yaml` to give your preferred GCP zone in
each machine's `providerConfig` field.
1. *Optional*: Update `cluster.yaml` to set a custom cluster name.
1. Create a cluster: `./gcp-deployer create -c cluster.yaml -m machines.yaml`

During cluster creation, you can watch the machine resources get created in Kubernetes,
see the corresponding virtual machines created in GCP, and then finally see nodes
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

You can add individual machines to your cluster using `kubectl apply` or
`kubectl create` or you can use the [client-side machineset
tool](https://github.com/kubernetes/kube-deploy/tree/master/ext-apiserver/tools/machineset)
to add (or remove) a bunch of identical nodes to your cluster.

#### Upgrading your cluster

By default, your cluster will initially be running Kubernetes version 1.7.4. You
can upgrade the control plane or nodes using `kubectl edit` or you can run the
[upgrader tool](https://github.com/kubernetes/kube-deploy/tree/master/ext-apiserver/tools/upgrader)
to upgrade your entire cluster with a single command.

#### Node repair

To test node repair, first pick a node, ssh into it, and "break" it by killing the `kubelet` process:

```
$ node=$(kubectl get nodes --no-headers | grep -v master | head -n 1 | awk '{print $1}')
$ gcloud compute ssh $node --zone us-central1-f
# sudo systemctl stop kubelet.service
# sudo systemctl daemon-reload
```

Then run the [node repair
tool]( https://github.com/kubernetes/kube-deploy/tree/master/ext-apiserver/tools/repair)
to find the broken node (using the dry run flag) and fix it.


### Deleting a cluster

To delete your cluster run `./gcp-deployer delete`


### How does the prototype work?

Right now, the Cluster and Machine objects are stored as resources in an extension apiserver, which
connected with main apiserver through api aggregation. We deploy the extension API server and
controller manager as a pod inside the cluster. Like other resources in Kubernetes, machine
controller as part of controller manager is responsible to reconcile the actual vs. desired machine
state. Bootstrapping and in-place upgrading is handled by
[kubeadm](https://kubernetes.io/docs/setup/independent/create-cluster-kubeadm/).
