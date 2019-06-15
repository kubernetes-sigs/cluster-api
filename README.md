# Cluster API Provider Kind

A temporary home for CAPK

## Manager Container Image

A sample is built and hosted at `gcr.io/kubernetes1-226021/capk-manager:latest` 

### Building the binaries

Requires go 1.12? Probably less strict than that.

* `go build ./cmd/...`
* `go build ./cmd/`

### Building the image

Requires `gcloud` authenticated and configured.

Requires a google cloud project

`./scripts/publish-capk-manager.sh`

# Testing out CAPK

Tested on: Linux, OS X

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

The kubeconfig is on the management cluster in secrets. Grab it and write it to a file:

`kubectl get secrets -o jsonpath='{.data.kubeconfig}' kubeconfig-my-cluster | base64 --decode > ~/.kube/kind-config-my-cluster`
 
`kubectl get po --all-namespaces --kubeconfig ~/.kube/kind-config-my-cluster`

