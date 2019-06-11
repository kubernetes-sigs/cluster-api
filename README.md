# Cluster API Provider Kind

A temporary home for CAPK

# Development

Please make an issue to discuss before large changes occur. 

# Manager Container Image

A sample is built and hosted at `gcr.io/kubernetes1-226021/capk-manager:latest` 

## Building the binaries

Requires go 1.12? Probably less strict than that.

* `go build ./cmd/...`
* `go build ./cmd/`

## Building the image

Requires `gcloud` authenticated and configured.

Requires a google cloud project

`./scripts/publish-capk-manager.sh`

# Running the capk manager

⚠️Only tested on linux⚠️

Start a management kind cluster

`capkctl setup`

Set up your `kubectl`

`export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"`

Install the cluster-api CRDs

`capkctl crds | kubectl apply -f -`

Run the capk & capi manager

`capkctl capk | kubectl apply -f -`

## Create a worker cluster

`kubectl apply -f examples/simple-cluster.yaml`

### Interact with a worker cluster

You will find the kubeconfigs in /kubeconfigs on the host. In this case:

`export KUBECONFIG=/kubeconfigs/kind-config-my-cluster`
