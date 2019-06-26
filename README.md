# Cluster API Provider Docker

## Manager Container Image

A sample is built and hosted at `gcr.io/kubernetes1-226021/capd-manager:latest`

### External Dependencies

- `go,  1.12+`
- `kind,  >= 0.3.0`
- `kubectl`
- `docker`

### Building Go binaries

Building Go binaries requires `go 1.12+` for go module support.

```
# required if `cluster-api-provider-docker` was cloned into $GOPATH
export GO111MODULE=on
# build the binaries
go build ./cmd/capdctl
go build ./cmd/capd-manager
go build ./cmd/kind-test
```

### Building the image

#### Using Gcloud

Make sure `gcloud` is authenticated and configured.

You also need to set up a google cloud project.

Run: `./scripts/publish-manager.sh`

#### Using Docker

Alternatively, run: `docker build -t <MY_REPOSITORY>/capd-manager:latest .`

## Trying CAPD

Tested on: Linux, works ok on OS X sometimes

Make sure you have `kind` > 0.3.0 and `kubectl`.

1. Install capdctl:

   `go install ./cmd/capdctl`

1. Start a management kind cluster

   `capdctl setup`

1. Set up your `kubectl`

   `export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"`

1. Install the cluster-api CRDs

   `capdctl crds | kubectl apply -f -`

1. Run the capd & capi manager

   `capdctl capd -capd-image=<YOUR_REGISTRY>/capd-manager:latest | kubectl apply -f -`

### Create a worker cluster

`kubectl apply -f examples/simple-cluster.yaml`

#### Interact with a worker cluster

The kubeconfig is on the management cluster in secrets. Grab it and write it to a file:

`kubectl get secrets -o jsonpath='{.data.kubeconfig}' kubeconfig-my-cluster | base64 --decode > ~/.kube/kind-config-my-cluster`

Look at the pods in your new worker cluster:
`kubectl get po --all-namespaces --kubeconfig ~/.kube/kind-config-my-cluster`
