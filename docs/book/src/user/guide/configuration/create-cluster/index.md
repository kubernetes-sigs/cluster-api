# Create a Cluster

This section provides details about creating a new Cluster managed by Cluster API.

If you are not using the [ClusterClass], at this point you should already have:

- an `InfrastructureCluster` resource and one or more `InfrastructureMachineTemplate` resources
- a control plane resource (for example, a `KubeadmControlPlane`)
- one or more `BootstrapConfigTemplate` resources
- one or more `MachineDeployment` or `MachinePool` resources

All you need now is a `Cluster` resource that ties everything together:

```yaml
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
      - 192.168.0.0/16
  controlPlaneRef:
    apiGroup: controlplane.cluster.x-k8s.io
    kind: KubeadmControlPlane
    name: my-cluster-controlplane
  infrastructureRef:
    apiGroup: infrastructure.cluster.x-k8s.io
    kind: VSphereCluster
    name: my-cluster-infrastructure
```

Once this is ready, you can apply your manifest, including the `Cluster` and all the other resources, and Cluster API will orchestrate 
infrastructure provisioning, Machine bootstrap as well as bootstrapping the Kubernetes cluster hosted on this infrastructure/machines.

> [!TIP]
> The [clusterctl generate cluster] command can help you in
> generating the `Cluster` and all the other resources starting from templates available for each infrastructure provider.

## Using ClusterClass

If you are using the [ClusterClass] feature, the [ClusterClass] author takes ownership of the responsibility of defining
`InfrastructureCluster`, `ControlPlane` and all the other resources described in the previous paragraphs.

As a consequence, all you need to create a cluster managed by Cluster API is to create a `Cluster` resource with
additional information defining the desired cluster topology.

e.g.

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
        - 192.168.0.0/16
  topology:
    classRef:
      name: vsphere-clusterclass-v0.1.0
    version: v1.34.0
    controlPlane:
      replicas: 3
    workers:
      machineDeployments:
      - class: default-worker
        name: md-0
        replicas: 5
```

> [!NOTE]
> When using [ClusterClass], infrastructure settings or machine bootstrap settings are exposed through
> variables defined by the [ClusterClass] author.

Cluster API will take care of creating the underlying `ControlPlane`, `MachineDeployment` or `MachinePool` resources
corresponding `BootstrapConfigTemplate` and `InfrastructureMachineTemplate` automatically.

> [!TIP]
> The [clusterctl generate cluster] command can also generate `Cluster` using [ClusterClass] (it depends
> on how templates available for each infrastructure provider are implemented).

> [!TIP]
> The `Cluster.spec.topology` field not only provides a very efficient way for defining a cluster configuration,
> but it also acts as a single point of control for the entire lifecycle of the cluster.

[ClusterClass]: ../cluster-class/index.md
[clusterctl generate cluster]: ../../../../reference/clusterctl/commands/generate-cluster.md

