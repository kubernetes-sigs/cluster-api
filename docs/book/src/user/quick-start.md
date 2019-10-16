# Quick Start

In this tutorial we'll cover the basics of how to use Cluster API to create one or more Kubernetes clusters.

{{#include ../tasks/installation.md}}

## Usage

Now that we've got Cluster API, Bootstrap and Infrastructure resources installed,
let's proceed to create a single node cluster.

For the purpose of this tutorial, we'll name our cluster `capi-quickstart`.

{{#tabs name:"tab-usage-cluster-resource" tabs:"AWS,Docker,vSphere"}}
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
{{#tab Docker}}

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
    kind: DockerCluster
    name: capi-quickstart
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: DockerCluster
metadata:
  name: capi-quickstart
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

{{#tabs name:"tab-usage-controlplane-resource" tabs:"AWS,Docker,vSphere"}}
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
{{#tab Docker}}

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
    kind: DockerMachine
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
    name: capi-quickstart-controlplane-0
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: DockerMachine
metadata:
  name: capi-quickstart-controlplane-0
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
kind: KubeadmConfig
metadata:
  name: capi-quickstart-controlplane-0
spec:
  initConfiguration:
    nodeRegistration:
      kubeletExtraArgs:
        # Default thresholds are higher to provide a buffer before resources
        # are completely depleted, at the cost of requiring more total
        # resources. These low thresholds allow running with fewer resources.
        # Appropriate for testing or development only.
        eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
  clusterConfiguration:
    controllerManager:
      extraArgs:
        # Enables dynamic storage provisioning without a cloud provider.
        # Appropriate for testing or development only.
        enable-hostpath-provisioner: "true"
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

<aside class="note warning">
<h1>Action Required (Docker provider v0.2.0 only)</h1>

A [known issue](https://github.com/kubernetes-sigs/kind/issues/891) affects Calico with the Docker provider v0.2.0. After you deploy Calico, apply this patch to work around the issue:

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig \
  -n kube-system patch daemonset calico-node \
  --type=strategic --patch='
spec:
  template:
    spec:
      containers:
      - name: calico-node
        env:
        - name: FELIX_IGNORELOOSERPF
          value: "true"
'
```
</aside>
{{#/tab }}
{{#/tabs }}

After a short while, our control plane should be up and in `Ready` state,
let's check the status using `kubectl get nodes`:

```bash
kubectl --kubeconfig=./capi-quickstart.kubeconfig get nodes
```

Finishing up, we'll create a single node _MachineDeployment_.

{{#tabs name:"tab-usage-machinedeployment" tabs:"AWS,Docker,vSphere"}}
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
{{#tab Docker}}

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
        kind: DockerMachineTemplate
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: DockerMachineTemplate
metadata:
  name: capi-quickstart-worker
spec:
  template:
    spec: {}
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
          kubeletExtraArgs:
            eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
      clusterConfiguration:
        controllerManager:
          extraArgs:
            enable-hostpath-provisioner: "true"
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
[target cluster]: ../reference/glossary.md#target-cluster
