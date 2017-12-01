# Cluster Repair
Cluster repair is an standalone tool to repair the problematic nodes in the cluster

## Build

```bash
$ cd $GOPATH/src/k8s.io/
$ git clone git@github.com:kubernetes/kube-deploy.git
$ cd kube-deploy/cluster-api/repair
$ go build
```

## Run
1) Spin up a cluster using cluster-api
2) To repair the nodes in cluster, run `./repair`
3) To do a dryrun to see what will happen, run `./repair --dryrun true`
