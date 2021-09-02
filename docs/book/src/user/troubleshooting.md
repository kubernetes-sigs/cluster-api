# Troubleshooting

## Interpretting token refresh messages

Sometimes, you'll see that the bootstrapping secret is noted as not having been consumed yet, *even though* you correctly ran the bootstrapping phase on your node.  This happens because Cluster API didn't yet hear back from a VM that it expected to bootstrap and join the cluster.  In this case, you might see a recurrent:

```
Refreshing token until the infrastructure has a chance to consume it
```

message in the logs for your `capi-kubeadm-bootstrap-system` controller.  

## Reasoning about Node bootstrap failures when using CABPK with cloud-init

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

## Avoid mislabeling nodes with reserved labels such as `node-role.kubernetes.io` fails with kubeadm error during bootstrap

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

## Troubleshooting Windows Node Bootstrapping Errors

- Check for errors in the bootstrapper you are using, such as `C:\Program Files\Cloudbase Solutions\Cloudbase-Init\log`.
- Be sure to look at `DEBUG` logs, as you might have a fatal error hidden in these log messages.  Possible bootstrap errors are:
  - The windows node is missing some Binaries which you expected to have in the image on boot.
  - The windows node might be accidentally running powershell or choco commands during bootstrap that aren't installed. 


## Getting the bootstrap data manually to inspect it

Ultimately, if a VM isnt joining/bootstrapping into the cluster, you'll probably just want to see wether or not it has the bootstrap data.  This is the most direct way to determine where in the chain of command your bootstrap is failing.

- If your VM doesnt have the right bootstrap data, its because of a bug in the provider's ability to attach bootstrap data.
- If your VM does have the right bootstrap data, then theres a bug in the way you are configuring its startup logic, and this is most often the case.

Irrespective of OS, on any Vsphere VM, for you can confirm the bootstrap information in the `Configuration Parameters` section of your VSphere machine settings, which are available from the `VM Hardware -> Edit Settings -> Advanced -> Configuration Parameters` section, and then read them by copying them and running `base64 --decode` on their contents.   

Other clouds will have similar methodologies for inspecting the user data attached to a VM.  

## Cluster API with Docker

When provisioning workload clusters using Cluster API with Docker infrastructure,
provisioning might be stuck: 
 
1. if there are stopped containers on your machine from previous runs. Clean unused containers with [docker rm -f ](https://docs.docker.com/engine/reference/commandline/rm/). 

2. if the docker space on your disk is being exhausted 
    * Run [docker system df](https://docs.docker.com/engine/reference/commandline/system_df/) to inspect the disk space consumed by Docker resources.
    * Run [docker system prune --volumes](https://docs.docker.com/engine/reference/commandline/system_prune/) to prune dangling images, containers, volumes and networks.


