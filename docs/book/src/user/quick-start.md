# Quick Start

In this tutorial we'll cover the basics of how to use Cluster API to create one or more Kubernetes clusters.

{{#include ../tasks/installation.md}}

## Usage

Now that we've got Cluster API, Bootstrap and Infrastructure resources installed,
let's proceed to create a single node cluster.

For the purpose of this tutorial, we'll name our cluster `capi-quickstart`.

{{#tabs name:"tab-usage-cluster-resource" tabs:"AWS,Docker,vSphere,OpenStack"}}
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

This quick start assumes the following vSphere environment which you should replace based on your own environment.

| Property       | Value                    | Description                                                                                                                                                           |
|----------------|--------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| vCenter Server | 10.0.0.1                 | The IP address or fully-qualified domain name (FQDN) of the vCenter server                                                                                            |
| Datacenter     | SDDC-Datacenter          | The datacenter to which VMs will be deployed                                                                                                                          |
| Datastore      | DefaultDatastore         | The datastore to use for VMs                                                                                                                                          |
| Resource Pool  | '*/Resources'            | The resource pool in which the VMs will be located. Please note that when using an * character in part of the inventory path, the entire value must be single quoted. |
| VM Network     | vm-network-1             | The VM network to use for VMs                                                                                                                                         |
| VM Folder      | vm                       | The VM folder in which VMs will be located                                                                                                                            |
| VM Template    | ubuntu-1804-kube-v1.16.2 | The VM template to use for VMs                                                                                                                                        |

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
  cloudProviderConfiguration:
    global:
      insecure: true
      secretName: cloud-provider-vsphere-credentials
      secretNamespace: kube-system
    network:
      name: vm-network-1
    providerConfig:
      cloud:
        controllerImage: gcr.io/cloud-provider-vsphere/cpi/release/manager:v1.0.0
      storage:
        attacherImage: quay.io/k8scsi/csi-attacher:v1.1.1
        controllerImage: gcr.io/cloud-provider-vsphere/csi/release/driver:v1.0.1
        livenessProbeImage: quay.io/k8scsi/livenessprobe:v1.1.0
        metadataSyncerImage: gcr.io/cloud-provider-vsphere/csi/release/syncer:v1.0.1
        nodeDriverImage: gcr.io/cloud-provider-vsphere/csi/release/driver:v1.0.1
        provisionerImage: quay.io/k8scsi/csi-provisioner:v1.2.1
        registrarImage: quay.io/k8scsi/csi-node-driver-registrar:v1.1.0
    virtualCenter:
      10.0.0.1:
        datacenters: SDDC-Datacenter
    workspace:
      datacenter: SDDC-Datacenter
      datastore: DefaultDatastore
      folder: vm
      resourcePool: '*/Resources'
      server: 10.0.0.1
  server: 10.0.0.1
```
{{#/tab }}
{{#tab OpenStack}}

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
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"] # CIDR block used by Calico.
    serviceDomain: "cluster.local"
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
    kind: OpenStackCluster
    name: capi-quickstart
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: OpenStackCluster
metadata:
  name: capi-quickstart
spec:
  cloudName: ${OPENSTACK_CLOUD}
  cloudsSecret:
    name: cloud-config
  nodeCidr: ${NODE_CIDR}
  externalNetworkId: ${OPENSTACK_EXTERNAL_NETWORK_ID}
  disablePortSecurity: true
  disableServerTags: true
---
apiVersion: v1
kind: Secret
metadata:
  name: cloud-config
type: Opaque
data:
  # This file has to be in the regular OpenStack cloud.yaml format
  clouds.yaml: ${OPENSTACK_CLOUD_CONFIG_B64ENCODED}
  cacert: ${OPENSTACK_CLOUD_CACERT_B64ENCODED}
```
{{#/tab }}
{{#/tabs }}

Now that we've created the cluster object, we can create a control plane Machine.

{{#tabs name:"tab-usage-controlplane-resource" tabs:"AWS,Docker,vSphere,OpenStack"}}
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
  iamInstanceProfile: "control-plane.cluster-api-provider-aws.sigs.k8s.io"
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

This quick start assumes the following vSphere environment which you should replace based on your own environment:

| Property       | Value                    | Description                                                                                                                                                           |
|----------------|--------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| vCenter Server | 10.0.0.1                 | The IP address or fully-qualified domain name (FQDN) of the vCenter server                                                                                            |
| Datacenter     | SDDC-Datacenter          | The datacenter to which VMs will be deployed                                                                                                                          |
| Datastore      | DefaultDatastore         | The datastore to use for VMs                                                                                                                                          |
| Resource Pool  | '*/Resources'            | The resource pool in which the VMs will be located. Please note that when using an * character in part of the inventory path, the entire value must be single quoted. |
| VM Network     | vm-network-1             | The VM network to use for VMs                                                                                                                                         |
| VM Folder      | vm                       | The VM folder in which VMs will be located                                                                                                                            |
| VM Template    | ubuntu-1804-kube-v1.16.2 | The VM template to use for VMs                                                                                                                                        |

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
  version: v1.16.2
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
  labels:
    cluster.x-k8s.io/cluster-name: capi-quickstart
    cluster.x-k8s.io/control-plane: "true"
  name: capi-quickstart-controlplane-0
  namespace: default
spec:
  datacenter: SDDC-Datacenter
  diskGiB: 50
  memoryMiB: 2048
  network:
    devices:
    - dhcp4: true
      dhcp6: false
      networkName: vm-network-1
  numCPUs: 2
  template: ubuntu-1804-kube-v1.16.2
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
kind: KubeadmConfig
metadata:
  name: capi-quickstart-controlplane-0
  namespace: default
spec:
  clusterConfiguration:
    apiServer:
      extraArgs:
        cloud-provider: external
    controllerManager:
      extraArgs:
        cloud-provider: external
    imageRepository: k8s.gcr.io
  initConfiguration:
    nodeRegistration:
      criSocket: /var/run/containerd/containerd.sock
      kubeletExtraArgs:
        cloud-provider: external
      name: '{{ ds.meta_data.hostname }}'
  preKubeadmCommands:
  - hostname "{{ ds.meta_data.hostname }}"
  - echo "::1         ipv6-localhost ipv6-loopback" >/etc/hosts
  - echo "127.0.0.1   localhost {{ ds.meta_data.hostname }}" >>/etc/hosts
  - echo "{{ ds.meta_data.hostname }}" >/etc/hostname
```
{{#/tab }}
{{#tab OpenStack}}

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
    kind: OpenStackMachine
    name: capi-quickstart-controlplane-0
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: OpenStackMachine
metadata:
  name: capi-quickstart-controlplane-0
spec:
  flavor: m1.medium
  image: ${IMAGE_NAME}
  availabilityZone: nova
  floatingIP: ${FLOATING_IP}
  cloudName: ${OPENSTACK_CLOUD}
  cloudsSecret:
    name: cloud-config
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
kind: KubeadmConfig
metadata:
  name: capi-quickstart-controlplane-0
spec:
  # For more information about these values,
  # refer to the Kubeadm Bootstrap Provider documentation.
  initConfiguration:
    localAPIEndpoint:
      advertiseAddress: '{{ ds.ec2_metadata.local_ipv4 }}'
      bindPort: 6443
    nodeRegistration:
      name: '{{ local_hostname }}'
      criSocket: "/var/run/containerd/containerd.sock"
      kubeletExtraArgs:
        cloud-provider: openstack
        cloud-config: /etc/kubernetes/cloud.conf
  clusterConfiguration:
    controlPlaneEndpoint: "${FLOATING_IP}:6443"
    imageRepository: k8s.gcr.io
    apiServer:
      extraArgs:
        cloud-provider: openstack
        cloud-config: /etc/kubernetes/cloud.conf
      extraVolumes:
      - name: cloud
        hostPath: /etc/kubernetes/cloud.conf
        mountPath: /etc/kubernetes/cloud.conf
        readOnly: true
    controllerManager:
      extraArgs:
        cloud-provider: openstack
        cloud-config: /etc/kubernetes/cloud.conf
      extraVolumes:
      - name: cloud
        hostPath: /etc/kubernetes/cloud.conf
        mountPath: /etc/kubernetes/cloud.conf
        readOnly: true
      - name: cacerts
        hostPath: /etc/certs/cacert
        mountPath: /etc/certs/cacert
        readOnly: true
  files:
  - path: /etc/kubernetes/cloud.conf
    owner: root
    permissions: "0600"
    encoding: base64
    # This file has to be in the format of the
    # OpenStack cloud provider 
    content: |-
      ${OPENSTACK_CLOUD_CONFIG_B64ENCODED}
  - path: /etc/certs/cacert
    owner: root
    permissions: "0600"
    content: |
      ${OPENSTACK_CLOUD_CACERT_B64ENCODED}
  users:
  - name: capo
    sudo: "ALL=(ALL) NOPASSWD:ALL"
    sshAuthorizedKeys:
    - "${SSH_AUTHORIZED_KEY}"
```
{{#/tab }}
{{#/tabs }}

After the controlplane is up and running, let's retrieve the [target cluster] Kubeconfig:

{{#tabs name:"tab-getting-kubeconfig" tabs:"AWS,Docker,vSphere"}}
{{#tab AWS}}

```bash
kubectl --namespace=default get secret/capi-quickstart-kubeconfig -o json \
  | jq -r .data.value \
  | base64 --decode \
  > ./capi-quickstart.kubeconfig
```
{{#/tab }}
{{#tab Docker}}

```bash
kubectl --namespace=default get secret/capi-quickstart-kubeconfig -o json \
  | jq -r .data.value \
  | base64 --decode \
  > ./capi-quickstart.kubeconfig
```

When using docker-for-mac MacOS, you will need to do a couple of additional
steps to get the correct kubeconfig:

```bash
# Point the kubeconfig to the exposed port of the load balancer, rather than the inaccessible container IP.
sed -i -e "s/server:.*/server: https:\/\/$(docker port capi-quickstart-lb 6443/tcp | sed "s/0.0.0.0/127.0.0.1/")/g" ./capi-quickstart.kubeconfig

# Ignore the CA, because it is not signed for 127.0.0.1
sed -i -e "s/certificate-authority-data:.*/insecure-skip-tls-verify: true/g" ./capi-quickstart.kubeconfig
```  
{{#/tab }}
{{#tab vSphere}}

```bash
kubectl --namespace=default get secret/capi-quickstart-kubeconfig -o json \
  | jq -r .data.value \
  | base64 --decode \
  > ./capi-quickstart.kubeconfig
```
{{#/tab }}
{{#/tabs }}

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

{{#tabs name:"tab-usage-machinedeployment" tabs:"AWS,Docker,vSphere,OpenStack"}}
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

This quick start assumes the following vSphere environment which you should replace based on your own environment:

| Property       | Value                    | Description                                                                                                                                                           |
|----------------|--------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| vCenter Server | 10.0.0.1                 | The IP address or fully-qualified domain name (FQDN) of the vCenter server                                                                                            |
| Datacenter     | SDDC-Datacenter          | The datacenter to which VMs will be deployed                                                                                                                          |
| Datastore      | DefaultDatastore         | The datastore to use for VMs                                                                                                                                          |
| Resource Pool  | '*/Resources'            | The resource pool in which the VMs will be located. Please note that when using an * character in part of the inventory path, the entire value must be single quoted. |
| VM Network     | vm-network-1             | The VM network to use for VMs                                                                                                                                         |
| VM Folder      | vm                       | The VM folder in which VMs will be located                                                                                                                            |
| VM Template    | ubuntu-1804-kube-v1.16.2 | The VM template to use for VMs                                                                                                                                        |

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
      version: v1.16.2
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
  name: capi-quickstart-md-0
  namespace: default
spec:
  template:
    spec:
      datacenter: SDDC-Datacenter
      diskGiB: 50
      memoryMiB: 2048
      network:
        devices:
        - dhcp4: true
          dhcp6: false
          networkName: vm-network-1
      numCPUs: 2
      template: ubuntu-1804-kube-v1.16.2
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
kind: KubeadmConfigTemplate
metadata:
  name: capi-quickstart-md-0
  namespace: default
spec:
  template:
    spec:
      joinConfiguration:
        nodeRegistration:
          criSocket: /var/run/containerd/containerd.sock
          kubeletExtraArgs:
            cloud-provider: external
          name: '{{ ds.meta_data.hostname }}'
      preKubeadmCommands:
      - hostname "{{ ds.meta_data.hostname }}"
      - echo "::1         ipv6-localhost ipv6-loopback" >/etc/hosts
      - echo "127.0.0.1   localhost {{ ds.meta_data.hostname }}" >>/etc/hosts
      - echo "{{ ds.meta_data.hostname }}" >/etc/hostname
```

{{#/tab }}
{{#tab OpenStack}}

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
        kind: OpenStackMachineTemplate
        name: capi-quickstart-worker
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: OpenStackMachineTemplate
metadata:
  name: capi-quickstart-worker
spec:
  template:
    spec:
      availabilityZone: nova
      cloudName: ${OPENSTACK_CLOUD}
      cloudsSecret:
        name: cloud-config
      flavor: m1.medium
      image: ${IMAGE_NAME}
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
          name: '{{ local_hostname }}'
          criSocket: "/var/run/containerd/containerd.sock"
          kubeletExtraArgs:
            cloud-config: /etc/kubernetes/cloud.conf
            cloud-provider: openstack
      files:
      - path: /etc/kubernetes/cloud.conf
        owner: root
        permissions: "0600"
        encoding: base64
        # This file has to be in the format of the
        # OpenStack cloud provider 
        content: |-
          ${OPENSTACK_CLOUD_CONFIG_B64ENCODED}
      - path: /etc/certs/cacert
        owner: root
        permissions: "0600"
        content: |
          ${OPENSTACK_CLOUD_CACERT_B64ENCODED}
      users:
      - name: capo
        sudo: "ALL=(ALL) NOPASSWD:ALL"
        sshAuthorizedKeys:
        - "${SSH_AUTHORIZED_KEY}"
```

{{#/tab }}
{{#/tabs }}

<!-- links -->
[target cluster]: ../reference/glossary.md#target-cluster
