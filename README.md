# Cluster API bootstrap provider kubeadm
## What is the Cluster API bootstrap provider kubeadm?

Cluster API bootstrap provider Kubeadm (CABPK) is a component of
[Cluster API](https://github.com/kubernetes-sigs/cluster-api/blob/master/README.md) 
that is responsible of generating a cloud-init script to
turn a Machine into a Kubernetes Node; this implementation uses [kubeadm](https://github.com/kubernetes/kubeadm) 
for kubernetes bootstrap.

### Resources

* [design doc](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20190610-machine-states-preboot-bootstrapping.md)
* [cluster-api.sigs.k8s.io](https://cluster-api.sigs.k8s.io)
* [The Kubebuilder Book](https://book.kubebuilder.io)

## How to build, deploy and test CABPK
CABPK is built using kubebuilder. Please refer to the [The Kubebuilder Book](https://book.kubebuilder.io) for build and deploy in a cluster.

CABPK is only part of a whole and does not function in isolation. It requires coordination
with the core provider, Cluster API (CAPI), and an infrastructure provider; see the [CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20190610-machine-states-preboot-bootstrapping.md)
document for more information. For the remainder of this document we will be referring to
[cluster-api-provider-docker](https://github.com/kubernetes-sigs/cluster-api-provider-docker)
(CAPD) as our infrastructure provider.

A convenient way to set up infrastructure for testing is to use [kind](https://kind.sigs.k8s.io/)
as the platform cluster to install the controllers and CRDs into. Then use a tool like
[tilt](https://tilt.dev/) or [skaffold](ttps://skaffold.dev/) to manage your dev environment.
A minimal example of a Tiltfile looks like this:

```
allow_k8s_contexts('kubernetes-admin@kubernetes')

controllers = {
    'capi': {
        'path': '../cluster-api',
        'image': 'gcr.io/k8s-staging-cluster-api/cluster-api-controller:dev',
    },
    'cabpk': {
        'path': './',
        'image': 'gcr.io/k8s-staging-cluster-api/cluster-api-bootstrap-provider-kubeadm:dev',
    },
    'capd': {
        'path': '../cluster-api-provider-docker',
        'image': 'gcr.io/k8s-staging-cluster-api/cluster-api-provider-docker:dev',
    },
}

for name, controller in controllers.items():
    command = '''sed -i'' -e 's@image: .*@image: '"{}"'@' ./{}/config/default/manager_image_patch.yaml'''.format(controller['image'], controller['path'])
    local(command)

    k8s_yaml(local('kustomize build ' + controller['path'] + '/config/default'))

    docker_build(controller['image'], controller['path'])

```

See [capi-dev](https://github.com/chuckha/capi-dev) for an example of a more complex developemt environment using [tilt](https://tilt.dev/).

## How does CABPK work?
Once your test environment is in place, create a `Cluster` object and its corresponding `DockerCluster`
infrastructure object.

```yaml
kind: DockerCluster
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
metadata:
  name: my-cluster-docker
---
kind: Cluster
apiVersion: cluster.x-k8s.io/v1alpha2
metadata:
  name: my-cluster
spec:
  infrastructureRef:
    kind: DockerCluster
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
    name: my-cluster-docker
```

Now you can start creating machines by defining a `Machine`, its corresponding `DockerMachine` object, and
the `KubeadmConfig` bootstrap object.

```yaml
kind: KubeadmConfig
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
metadata:
  name: my-control-plane1-config
spec:
  initConfiguration:
    nodeRegistration:
      kubeletExtraArgs:
        eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
  clusterConfiguration:
    controllerManager:
      extraArgs:
        enable-hostpath-provisioner: "true"
---
kind: DockerMachine
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
metadata:
  name: my-control-plane1-docker
---
kind: Machine
apiVersion: cluster.x-k8s.io/v1alpha2
metadata:
  name: my-control-plane1
  labels:
    cluster.x-k8s.io/cluster-name: my-cluster
    cluster.x-k8s.io/control-plane: "true"
    set: controlplane
spec:
  bootstrap:
    configRef:
      kind: KubeadmConfig
      apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
      name: my-control-plane1-config
  infrastructureRef:
    kind: DockerMachine
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
    name: my-control-plane1-docker
  version: "v1.14.2"
```

CABPK's main responsibility is to convert a `KubeadmConfig` bootstrap object into a cloud-init script that is
going to turn a Machine into a Kubernetes Node using `kubeadm`.

The cloud-init script will be saved into the `KubeadmConfig.Status.BootstrapData` and then the infrastructure provider
(CAPD in this example) will pick up this value and proceed with the machine creation and the actual bootstrap.

### KubeadmConfig objects
The `KubeadmConfig` object allows full control of Kubeadm init/join operations by exposing raw `InitConfiguration`,
`ClusterConfiguration` and `JoinConfiguration` objects.

CABPK will fill in some values if they are left empty with sensible defaults:

| `KubeadmConfig` field                           | Default                                                      |
| ----------------------------------------------- | ------------------------------------------------------------ |
| `clusterConfiguration.KubernetesVersion`        | `Machine.Spec.Version` [1]                                     |
| `clusterConfiguration.clusterName`              | `Cluster.metadata.name`                                      |
| `clusterConfiguration.controlPlaneEndpoint`     | `Cluster.status.apiEndpoints[0]` |
| `clusterConfiguration.networking.dnsDomain` | `Cluster.spec.clusterNetwork.serviceDomain`              |
| `clusterConfiguration.networking.serviceSubnet` | `Cluster.spec.clusterNetwork.service.cidrBlocks[0]`              |
| `clusterConfiguration.networking.podSubnet` | `Cluster.spec.clusterNetwork.pods.cidrBlocks[0]`              |
| `joinConfiguration.discovery`                   | a short lived BootstrapToken generated by CABPK              |

> IMPORTANT! overriding above defaults could lead to broken Clusters.

[1] if both `clusterConfiguration.KubernetesVersion` and `Machine.Spec.Version` are empty, the latest Kubernetes
version will be installed (as defined by the default kubeadm behavior). 

#### Examples
Valid combinations of configuration objects are:
- at least one of `InitConfiguration` and `ClusterConfiguration` for the first control plane node only
- `JoinConfiguration` for worker nodes and additional control plane nodes

Bootstrap control plane node:
```yaml
kind: KubeadmConfig
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
metadata:
  name: my-control-plane1-config
spec:
  initConfiguration:
    nodeRegistration:
      kubeletExtraArgs:
        eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
  clusterConfiguration:
    controllerManager:
      extraArgs:
        enable-hostpath-provisioner: "true"
```

Additional control plane nodes:
```yaml
kind: KubeadmConfig
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
metadata:
  name: my-control-plane2-config
spec:
  joinConfiguration:
    nodeRegistration:
      kubeletExtraArgs:
        eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
    controlPlane: {}
```

worker nodes:
```yaml
kind: KubeadmConfig
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
metadata:
  name: my-worker1-config
spec:
  joinConfiguration:
    nodeRegistration:
      kubeletExtraArgs:
        eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
```

### Bootstrap Orchestration
CABPK supports multiple control plane machines initing at the same time.
The generation of cloud-init scripts of different machines is orchestrated in order to ensure a cluster
bootstrap process that will be compliant with the correct Kubeadm init/join sequence. More in detail:
1. cloud-config-data generation starts only after `Cluster.InfrastructureReady` flag is set to `true`.
2. at this stage, cloud-config-data will be generated for the first control plane machine even
if multiple control plane machines are ready (kubeadm init).
3. after `Cluster.metadata.Annotations[cluster.x-k8s.io/control-plane-ready]` is set to true,
the cloud-config-data for all the other machines are generated (kubeadm join/join —control-plane).

### Certificate Management
The user can choose two approaches for certificate management:
1. provide required certificate authorities (CAs) to use for `kubeadm init/kubeadm join --control-plane`; such CAs
should be provided as a `Secrets` objects in the management cluster.
2. let CABPK to generate the necessary `Secrets` objects with a self-signed certificate authority for kubeadm

TODO: Add more info about certificate secrets

### Additional Features
The `KubeadmConfig` object supports customizing the content of the config-data:

- `KubeadmConfig.Files` specifies additional files to be created on the machine
- `KubeadmConfig.PreKubeadmCommands` specifies a list of commands to be executed before `kubeadm init/join`
- `KubeadmConfig.PostKubeadmCommands` same as above, but after `kubeadm init/join`
- `KubeadmConfig.Users` specifies a list of users to be created on the machine
- `KubeadmConfig.NTP` specifies NPT settings for the machine

## Versioning, Maintenance, and Compatibility

- We follow [Semantic Versioning (semver)](https://semver.org/).
- Cluster API bootstrap provider kubeadm versioning is syncronized with
  [Cluster API](https://github.com/kubernetes-sigs/cluster-api/blob/master/README.md).
- The _master_ branch is where development happens, this might include breaking changes.
- The _release-X_ branches contain stable, backward compatible code. A new _release-X_ branch
  is created at every major (X) release.

## Get involved!

* Join the [Cluster API discuss forum](https://discuss.kubernetes.io/c/contributors/cluster-api).

* Join the [sig-cluster-lifecycle](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle)
Google Group for access to documents and calendars.

* Join our Cluster API working group sessions
  * Weekly on Wednesdays @ 10:00 PT on [Zoom][zoomMeeting]
  * Previous meetings: \[ [notes][notes] | [recordings][recordings] \]

* Provider implementer office hours
  * Weekly on Tuesdays @ 12:00 PT ([Zoom][providerZoomMeetingTues]) and Wednesdays @ 15:00 CET ([Zoom][providerZoomMeetingWed])
  * Previous meetings: \[ [notes][implementerNotes] \]

* Chat with us on [Slack](http://slack.k8s.io/): #cluster-api

[notes]: https://docs.google.com/document/d/1Ys-DOR5UsgbMEeciuG0HOgDQc8kZsaWIWJeKJ1-UfbY/edit
[recordings]: https://www.youtube.com/playlist?list=PL69nYSiGNLP29D0nYgAGWt1ZFqS9Z7lw4
[zoomMeeting]: https://zoom.us/j/861487554
[implementerNotes]: https://docs.google.com/document/d/1IZ2-AZhe4r3CYiJuttyciS7bGZTTx4iMppcA8_Pr3xE/edit
[providerZoomMeetingTues]: https://zoom.us/j/140808484
[providerZoomMeetingWed]: https://zoom.us/j/424743530
