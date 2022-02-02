# Troubleshooting

## Node bootstrap failures when using CABPK with cloud-init

Failures during Node bootstrapping can have a lot of different causes. For example, Cluster API resources might be 
misconfigured or there might be problems with the network. The following steps describe how bootstrap failures can 
be troubleshooted systematically.

1. Access the Node via ssh.
1. Take a look at cloud-init logs via `less /var/log/cloud-init-output.log` or `journalctl -u cloud-init --since "1 day ago"`.
   (Note: cloud-init persists logs of the commands it executes (like kubeadm) only after they have returned.)
1. It might also be helpful to take a look at `journalctl --since "1 day ago"`.
1. If you see that kubeadm times out waiting for the static Pods to come up, take a look at:
   1. containerd: `crictl ps -a`, `crictl logs`, `journalctl -u containerd`
   1. Kubelet: `journalctl -u kubelet --since "1 day ago"`
      (Note: it might be helpful to increase the Kubelet log level by e.g. setting `--v=8` via 
      `systemctl edit --full kubelet && systemctl restart kubelet`)
1. If Node bootstrapping consistently fails and the kubeadm logs are not verbose enough, the `kubeadm` verbosity 
   can be increased via `KubeadmConfigSpec.Verbosity`.

## Labeling nodes with reserved labels such as `node-role.kubernetes.io` fails with kubeadm error during bootstrap

Self-assigning Node labels such as `node-role.kubernetes.io` using the kubelet `--node-labels` flag
(see `kubeletExtraArgs` in the [CABPK examples](https://github.com/kubernetes-sigs/cluster-api/tree/master/bootstrap/kubeadm))
is not possible due to a security measure imposed by the
[`NodeRestriction` admission controller](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#noderestriction)
that kubeadm enables by default.

Assigning such labels to Nodes must be done after the bootstrap process has completed:

```
kubectl label nodes <name> node-role.kubernetes.io/worker=""
```

For convenience, here is an example one-liner to do this post installation

```
kubectl get nodes --no-headers -l '!node-role.kubernetes.io/master' -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}' | xargs -I{} kubectl label node {} node-role.kubernetes.io/worker=''
```                  

## Cluster API with Docker

When provisioning workload clusters using Cluster API with Docker infrastructure,
provisioning might be stuck: 
 
1. if there are stopped containers on your machine from previous runs. Clean unused containers with [docker rm -f ](https://docs.docker.com/engine/reference/commandline/rm/). 

2. if the docker space on your disk is being exhausted 
    * Run [docker system df](https://docs.docker.com/engine/reference/commandline/system_df/) to inspect the disk space consumed by Docker resources.
    * Run [docker system prune --volumes](https://docs.docker.com/engine/reference/commandline/system_prune/) to prune dangling images, containers, volumes and networks.


## Failed clusterctl init - 'failed to get cert-manager object'

When using older versions of Cluster API 0.4 and 1.0 releases - 0.4.6, 1.0.3 and older respectively - Cert Manager may not be downloadable due to a change in the repository location. This will cause `clusterctl init` to fail with the error:

```bash
$ cluster-api % clusterctl init --infrastructure docker

Fetching providers
Installing cert-manager Version="v1.5.3"
Error: action failed after 10 attempts: failed to get cert-manager object /, Kind=, /: Object 'Kind' is missing in 'unstructured object has no kind'
```

This error was fixed in more recent Cluster API releases on the 0.4 and 1.0 release branches. The simplest way to resolve the issue is to upgrade to a newer version of Cluster API for a given release. For who need to continue using an older release it is possible to override the repository used by `clusterctl init` in the clusterctl config file. The default location of this file is in `~/.cluster-api/clusterctl.yaml`.

To do so add the following to the file:
```yaml
cert-manager:
  url: "https://github.com/cert-manager/cert-manager/releases/latest/cert-manager.yaml"
```

Alternatively a Cert Manager yaml file can be placed in the [clusterctl overrides layer](../clusterctl/configuration.md#overrides-layer) which is by default in `$HOME/.cluster-api/overrides`. A Cert Manager yaml file can be placed at `$(HOME)/.cluster-api/overrides/cert-manager/v1.5.3/cert-manager.yaml`

More information on the clusterctl config file can be found at [its page in the book](../clusterctl/configuration.md#clusterctl-configuration-file)

>>>>>>> 98e3f975d (Fix cert manager repo and add troubleshooting guide)
