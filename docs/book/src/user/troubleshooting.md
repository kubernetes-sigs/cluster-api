# Troubleshooting

## Troubleshooting Quick Start with Docker (CAPD)

<aside class="note warning">

<h1>Warning</h1>

If you've run the Quick Start before ensure that you've [cleaned up](./quick-start.md#clean-up) all resources before trying it again. Check `docker ps` to ensure there are no running containers left before beginning the Quick Start.

</aside>

This guide assumes you've completed the [apply the workload cluster](./quick-start.md#apply-the-workload-cluster) section of the Quick Start using Docker.

When running `clusterctl describe cluster capi-quickstart` to verify the created resources, we expect the output to be similar to this (**note: this is before installing the Calico CNI**).

```shell
NAME                                                           READY  SEVERITY  REASON                       SINCE  MESSAGE
Cluster/capi-quickstart                                        True                                          46m
├─ClusterInfrastructure - DockerCluster/capi-quickstart-94r9d  True                                          48m
├─ControlPlane - KubeadmControlPlane/capi-quickstart-6487w     True                                          46m
│ └─3 Machines...                                              True                                          47m    See capi-quickstart-6487w-d5lkp, capi-quickstart-6487w-mpmkq, ...
└─Workers
  └─MachineDeployment/capi-quickstart-md-0-d6dn6               False  Warning   WaitingForAvailableMachines  48m    Minimum availability requires 3 replicas, current 0 available
    └─3 Machines...                                            True                                          47m    See capi-quickstart-md-0-d6dn6-584ff97cb7-kr7bj, capi-quickstart-md-0-d6dn6-584ff97cb7-s6cbf, ...
```

Machines should be started, but Workers are not because Calico isn't installed yet. You should be able to see the containers running with `docker ps --all` and they should not be restarting.

If you notice Machines are failing to start/restarting your output might look similar to this:

```shell
clusterctl describe cluster capi-quickstart
NAME                                                           READY  SEVERITY  REASON                       SINCE  MESSAGE
Cluster/capi-quickstart                                        False  Warning   ScalingUp                    57s    Scaling up control plane to 3 replicas (actual 2)
├─ClusterInfrastructure - DockerCluster/capi-quickstart-n5w87  True                                          110s
├─ControlPlane - KubeadmControlPlane/capi-quickstart-6587k     False  Warning   ScalingUp                    57s    Scaling up control plane to 3 replicas (actual 2)
│ ├─Machine/capi-quickstart-6587k-fgc6m                        True                                          81s
│ └─Machine/capi-quickstart-6587k-xtvnz                        False  Warning   BootstrapFailed              52s    1 of 2 completed
└─Workers
  └─MachineDeployment/capi-quickstart-md-0-5whtj               False  Warning   WaitingForAvailableMachines  110s   Minimum availability requires 3 replicas, current 0 available
    └─3 Machines...                                            False  Info      Bootstrapping                77s    See capi-quickstart-md-0-5whtj-5d8c9746c9-f8sw8, capi-quickstart-md-0-5whtj-5d8c9746c9-hzxc2, ...
```

In the example above we can see that the Machine `capi-quickstart-6587k-xtvnz` has failed to start. The reason provided is `BootstrapFailed`. 

To investigate why a machine fails to start you can inspect the conditions of the objects using `clusterctl describe --show-conditions all cluster capi-quickstart`. You can get more detailed information about the status of the machines using `kubectl describe machines`.

To inspect the underlying infrastructure - in this case docker containers acting as Machines - you can access the logs using `docker logs <MACHINE-NAME>`. For example:

```shell
docker logs capi-quickstart-6587k-xtvnz
(...)
Failed to create control group inotify object: Too many open files
Failed to allocate manager object: Too many open files
[!!!!!!] Failed to allocate manager object.
Exiting PID 1...
```

To resolve this specific error please read [Cluster API with Docker  - "too many open files"](#cluster-api-with-docker----too-many-open-files).



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
(see `kubeletExtraArgs` in the [CABPK examples](https://github.com/kubernetes-sigs/cluster-api/tree/main/bootstrap/kubeadm))
is not possible due to a security measure imposed by the
[`NodeRestriction` admission controller](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#noderestriction)
that kubeadm enables by default.

Assigning such labels to Nodes must be done after the bootstrap process has completed:

```bash
kubectl label nodes <name> node-role.kubernetes.io/worker=""
```

For convenience, here is an example one-liner to do this post installation

```bash
# Kubernetes 1.19 (kubeadm 1.19 sets only the node-role.kubernetes.io/master label)
kubectl get nodes --no-headers -l '!node-role.kubernetes.io/master' -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}' | xargs -I{} kubectl label node {} node-role.kubernetes.io/worker=''
# Kubernetes >= 1.20 (kubeadm >= 1.20 sets the node-role.kubernetes.io/control-plane label) 
kubectl get nodes --no-headers -l '!node-role.kubernetes.io/control-plane' -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}' | xargs -I{} kubectl label node {} node-role.kubernetes.io/worker=''
```                  

## Cluster API with Docker

When provisioning workload clusters using Cluster API with the Docker infrastructure provider,
provisioning might be stuck: 
 
1. if there are stopped containers on your machine from previous runs. Clean unused containers with [docker rm -f ](https://docs.docker.com/engine/reference/commandline/rm/). 

2. if the docker space on your disk is being exhausted 
    * Run [docker system df](https://docs.docker.com/engine/reference/commandline/system_df/) to inspect the disk space consumed by Docker resources.
    * Run [docker system prune --volumes](https://docs.docker.com/engine/reference/commandline/system_prune/) to prune dangling images, containers, volumes and networks.

   
## Cluster API with Docker  - "too many open files"
When creating many nodes using Cluster API and Docker infrastructure, either by creating large Clusters or a number of small Clusters, the OS may run into inotify limits which prevent new nodes from being provisioned.
If the error  `Failed to create inotify object: Too many open files` is present in the logs of the Docker Infrastructure provider this limit is being hit.

On Linux this issue can be resolved by increasing the inotify watch limits with:

```bash
sysctl fs.inotify.max_user_watches=1048576
sysctl fs.inotify.max_user_instances=8192
```

Newly created clusters should be able to take advantage of the increased limits.

### MacOS and Docker Desktop -  "too many open files" 
This error was also observed in Docker Desktop 4.3 and 4.4 on MacOS. It can be resolved by updating to Docker Desktop for Mac 4.5 or using a version lower than 4.3.

[The upstream issue for this error is closed as of the release of Docker 4.5.0](https://github.com/docker/for-mac/issues/6071)

Note: The below workaround is not recommended unless upgrade or downgrade can not be performed.

If using a version of Docker Desktop for Mac 4.3 or 4.4, the following workaround can be used:

Increase the maximum inotify file watch settings in the Docker Desktop VM:

1) Enter the Docker Desktop VM
```bash
nc -U ~/Library/Containers/com.docker.docker/Data/debug-shell.sock
```
2) Increase the inotify limits using sysctl
```bash
sysctl fs.inotify.max_user_watches=1048576
sysctl fs.inotify.max_user_instances=8192
```
3) Exit the Docker Desktop VM
```bash
exit
```

## Failed clusterctl init - 'failed to get cert-manager object'

When using older versions of Cluster API 0.4 and 1.0 releases - 0.4.6, 1.0.3 and older respectively - Cert Manager may not be downloadable due to a change in the repository location. This will cause `clusterctl init` to fail with the error:

```bash
clusterctl init --infrastructure docker
```
```bash
Fetching providers
Installing cert-manager Version="v1.10.0"
Error: action failed after 10 attempts: failed to get cert-manager object /, Kind=, /: Object 'Kind' is missing in 'unstructured object has no kind'
```

This error was fixed in more recent Cluster API releases on the 0.4 and 1.0 release branches. The simplest way to resolve the issue is to upgrade to a newer version of Cluster API for a given release. For who need to continue using an older release it is possible to override the repository used by `clusterctl init` in the clusterctl config file. The default location of this file is in `~/.cluster-api/clusterctl.yaml`.

To do so add the following to the file:
```yaml
cert-manager:
  url: "https://github.com/cert-manager/cert-manager/releases/latest/cert-manager.yaml"
```

Alternatively a Cert Manager yaml file can be placed in the [clusterctl overrides layer](../clusterctl/configuration.md#overrides-layer) which is by default in `$HOME/.cluster-api/overrides`. A Cert Manager yaml file can be placed at `$(HOME)/.cluster-api/overrides/cert-manager/v1.10.0/cert-manager.yaml`

More information on the clusterctl config file can be found at [its page in the book](../clusterctl/configuration.md#clusterctl-configuration-file)

## Failed clusterctl upgrade apply - 'failed to update cert-manager component'

Upgrading Cert Manager may fail due to a breaking change introduced in Cert Manager release v1.6.
An upgrade using `clusterctl` is affected when:

* using `clusterctl` in version `v1.1.4` or a more recent version.
* Cert Manager lower than version `v1.0.0` did run in the management cluster (which was shipped in Cluster API until including `v0.3.14`).

This will cause `clusterctl upgrade apply` to fail with the error:

```bash
clusterctl upgrade apply
```

```bash
Checking cert-manager version...
Deleting cert-manager Version="v1.5.3"
Installing cert-manager Version="v1.7.2"
Error: action failed after 10 attempts: failed to update cert-manager component apiextensions.k8s.io/v1, Kind=CustomResourceDefinition, /certificaterequests.cert-manager.io: CustomResourceDefinition.apiextensions.k8s.io "certificaterequests.cert-manager.io" is invalid: status.storedVersions[0]: Invalid value: "v1alpha2": must appear in spec.versions
```

The Cert Manager maintainers provide documentation to [migrate the deprecated API Resources](https://cert-manager.io/docs/installation/upgrading/remove-deprecated-apis/#upgrading-existing-cert-manager-resources) to the new storage versions to mitigate the issue.

More information about the change in Cert Manager can be found at [their upgrade notes from v1.5 to v1.6](https://cert-manager.io/docs/installation/upgrading/upgrading-1.5-1.6).

## Clusterctl failing to start providers due to outdated image overrides

clusterctl allows users to configure [image overrides](../clusterctl/configuration.md#image-overrides) via the clusterctl config file.
However, when the image override is pinning a provider image to a specific version, it could happen that this
conflicts with clusterctl behavior of picking the latest version of a provider.

E.g., if you are pinning KCP images to version v1.0.2 but then clusterctl init fetches yamls for version v1.1.0 or greater KCP will 
fail to start with the following error: 

```bash
invalid argument "ClusterTopology=false,KubeadmBootstrapFormatIgnition=false" for "--feature-gates" flag: unrecognized feature gate: KubeadmBootstrapFormatIgnition
```

In order to solve this problem you should specify the version of the provider you are installing by appending a
version tag to the provider name:

```bash
clusterctl init -b kubeadm:v1.0.2 -c kubeadm:v1.0.2 --core cluster-api:v1.0.2 -i docker:v1.0.2
```

Even if slightly verbose, pinning the version provides a better control over what is installed, as usually
required in an enterprise environment, especially if you rely on an internal repository with a separated
software supply chain or a custom versioning schema.

## Managed Cluster and co-authored slices

As documented in [#6320](https://github.com/kubernetes-sigs/cluster-api/issues/6320) managed topologies
assumes a slice to be either authored from templates or by the users/the infrastructure controllers.

In cases the slice is instead co-authored (templates provide some info, the infrastructure controller
fills in other info) this can lead to infinite reconcile.

A solution to this problem is being investigated, but in the meantime you should avoid co-authored slices.
