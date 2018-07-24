# Cluster Upgrader

`upgrader` is a standalone tool to upgrade an entire cluster, including the
control plane and all nodes. It is an example of a tool that can be written on
top of the Cluster API in a completely cloud-agnostic way.

## Building

```bash
$ cd $GOPATH/src/sigs.k8s.io/
$ git clone https://github.com/kubernetes-sigs/cluster-api
$ cd cluster-api/tools/upgrader
$ go build
```

## Running
1) First, create a cluster using the `clusterctl` tool (the default Kubernetes version should be `1.9.4`)
2) To update the entire cluster to `v1.9.5`, run `./upgrader -v 1.9.5`
