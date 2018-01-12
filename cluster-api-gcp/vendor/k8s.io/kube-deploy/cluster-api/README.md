# Cluster API
## What is the Cluster API?

The Cluster API is a Kubernetes project to bring declarative, Kubernetes-style
APIs to cluster creation, configuration, and management. It provides optional,
additive functionality on top of core Kubernetes.

Note that Cluster API effort is still in the prototype stage while we get
feedback on the API types themselves. All of the code here is to experiment with
the API and demo its abilities, in order to drive more technical feedback to the
API design. Because of this, all of the prototype code is rapidly changing.

To learn more, see the full [Cluster API proposal][proposal].

## Get involved!

* Join our Cluster API working group sessions
  * Weekly on Wednesdays @ 11:00 PT (19:00 UTC) on [Zoom][zoomMeeting]
  * Previous meetings: \[ [notes][notes] | [recordings][recordings] \]
* Chat with us on [Slack](http://slack.k8s.io/): #sig-cluster-lifecycle

## Getting Started
### Prerequisites

* `kubectl` is required, see [here](http://kubernetes.io/docs/user-guide/prereqs/).
* If you want to create a cluster on Google Cloud Platform (GCP):
  * The [Google Cloud SDK][gcpSDK] needs to be installed.
  * You will need to have a GCP account.

### Building

```bash
$ cd $GOPATH/src/k8s.io/
$ git clone git@github.com:kubernetes/kube-deploy.git
$ cd kube-deploy/cluster-api
$ go build
```

### Creating a cluster on GCP

1. Follow the above steps to clone the repository and build the `cluster-api` tool.
2. Update the `machines.yaml` file to give your preferred GCP project/zone in
each machine's `providerConfig` field.
   - *Optional*: Update the `cluster.yaml` file to change the cluster name.
3. Run `gcloud auth application-default login` to get default credentials.
4. Create a cluster: `./cluster-api create -c cluster.yaml -m machines.yaml`
5. Delete that cluster: `./cluster-api delete`

### How to use the API

To see how to build tooling on top of the Cluster API, please check out a few examples below:

* [upgrader](upgrader/README.md): a cluster upgrade tool.
* [repair](repair/README.md): detect problematic nodes and fix them.
* [machineset](examples/machineset/README.md): a client-side implementation of MachineSets for declaratively scaling Machines.

### How does the prototype work?

Right now, the Cluster and Machine objects are stored as Custom Resources (CRDs)
in the cluster's apiserver.  Like other resources in kubernetes, a [machine
controller](machine-controller/README.md) is run as a pod on the cluster to
reconcile the actual vs. desired machine state. Bootstrapping and in-place
upgrading is handled by kubeadm.

[proposal]: https://docs.google.com/document/d/1G2sqUQlOYsYX6w1qj0RReffaGXH4ig2rl3zsIzEFCGY/edit#
[notes]: https://docs.google.com/document/d/16ils69KImmE94RlmzjWDrkmFZysgB2J4lGnYMRN89WM/edit#heading=h.xqb69epnpv
[recordings]: https://www.youtube.com/watch?v=I9764DRBKLI&list=PL69nYSiGNLP29D0nYgAGWt1ZFqS9Z7lw4
[gcpSDK]: https://cloud.google.com/sdk/downloads
[zoomMeeting]: https://zoom.us/j/166836624
