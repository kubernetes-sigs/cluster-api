# Quick Start

In this tutorial we'll cover the basics of how to use Cluster API to create one or more Kubernetes clusters.

## Prerequisites

- Install and setup [kubectl] in your local environment.
- A Kubernetes cluster to be used as [management cluster].
  For the purpose of this quick start we suggest you to use [kind] as a _non production-ready_ solution.
  ```bash
  kind create cluster --name=clusterapi
  export KUBECONFIG="$(kind get kubeconfig-path --name="clusterapi")"
  ```

## Installation

Using [kubectl], let's create the components on the [management cluster]:

#### Install Cluster API Components

```bash
kubectl create -f {{#releaselink gomodule:"sigs.k8s.io/cluster-api" asset:"cluster-api-components.yaml" version:"0.2.x"}}
```

#### Install the Bootstrap Provider Components

{{#tabs name:"tab-installation-bootstrap" tabs:"Kubeadm"}}
{{#tab Kubeadm}}

Check the [Kubeadm provider releases](https://github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm/releases) for an up-to-date components file.

```bash
kubectl create -f {{#releaselink gomodule:"sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm" asset:"bootstrap-components.yaml" version:"0.1.x"}}
```

{{#/tab }}
{{#/tabs }}


#### Install Infrastructure Provider Components

{{#tabs name:"tab-installation-infrastructure" tabs:"AWS,vSphere"}}
{{#tab AWS}}

<aside class="note warning">

<h1>Action Required</h1>

For more information about credentials management, IAM, or requirements for AWS, visit the [AWS Provider Prerequisites](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/master/docs/prerequisites.md) document.

</aside>

#### Install clusterawsadm

Download the latest binary of `clusterawsadm` from the [AWS provider releases] and make sure to place it in your path.

##### Create the components

Check the [AWS provider releases] for an up-to-date components file.

```bash
# Create the base64 encoded credentials using clusterawsadm.
# This command uses your environment variables and encodes
# them in a value to be stored in a Kubernetes Secret.
export AWS_B64ENCODED_CREDENTIALS=$(clusterawsadm alpha bootstrap encode-aws-credentials)

# Create the components.
curl -L {{#releaselink gomodule:"sigs.k8s.io/cluster-api-provider-aws" asset:"infrastructure-components.yaml" version:"0.4.x"}} \
  | envsubst \
  | kubectl create -f -
```

{{#/tab }}
{{#tab vSphere}}

Check the [vSphere provider releases](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/releases) for an up-to-date components file.

For more information about prerequisites, credentials management, or permissions for vSphere, visit the [getting started guide](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/docs/getting_started.md).

```bash
kubectl create -f {{#releaselink gomodule:"sigs.k8s.io/cluster-api-provider-vsphere" asset:"infrastructure-components.yaml" version:"0.5.x"}}
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
  # Change this value to the region you want to deploy the cluster in.
  region: us-east-1
  # Change this value to a valid SSH Key Pair present in your AWS Account.
  sshKeyName: default
```
{{#/tab }}
{{#tab vSphere}}

<aside class="note warning">

<h1>Action Required</h1>

These examples include environment variables that you should substitute before creating the resources.

</aside>

```yaml
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
  # This IAM profile is part of the pre-requisites.
  iamInstanceProfile: "controllers.cluster-api-provider-aws.sigs.k8s.io"
  # Change this value to a valid SSH Key Pair present in your AWS Account.
  sshKeyName: default
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
kind: KubeadmConfig
metadata:
  name: capi-quickstart-controlplane-0
spec:
  # For more information about these values,
  # refer to the Kubeadm Bootstrap Provider documentation.
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
  # For more information about these values,
  # refer to the Kubeadm Bootstrap Provider documentation.
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

After the controlplane is up and running, let's retrieve the [target cluster] Kubeconfig:

```bash
kubectl --namespace=default get secret/capi-quickstart-kubeconfig -o json \
  | jq -r .data.value \
  | base64 --decode \
  > ./capi-quickstart.kubeconfig
```

Deploy a CNI solution, Calico is used here as an example.

{{#tabs name:"tab-usage-addons" tabs:"Calico"}}
{{#tab Calico}}

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig \
  apply -f https://docs.projectcalico.org/v3.8/manifests/calico.yaml
```

{{#/tab }}
{{#/tabs }}

After a short while, our control plane should be up and in `Ready` state,
let's check the status using `kubectl get nodes`:

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig get nodes
```

Finishing up, we'll create a single node _MachineDeployment_.

{{#tabs name:"tab-usage-machinedeployment" tabs:"AWS,vSphere"}}
{{#tab AWS}}

```yaml
apiVersion: cluster.x-k8s.io/v1alpha2
kind: MachineDeployment
metadata:
  name: capi-quickstart-worker
  labels:
    cluster.x-k8s.io/cluster-name: capi-quickstart
    # Labels beyond this point are for example purposes,
    # feel free to add more or change with something more meaningful.
    # Sync these values with spec.selector.matchLabels and spec.template.metadata.labels.
    nodepool: nodepool-0
spec:
  replicas: 1
  selector:
    matchLabels:
      cluster.x-k8s.io/cluster-name: capi-quickstart
      nodepool: nodepool-0
  template:
    metadata:
      labels:
        cluster.x-k8s.io/cluster-name: capi-quickstart
        nodepool: nodepool-0
    spec:
      version: v1.15.3
      bootstrap:
        configRef:
          name: capi-quickstart-worker
          apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
          kind: KubeadmConfigTemplate
      infrastructureRef:
        name: capi-quickstart-worker
        apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
        kind: AWSMachineTemplate
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: AWSMachineTemplate
metadata:
  name: capi-quickstart-worker
spec:
  template:
    spec:
      instanceType: t3.large
      # This IAM profile is part of the pre-requisites.
      iamInstanceProfile: "nodes.cluster-api-provider-aws.sigs.k8s.io"
      # Change this value to a valid SSH Key Pair present in your AWS Account.
      sshKeyName: default
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
kind: KubeadmConfigTemplate
metadata:
  name: capi-quickstart-worker
spec:
  template:
    spec:
      # For more information about these values,
      # refer to the Kubeadm Bootstrap Provider documentation.
      joinConfiguration:
        nodeRegistration:
          name: '{{ ds.meta_data.hostname }}'
          kubeletExtraArgs:
            cloud-provider: aws
```

{{#/tab }}
{{#tab vSphere}}

<aside class="note warning">

<h1>Action Required</h1>

These examples include environment variables that you should substitute before creating the resources.

</aside>

```yaml
apiVersion: cluster.x-k8s.io/v1alpha2
kind: MachineDeployment
metadata:
  name: capi-quickstart-worker
  labels:
    cluster.x-k8s.io/cluster-name: capi-quickstart
    # Labels beyond this point are for example purposes,
    # feel free to add more or change with something more meaningful.
    # Sync these values with spec.selector.matchLabels and spec.template.metadata.labels.
    nodepool: nodepool-0
spec:
  replicas: 1
  selector:
    matchLabels:
      cluster.x-k8s.io/cluster-name: capi-quickstart
      nodepool: nodepool-0
  template:
    metadata:
      labels:
        cluster.x-k8s.io/cluster-name: capi-quickstart
        nodepool: nodepool-0
    spec:
      version: v1.15.3
      bootstrap:
        configRef:
          apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
          kind: KubeadmConfigTemplate
          name: capi-quickstart-worker
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
        kind: VSphereMachineTemplate
        name: capi-quickstart-worker
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: VSphereMachineTemplate
metadata:
  name: capi-quickstart-worker
spec:
  template:
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
kind: KubeadmConfigTemplate
metadata:
  name: capi-quickstart-worker
spec:
  template:
    spec:
      # For more information about these values,
      # refer to the Kubeadm Bootstrap Provider documentation.
      joinConfiguration:
        nodeRegistration:
          name: "{{ ds.meta_data.hostname }}"
          criSocket: "/var/run/containerd/containerd.sock"
          kubeletExtraArgs:
            cloud-provider: vsphere
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
[management cluster]: ../reference/glossary.md#management-cluster
[target cluster]: ../reference/glossary.md#target-cluster
[AWS provider releases]: https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases

