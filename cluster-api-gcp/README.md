# Cluster API GCP Prototype

The Cluster API GCP prototype implements the [Cluster API][https://github.com/kubernetes/kube-deploy/blob/master/cluster-api-gcp/README.md] for GCP.

## Getting Started
### Prerequisites

* `kubectl` is required, see [here](http://kubernetes.io/docs/user-guide/prereqs/).
* The [Google Cloud SDK][gcpSDK] needs to be installed.
* You will need to have a GCP account.
* Open firewall rules to allow communication (from kubectl and nodes) to the control plane.
     (Eg. Opening TCP port 443 from 0.0.0.0/0 to all machines tagged with "https-server"
     `gcloud compute firewall-rules create cluster-api-open --allow=TCP:443 --source-ranges=0.0.0.0/0 --target-tags='https-server'`)

### Building

```bash
$ cd $GOPATH/src/k8s.io/
$ git clone git@github.com:kubernetes/kube-deploy.git
$ cd kube-deploy/cluster-api-gcp
$ go build
```

### Creating a cluster on GCP

1. Follow the above steps to clone the repository and build the `cluster-api` tool.
2. Update the `machines.yaml` file to give your preferred GCP project/zone in
each machine's `providerConfig` field.
   - *Optional*: Update the `cluster.yaml` file to change the cluster name.
3. Run `gcloud auth application-default login` to get default credentials.
4. Create a cluster: `./cluster-api-gcp create -c cluster.yaml -m machines.yaml`
5. Delete that cluster: `./cluster-api-gcp delete`

### How does the prototype work?

Right now, the Cluster and Machine objects are stored as Custom Resources (CRDs)
in the cluster's apiserver.  Like other resources in kubernetes, a [machine
controller](machine-controller/README.md) is run as a pod on the cluster to
reconcile the actual vs. desired machine state. Bootstrapping and in-place
upgrading is handled by kubeadm.