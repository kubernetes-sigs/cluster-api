# Cluster API Provider Docker

A temporary home for CAPD

## Manager Container Image

A sample is built and hosted at `gcr.io/kubernetes1-226021/capd-manager:latest` 

### Building the binaries

Requires go with go modules.

* `go build ./cmd/...`
* `go build ./cmd/`

### Building the image

Requires `gcloud` authenticated and configured.

Requires a google cloud project

`./scripts/publish-capd-manager.sh`

# Testing out CAPD

Tested on: Linux, OS X

Requirements: `kind` and `kubectl`

Install capkctl

`go install ./cmd/capkctl`

Start a management kind cluster

`capdctl setup`

Set up your `kubectl`

`export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"`

Install the cluster-api CRDs

`capdctl crds | kubectl apply -f -`

Run the capd & capi manager

`capdctl capd | kubectl apply -f -`

## Create a worker cluster

`kubectl apply -f examples/simple-cluster.yaml`

### Interact with a worker cluster

The kubeconfig is on the management cluster in secrets. Grab it and write it to a file:

`kubectl get secrets -o jsonpath='{.data.kubeconfig}' kubeconfig-my-cluster | base64 --decode > ~/.kube/kind-config-my-cluster`
 
`kubectl get po --all-namespaces --kubeconfig ~/.kube/kind-config-my-cluster`

