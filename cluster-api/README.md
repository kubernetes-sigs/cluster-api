## What is cluster-api

It's a declarative way to create, configure, and manage a cluster. It provides an optional, additive functionality on
top of core Kubernetes.

Note that cluster-api effort is still in the prototype stage. All the code here is for experimental and demo-purpose,
and under rapid change.

## How is it implemented
We use custom resource type (CRD) to model new machine and cluster object. Just like other resources in kubernetes,
a [machine controller](machine-controller/README.md) is running as regular pod to reconcile the actual machine state vs desired state.

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

To see how we can use API to build toolings on top of cluster API, please check out a few examples below.

* [Upgrade](upgrader/README.md)
* [Repair](repair/README.md)
* [Scaling](examples/machineset/README.md)