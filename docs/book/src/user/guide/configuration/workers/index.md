# Workers Configuration

When creating a Cluster managed by Cluster API, you have to define how to manage your worker machines, the
machines where Kubernetes nodes and your workload will be hosted.

Instead of managing each worker machine individually one by one, it is a common practice to manage groups of
worker machines with a similar configuration by using `MachineDeployment` or `MachinePool` resources.

For example, if you are using the `MachineDeployment` resource (see [MachinePools] for the `MachinePool` alternative).

```yaml
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: MachineDeployment
metadata:
  name: my-cluster-md-0
spec:
  clusterName: my-cluster
  replicas: 5
  template:
    spec:
      bootstrap:
        configRef:
          apiGroup: bootstrap.cluster.x-k8s.io
          kind: KubeadmConfigTemplate
          name: my-cluster-workers
      deletion:
        nodeDeletionTimeoutSeconds: 30
      infrastructureRef:
        apiGroup: infrastructure.cluster.x-k8s.io
        kind: VSphereMachineTemplate
        name: my-cluster-worker-machine
      version: v1.34.0
```

> [!TIP]
> [ClusterClass] users can configure worker options (e.g. replicas and variables) directly from the Cluster object using `spec.topology.workers`. See [Cluster Creation].

[MachinePools]: ./machine-pools.md
[ClusterClass]: ../cluster-class/index.md
[Cluster Creation]: ../create-cluster/index.md
