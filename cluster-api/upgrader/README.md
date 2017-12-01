# Cluster Upgrader
Cluster upgrader is an standalone tool to upgrade the entire cluster including the control plan and nodes. It will only exercise cluster api w/o any cloud-specific logic.

## Build

```bash
$ cd $GOPATH/src/k8s.io/
$ git clone git@github.com:kubernetes/kube-deploy.git
$ cd kube-deploy/cluster-api/upgrader
$ go build
```

## Run
1) Spin up a cluster using cluster-api (the default version should be 1.7.4)
2) To update the entire cluster to v1.8.3, run `./upgrader -v 1.8.3`
