Note that cluster-api effort is still in the prototype stage. All the code here is for experimental and demo-purpose, and under rapid change.

### How to build

```bash
$ cd $GOPATH/src/k8s.io/
$ git clone git@github.com:kubernetes/kube-deploy.git
$ cd kube-deploy/cluster-api
$ go build
```

### How to run
1) Follow steps mentioned above and build cluster-api.
2) Update cluster.yaml with cluster name.
3) Update machines.yaml with google cloud project name.
4) Run `gcloud auth application-default login` to get default credentials.
5) Create cluster: `./cluster-api create -c cluster.yaml -m machines.yaml`
6) Add new nodes: update new-machines.yaml with cloud project name and run `./cluster-api add -m new-machines.yaml`
7) Delete cluster: `./cluster-api delete`