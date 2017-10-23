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
3) Update test.yaml with your ssh key, google cloud project name and cluster name.
4) Run `gcloud auth application-default login` to get default credentials.
5) Create cluster `./cluster-api create test.yaml`
6) Delete cluster `./cluster-api delete test.yaml`

