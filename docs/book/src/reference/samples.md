```
apiVersion: cluster.x-k8s.io/v1alpha4
kind: Cluster
metadata:
  name: capi-quickstart
  namespace: default
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
      - 192.168.0.0/16
    serviceDomain: cluster.local
    services:
      cidrBlocks:
      - 10.128.0.0/12
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1alpha4
    kind: KubeadmControlPlane
    name: capi-quickstart-control-plane
    namespace: default
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
    kind: DockerCluster
    name: capi-quickstart
    namespace: default
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: DockerCluster
metadata:
  name: capi-quickstart
  namespace: default
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: DockerMachineTemplate
metadata:
  name: capi-quickstart-control-plane
  namespace: default
spec:
  template:
    spec:
      extraMounts:
      - containerPath: /var/run/docker.sock
        hostPath: /var/run/docker.sock
---
apiVersion: controlplane.cluster.x-k8s.io/v1alpha4
kind: KubeadmControlPlane
metadata:
  name: capi-quickstart-control-plane
  namespace: default
spec:
  kubeadmConfigSpec:
    clusterConfiguration:
      apiServer:
        certSANs:
        - localhost
        - 127.0.0.1
      controllerManager:
        extraArgs:
          enable-hostpath-provisioner: "true"
    initConfiguration:
      nodeRegistration:
        criSocket: /var/run/containerd/containerd.sock
        kubeletExtraArgs:
          cgroup-driver: cgroupfs
          eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
    joinConfiguration:
      nodeRegistration:
        criSocket: /var/run/containerd/containerd.sock
        kubeletExtraArgs:
          cgroup-driver: cgroupfs
          eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
  machineTemplate:
    infrastructureRef:
      apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
      kind: DockerMachineTemplate
      name: capi-quickstart-control-plane
      namespace: default
  replicas: 3
  version: v1.21.2
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: DockerMachineTemplate
metadata:
  name: capi-quickstart-md-0
  namespace: default
spec:
  template:
    spec: {}
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha4
kind: KubeadmConfigTemplate
metadata:
  name: capi-quickstart-md-0
  namespace: default
spec:
  template:
    spec:
      joinConfiguration:
        nodeRegistration:
          kubeletExtraArgs:
            cgroup-driver: cgroupfs
            eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
---
apiVersion: cluster.x-k8s.io/v1alpha4
kind: MachineDeployment
metadata:
  name: capi-quickstart-md-0
  namespace: default
spec:
  clusterName: capi-quickstart
  replicas: 3
  selector:
    matchLabels: null
  template:
    spec:
      bootstrap:
        configRef:
          apiVersion: bootstrap.cluster.x-k8s.io/v1alpha4
          kind: KubeadmConfigTemplate
          name: capi-quickstart-md-0
          namespace: default
      clusterName: capi-quickstart
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
        kind: DockerMachineTemplate
        name: capi-quickstart-md-0
        namespace: default
      version: v1.21.2```