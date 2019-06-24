# Cluster API Provider Docker

## Manager Container Image

A sample is built and hosted at `gcr.io/kubernetes1-226021/capd-manager:latest`

### Building the binaries

Requires go 1.12+ with go modules.

```
# required if `cluster-api-provider-docker` was cloned into $GOPATH
export GO111MODULE=on
# build the binaries
go build ./cmd/capdctl
go build ./cmd/capd-manager
go build ./cmd/kind-test
```

### Building the image

Requires `gcloud` authenticated and configured.

Requires a google cloud project

`./scripts/publish-manager.sh`

#### Using Docker

Alternatively, replace "my-repository" with an appropriate prefix and run:

```
docker build -t my-repository/capd-manager:latest .
```

# Testing out CAPD

Tested on: Linux, works ok on OS X sometimes

Requirements: `kind` > 0.3.0 and `kubectl`

Install capdctl

`go install ./cmd/capdctl`

Start a management kind cluster

`capdctl setup`

Set up your `kubectl`

`export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"`

Install the cluster-api CRDs

`capdctl crds | kubectl apply -f -`

Run the capd & capi manager

`capdctl capd -capd-image=gcr.io/my-project/capd-manager:latest | kubectl apply -f -`

## Create a worker cluster

`kubectl apply -f examples/simple-cluster.yaml`

### Interact with a worker cluster

The kubeconfig is on the management cluster in secrets. Grab it and write it to a file:

`kubectl get secrets -o jsonpath='{.data.kubeconfig}' kubeconfig-my-cluster | base64 --decode > ~/.kube/kind-config-my-cluster`

`kubectl get po --all-namespaces --kubeconfig ~/.kube/kind-config-my-cluster`
