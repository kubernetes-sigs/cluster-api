# Cluster API GCP Prototype

The Cluster API GCP prototype implements the [Cluster API](https://github.com/kubernetes/kube-deploy/blob/master/cluster-api/README.md) for GCP.

## Getting Started

### Prerequisites

* `kubectl` is required, see [here](http://kubernetes.io/docs/user-guide/prereqs/).
* The [Google Cloud SDK](https://cloud.google.com/sdk/downloads) needs to be installed.
* You will need to have a GCP account.
* Create a firewall rule to allow communication from kubectl (and nodes) to the control plane.

   ```bash
   gcloud compute firewall-rules create cluster-api-open --allow=TCP:443 --source-ranges=0.0.0.0/0 --target-tags='https-server'
   ```


### Building

```bash
$ cd $GOPATH/src/k8s.io/
$ git clone git@github.com:kubernetes/kube-deploy.git
$ cd kube-deploy/cluster-api-gcp
$ go build
```


### Creating a cluster

1. Follow the above steps to clone the repository and build the `cluster-api-gcp` tool.
1. Update `machines.yaml` to give your preferred GCP project/zone in
each machine's `providerConfig` field.
   - *Optional*: Update `cluster.yaml` to set a custom cluster name.
1. Run `gcloud auth application-default login` to get default credentials.
1. Create a cluster: `./cluster-api-gcp create -c cluster.yaml -m machines.yaml`

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
tool](https://github.com/kubernetes/kube-deploy/tree/master/cluster-api/tools/machineset)
to add (or remove) a bunch of identical nodes to your cluster.

#### Upgrading your cluster

By default, your cluster will initially be running Kubernetes version 1.7.4. You
can upgrade the control plane or nodes using `kubectl edit` or you can run the
[upgrader tool](https://github.com/kubernetes/kube-deploy/tree/master/cluster-api/tools/upgrader)
to upgrade your entire cluster with a single command.

#### Node repair

To test node repair, first pick a node, ssh into it, and "break" it by killing the `kubelet` process:

```
$ node=$(kubectl get nodes --no-headers | head -1 | awk '{print $1}')
$ gcloud compute ssh $node --zone us-central1-f
# sudo systemctl stop kubelet.service
# sudo systemctl daemon-reload
```

Then run the [node repair
tool]( https://github.com/kubernetes/kube-deploy/tree/master/cluster-api/tools/repair)
to find the broken node (using the dry run flag) and fix it.


### Deleting a cluster

To delete your cluster run `./cluster-api-gcp delete`


### How does the prototype work?

Right now, the Cluster and Machine objects are stored as Custom Resources (CRDs)
in the cluster's apiserver.  Like other resources in Kubernetes, a [machine
controller](machine-controller/README.md) is run as a pod on the cluster to
reconcile the actual vs. desired machine state. Bootstrapping and in-place
upgrading is handled by
[kubeadm](https://kubernetes.io/docs/setup/independent/create-cluster-kubeadm/).
