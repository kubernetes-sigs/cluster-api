# Machine Bootstrap Configuration

When creating a cluster managed by Cluster API, you have to define how new machines joining the cluster
should be bootstrapped.

This is achieved by defining one or more `BootstrapConfigTemplate` resources.

For example, if you are using the [kubeadm bootstrap provider].

```yaml
---
apiVersion: bootstrap.cluster.x-k8s.io/v1beta2
kind: KubeadmConfigTemplate
metadata:
  name: my-cluster-workers
spec:
  template:
    spec:
      joinConfiguration:
        nodeRegistration:
          criSocket: /var/run/containerd/containerd.sock
          kubeletExtraArgs:
          - name: cloud-provider
            value: external
      users:
      - name: my-cluster-admin
        sshAuthorizedKeys:
        - '...'
        sudo: ALL=(ALL) NOPASSWD:ALL
```

See documentation for the specific bootstrap provider for more details.

> [!IMPORTANT]
> Depending on the bootstrap provider you are planning to use, some additional prerequisites should be satisfied
> before configuring a cluster with Cluster API.
> See [Required configuration for common providers] for more details.

> [!TIP]
> If you are using [ClusterClass], the [ClusterClass] author will take ownership of the responsibility of defining
> `BootstrapConfigTemplate` resources, hiding all the complexity.
>
> [ClusterClass] users can configure machine bootstrap options directly from the Cluster object. See [Cluster Creation].

[Required configuration for common providers]: ../../../quick-start.md#required-configuration-for-common-providers
[ClusterClass]: ../cluster-class/index.md
[Cluster Creation]: ../create-cluster/index.md
[kubeadm bootstrap provider]: ./kubeadm-bootstrap/index.md
