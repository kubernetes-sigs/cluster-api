# Infrastructure Configuration

When creating a Cluster managed by Cluster API, you have to define how this cluster will be hosted in the target
infrastructure.

This is achieved by defining an `InfrastructureCluster` resource and one or more `InfrastructureMachineTemplate` resources.

For example, if your target infrastructure is vSphere.

```yaml
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: VSphereCluster
metadata:
  name: my-cluster-infrastructure
spec:
  controlPlaneEndpoint:
    host: 10.1.47.253
    port: 6443
  identityRef:
    kind: Secret
    name: my-cluster-credentials
  server: prod
  thumbprint: ...
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: VSphereMachineTemplate
metadata:
  name: my-cluster-control-plane-machine-infrastructure
spec:
  template:
    spec:
      cloneMode: linkedClone
      datacenter: dc-1
      datastore: prod-vms
      diskGiB: 25
      folder: cluster-api
      memoryMiB: 8192
      network:
        devices:
          - dhcp4: true
            dhcp6: false
            networkName: network-1
      numCPUs: 2
      os: Linux
      powerOffMode: trySoft
      resourcePool: cluster-api
      server: prod
      storagePolicyName: ssd
      template: ubuntu
```

See documentation for the specific infrastructure provider for more details.

> [!IMPORTANT]
> Depending on the infrastructure provider you are planning to use, some additional prerequisites should be satisfied
> before configuring a cluster with Cluster API.
> See [Required configuration for common providers] for more details.

> [!TIP]
> If you are using [ClusterClass], the [ClusterClass] author will take ownership of the responsibility of defining
> `InfrastructureCluster` and `InfrastructureMachineTemplate` resources, hiding all the complexity.
>
> [ClusterClass] users can configure infrastructure directly from the Cluster object. See [Cluster Creation].

[Required configuration for common providers]: ../../../quick-start.md#required-configuration-for-common-providers
[ClusterClass]: ../cluster-class/index.md
[Cluster Creation]: ../create-cluster/index.md
