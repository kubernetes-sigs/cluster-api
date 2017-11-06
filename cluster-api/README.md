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
2) cluster-api script requires ssh key to talk to GCE to fetch kubeconfig. Follow [add ssh key to ssh agent](https://help.github.com/articles/generating-a-new-ssh-key-and-adding-it-to-the-ssh-agent/) to create ssh key(if you don't have one) and add it to ssh agent.
3) Update cluster.yaml with your public ssh key, google cloud project name and cluster name.
4) Update machines.yaml with google cloud project name.
5) Run `gcloud auth application-default login` to get default credentials.
6) Create cluster `./cluster-api create -c cluster.yaml -m machines.yaml`
7) Delete cluster `./cluster-api delete -c cluster.yaml -m machines.yaml`

