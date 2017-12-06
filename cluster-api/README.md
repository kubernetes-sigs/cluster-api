## What is the Cluster API?

The Cluster API is a Kubernetes project to bring declarative, Kubernetes-style
APIs to cluster creation, configuration, and management. It provides optional,
additive functionality on top of core Kubernetes.

Note that Cluster API effort is still in the prototype stage while we get
feedback on the API types themselves. All of the code here is to experiment with
the API and demo its abilities, in order to drive more technical feedback to the
API design. Because of this, all of the prototype code is rapidly changing.

To learn more, see the full [Cluster API
proposal](https://docs.google.com/document/d/1G2sqUQlOYsYX6w1qj0RReffaGXH4ig2rl3zsIzEFCGY/edit#).

## How is it implemented?
Right now, the Cluster and Machine objects are stored as Custom Resources (CRDs)
in the cluster's apiserver.  Like other resources in kubernetes, a [machine
controller](machine-controller/README.md) is run as a pod on the cluster to
reconcile the actual vs. desired machine state. Bootstrapping and in-place
upgrading is handled by kubeadm.

## How to run
### Prerequisite

* `kubectl` is required, see [here](http://kubernetes.io/docs/user-guide/prereqs/).
* `Google Cloud SDK` is installed if you are creating cluster on GCP, see [here](https://cloud.google.com/sdk/downloads).
* You need to have an account on Google Cloud Platform which have enough quota for the resource.

### How to build
```bash
$ cd $GOPATH/src/k8s.io/
$ git clone git@github.com:kubernetes/kube-deploy.git
$ cd kube-deploy/cluster-api
$ go build
```

### How to run

1) Follow steps mentioned above and build cluster-api.
2) Update cluster.yaml with cluster name.
3) Update machines.yaml with google cloud project name.
4) Run `gcloud auth application-default login` to get default credentials.
5) Create cluster: `./cluster-api create -c cluster.yaml -m machines.yaml`
6) Add new nodes: update new-machines.yaml with cloud project name and run `./cluster-api add -m new-machines.yaml`
7) Delete cluster: `./cluster-api delete`

### How to use the API

To see how to build tooling on top of the Cluster API, please check out a few examples below:

* [Upgrade](upgrader/README.md)
* [Repair](repair/README.md)
* [Scaling](examples/machineset/README.md)
