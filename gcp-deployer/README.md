# Cluster API GCP Prototype

The Cluster API GCP prototype implements the [Cluster API](../README.md) for GCP.

## Getting Started

### Prerequisites

Follow the steps listed at [CONTRIBUTING.md](CONTRIBUTING.md) to:
1. Build the `gcp-deployer` tool
2. Generate base `machines.yaml` file configured for your GCP project
3. Login with gcloud `gcloud auth application-default login`

### Limitation

gcp-deployer tool only supports Kubernetes version 1.9 or newer.

### Creating a cluster

1. *Optional* update `machines.yaml` to give your preferred GCP zone in
each machine's `providerConfig` field.
1. *Optional*: Update `cluster.yaml` to set a custom cluster name.
1. Create a cluster: `./gcp-deployer create -c cluster.yaml -m machines.yaml -s machine_setup_configs.yaml`.
    1. **Note**: The `--machinesetup` or `-s` flag is set to `machine_setup_configs.yaml` by default.

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

You can add machines to your cluster using `kubectl apply` or `kubectl create`.

#### Upgrading your cluster

By default, your cluster will initially be running Kubernetes version 1.9.4. You
can upgrade the control plane or nodes using `kubectl edit` or you can run the
[upgrader tool](../tools/upgrader)
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
tool]( ../tools/repair)
to find the broken node (using the dry run flag) and fix it.


### Deleting a cluster

***NOTE***: Before deleting your cluster, it is recommended that you delete any Kubernetes
objects which have created resources external to the cluster, like services with type LoadBalancer,
some types of persistent volume claims, and ingress resources.

To delete your cluster run `./gcp-deployer delete`


### How does the prototype work?

Right now, the Cluster and Machine objects are stored as resources in an extension apiserver, which
connected with main apiserver through api aggregation. We deploy the extension API server and
controller manager as a pod inside the cluster. Like other resources in Kubernetes, machine
controller as part of controller manager is responsible to reconcile the actual vs. desired machine
state. Bootstrapping and in-place upgrading is handled by
[kubeadm](https://kubernetes.io/docs/setup/independent/create-cluster-kubeadm/).

### Machine Setup Configs

The `machine_setup_configs.yaml` file defines what machine setups are supported 
(i.e. what machine definitions are allowed in `machines.yaml`). 
Here is an [example yaml](machine_setup_configs.yaml) with a master config and a node config.
You are free to edit this file to support other machine setups (i.e. different versions, images, or startup scripts).
The yaml is unmarshalled into a [machinesetup.configList](../cloud/google/machinesetup/config_types.go).

#### Startup Scripts
Right now, the startup scripts must be bash scripts. 
There is a set of environment variables that is concatenated to the beginning of the startup script.
You can find them [here](../cloud/google/metadata.go) and use them in your startup script as needed.
