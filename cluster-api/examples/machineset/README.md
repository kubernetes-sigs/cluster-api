# Machineset
Machineset is client side implementation for machine set concept. It allows scale up/down the nodes in the cluster.

## Build

```bash
$ cd $GOPATH/src/k8s.io/
$ git clone git@github.com:kubernetes/kube-deploy.git
$ cd kube-deploy/cluster-api/examples/machineset
$ go build
```

## Run
1) Spin up a cluster using cluster-api
2) To scale up or down the nodes to N, run `./machineset scale set=node --replicas N`
