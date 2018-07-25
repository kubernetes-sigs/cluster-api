# Cluster Repair
`repair` is an standalone tool to detect problematic nodes in a cluster and
repair them. It is built on top of the Cluster API, and is an example of tooling
that can be built in a cloud-agnostic way.

## Build

```bash
$ cd $GOPATH/src/sigs.k8s.io/
$ git clone https://github.com/kubernetes-sigs/cluster-api
$ cd cluster-api/tools/repair
$ go build
```

## Run
1) Create a cluster using the `clusterctl` tool.
2) To do a dry run of detecting broken nodes and seeing what needs to be
repaired, run `./repair --dryrun true`.
3) To actually repair the nodes in cluster, run `./repair` without the
`--dryrun` flag.
