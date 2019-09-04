# Quick Start

In this tutorial we'll cover the basics of how to use Cluster API to create one or more Kubernetes clusters.

## Prerequisites

- Install and setup [kubectl] in your local environment.
- A Kubernetes cluster to be used as [management cluster](../reference/glossary.md#management-cluster), for the purpose of this quick start, we suggest you to use [kind].
    ```bash
    kind create cluster --name=clusterapi
    export KUBECONFIG="$(kind get kubeconfig-path --name="clusterapi")"
    ```

## Installation

Using [kubectl], let's create the components on the management cluster:

#### Install Cluster API Components

Check the [releases](https://github.com/kubernetes-sigs/cluster-api/releases) for an up-to-date components file.

```bash
kubectl create -f https://github.com/kubernetes-sigs/cluster-api/releases/download/v0.2.1/cluster-api-components.yaml
```

#### Install the Bootstrap Provider Components

{{#tabs name:"tab-installation-bootstrap" tabs:"Kubeadm"}}
{{#tab Kubeadm}}

Check the [Kubeadm provider releases](https://github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm/releases) for an up-to-date components file.

```bash
kubectl create -f https://github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm/releases/download/v0.1.0/bootstrap-components.yaml
```

{{#/tab }}
{{#/tabs }}


#### Install Infrastructure Provider Components

{{#tabs name:"tab-installation-infrastructure" tabs:"AWS,vSphere"}}
{{#tab AWS}}

Check the [AWS provider releases](https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases) for an up-to-date components file.

For more information about prerequisites, credentials management, and or permissions for AWS, visit the [getting started guide](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/master/docs/getting-started.md).

```bash
kubectl create -f https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/download/v0.4.0/infrastructure-components.yaml
```

{{#/tab }}
{{#tab vSphere}}

Check the [vSphere provider releases](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/releases) for an up-to-date components file.

For more information about prerequisites, credentials management, and or permissions for vSphere, visit the [getting started guide](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/docs/getting_started.md).

```bash
kubectl create -f https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/releases/download/v0.5.0/infrastructure-components.yaml
```

{{#/tab }}
{{#/tabs }}

## Usage

The Cluster API resources are now installed.

![](../images/management-cluster.svg)

Now that we've got Cluster API, Bootstrap and Infrastructure resources installed,
let's proceed to create a single node cluster.

For the purpose of this tutorial, we'll name our cluster `capi-quickstart`.

{{#tabs name:"tab-usage-cluster-resource" tabs:"AWS,vSphere"}}
{{#tab AWS}}

```yaml
---
apiVersion: cluster.x-k8s.io/v1alpha2
kind: Cluster
metadata:
  name: capi-quickstart
spec:
  clusterNetwork:
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
    kind: AWSCluster
    name: capi-quickstart
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: AWSCluster
metadata:
  name: capi-quickstart
spec:
  region: us-east-1 # Change this value to the region you want to deploy the cluster in.
  sshKeyName: default # Change this value to a valid SSH Key Pair present in your AWS Account.
```
{{#/tab }}
{{#tab vSphere}}

<aside class="note warning">

<h1>Action Required</h1>

These examples include environment variables that you should substitute before creating the resources.

</aside>

```yaml
---
apiVersion: cluster.x-k8s.io/v1alpha2
kind: Cluster
metadata:
  name: capi-quickstart
spec:
  clusterNetwork:
    pods:
      cidrBlocks: ["192.168.0.0/16"] # CIDR block used by Calico.
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
    kind: VSphereCluster
    name: capi-quickstart
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: VSphereCluster
metadata:
  name: capi-quickstart
spec:
  server: "${VSPHERE_SERVER}"
  cloudProviderConfiguration:
    global:
      secretName: "cloud-provider-vsphere-credentials"
      secretNamespace: "kube-system"
    virtualCenter:
      "${VSPHERE_SERVER}":
        datacenters: "${VSPHERE_DATACENTER}"
    network:
      name: "${VSPHERE_NETWORK}"
    workspace:
      server: "${VSPHERE_SERVER}"
      datacenter: "${VSPHERE_DATACENTER}"
      datastore: "${VSPHERE_DATASTORE}"
      resourcePool: "${VSPHERE_RESOURCE_POOL}"
      folder: "${VSPHERE_FOLDER}"
```
{{#/tab }}
{{#/tabs }}

Now that we've created the cluster object, we can create a control plane Machine.

{{#tabs name:"tab-usage-controlplane-resource" tabs:"AWS,vSphere"}}
{{#tab AWS}}

```yaml
---
apiVersion: cluster.x-k8s.io/v1alpha2
kind: Machine
metadata:
  name: capi-quickstart-controlplane-0
  labels:
    cluster.x-k8s.io/control-plane: "true"
    cluster.x-k8s.io/cluster-name: "capi-quickstart"
spec:
  version: v1.15.3
  bootstrap:
    configRef:
      apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
      kind: KubeadmConfig
      name: capi-quickstart-controlplane-0
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
    kind: AWSMachine
    name: capi-quickstart-controlplane-0
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: AWSMachine
metadata:
  name: capi-quickstart-controlplane-0
spec:
  instanceType: t3.large
  iamInstanceProfile: "controllers.cluster-api-provider-aws.sigs.k8s.io" # This IAM profile is part of the pre-requisites.
  sshKeyName: default # Change this value to a valid SSH Key Pair present in your AWS Account.
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
kind: KubeadmConfig
metadata:
  name: capi-quickstart-controlplane-0
spec:
  initConfiguration:
    nodeRegistration:
      name: '{{ ds.meta_data.hostname }}'
      kubeletExtraArgs:
        cloud-provider: aws
  clusterConfiguration:
    apiServer:
      extraArgs:
        cloud-provider: aws
    controllerManager:
      extraArgs:
        cloud-provider: aws
```
{{#/tab }}
{{#tab vSphere}}

<aside class="note warning">

<h1>Action Required</h1>

These examples include environment variables that you should substitute before creating the resources.

</aside>

```yaml
---
apiVersion: cluster.x-k8s.io/v1alpha2
kind: Machine
metadata:
  name: capi-quickstart-controlplane-0
  labels:
    cluster.x-k8s.io/control-plane: "true"
    cluster.x-k8s.io/cluster-name: "capi-quickstart"
spec:
  version: v1.15.3
  bootstrap:
    configRef:
      apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
      kind: KubeadmConfig
      name: capi-quickstart-controlplane-0
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
    kind: VSphereMachine
    name: capi-quickstart-controlplane-0
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: VSphereMachine
metadata:
  name: capi-quickstart-controlplane-0
spec:
  datacenter: "${VSPHERE_DATACENTER}"
  network:
    devices:
    - networkName: "${VSPHERE_NETWORK}"
      dhcp4: true
      dhcp6: false
  numCPUs: ${VSPHERE_NUM_CPUS}
  memoryMiB: ${VSPHERE_MEM_MIB}
  diskGiB: ${VSPHERE_DISK_GIB}
  template: "${VSPHERE_TEMPLATE}"
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
kind: KubeadmConfig
metadata:
  name: capi-quickstart-controlplane-0
spec:
  initConfiguration:
    nodeRegistration:
      name: "{{ ds.meta_data.hostname }}"
      criSocket: "/var/run/containerd/containerd.sock"
      kubeletExtraArgs:
        cloud-provider: vsphere
  clusterConfiguration:
    apiServer:
      extraArgs:
        cloud-provider: vsphere
        cloud-config: /etc/kubernetes/vsphere.conf
      extraVolumes:
      - name: "cloud-config"
        hostPath: /etc/kubernetes/vsphere.conf
        mountPath: /etc/kubernetes/vsphere.conf
        readOnly: true
        pathType: File
    controllerManager:
      extraArgs:
        cloud-provider: vsphere
        cloud-config: /etc/kubernetes/vsphere.conf
      extraVolumes:
      - name: "cloud-config"
        hostPath: /etc/kubernetes/vsphere.conf
        mountPath: /etc/kubernetes/vsphere.conf
        readOnly: true
        pathType: File
  files:
  - path: /etc/kubernetes/vsphere.conf
    owner: root:root
    permissions: "0600"
    encoding: base64
    content: |
      ${CLOUD_CONFIG_B64ENCODED}
  users:
  - name: capv
    sudo: "ALL=(ALL) NOPASSWD:ALL"
    sshAuthorizedKeys:
    - "${SSH_AUTHORIZED_KEY}"
  preKubeadmCommands:
  - hostname "{{ ds.meta_data.hostname }}"
  - echo "::1         ipv6-localhost ipv6-loopback" >/etc/hosts
  - echo "127.0.0.1   localhost {{ ds.meta_data.hostname }}" >>/etc/hosts
  - echo "{{ ds.meta_data.hostname }}" >/etc/hostname
```
{{#/tab }}
{{#/tabs }}


<!-- links -->
[kubectl]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[components]: ../reference/glossary.md#provider-components
[kind]: https://sigs.k8s.io/kind


