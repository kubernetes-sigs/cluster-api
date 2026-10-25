# Control Plane Configuration

When creating a cluster managed by Cluster API, you have to define options for managing the cluster's control plane.

This is achieved by defining a control plane resource (for example, a `KubeadmControlPlane`).

For example, if you are using the [kubeadm control plane provider].

```yaml
---
apiVersion: controlplane.cluster.x-k8s.io/v1beta2
kind: KubeadmControlPlane
metadata:
  name: my-cluster-controlplane
spec:
  kubeadmConfigSpec:
    clusterConfiguration:
      controllerManager:
        extraArgs:
          - name: cloud-provider
            value: external
    initConfiguration:
      nodeRegistration:
        criSocket: /var/run/containerd/containerd.sock
        kubeletExtraArgs:
          - name: cloud-provider
            value: external
        name: '{{ local_hostname }}'
    joinConfiguration:
      nodeRegistration:
        criSocket: /var/run/containerd/containerd.sock
        kubeletExtraArgs:
          - name: cloud-provider
            value: external
        name: '{{ local_hostname }}'
    users:
      - name: my-cluster-admin
        sshAuthorizedKeys:
          - '...'
        sudo: ALL=(ALL) NOPASSWD:ALL
  machineTemplate:
    spec:
      deletion:
        nodeDeletionTimeoutSeconds: 0
      infrastructureRef:
        apiGroup: infrastructure.cluster.x-k8s.io
        kind: VSphereMachineTemplate
        name: my-cluster-control-plane-machine-infrastructure
  replicas: 3
  version: v1.34.0
```

See documentation for the specific control plane provider for more details.

> [!IMPORTANT]
> Depending on the control plane provider you are planning to use, some additional prerequisites should be satisfied
> before configuring a cluster with Cluster API.
> See [Required configuration for common providers] for more details.

> [!TIP]
> If you are using [ClusterClass], the [ClusterClass] author will take ownership of the responsibility of defining
> control plane resources, hiding all the complexity.
>
> [ClusterClass] users can configure control plane options directly from the Cluster object. See [Cluster Creation].

[Required configuration for common providers]: ../../../quick-start.md#required-configuration-for-common-providers
[ClusterClass]: ../cluster-class/index.md
[Cluster Creation]: ../create-cluster/index.md
[kubeadm control plane provider]: ./kubeadm-control-plane/index.md
