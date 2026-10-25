# Using the Cluster Autoscaler

Cluster Autoscaler is a tool that automatically adjusts the size of the Kubernetes cluster based
on the utilization of Pods and Nodes in your cluster. For more general information about the
Cluster Autoscaler, please see the [project documentation](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler).

Cluster Autoscaler can also work with Cluster API, and when active it controls the number of replicas
of `MachineDeployment` like a user does when manually [Scaling Replicas].

In order to inform the autoscaler that it should manage replicas for a `MachineDeployment` resource,
the autoscaler-specific annotations should be set on the resource, e.g.:

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: MachineDeployment
metadata:
  annotations:
    cluster.x-k8s.io/cluster-api-autoscaler-node-group-max-size: "5"
    cluster.x-k8s.io/cluster-api-autoscaler-node-group-min-size: "0"
```

> [!TIP]
> If you are using [ClusterClass], autoscaler configuration can be managed from the `Cluster` resource.
>
> See [Using ClusterClass](#using-clusterclass).

See the following [Autoscaler project documentation](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/cloudprovider/clusterapi) for more details about how the autoscaler works.

{{#embed-github repo:"kubernetes/autoscaler" path:"cluster-autoscaler/cloudprovider/clusterapi/README.md" }}

> [!NOTE]
> **Defaulting of the MachineDeployment, MachineSet replicas field**
>
> Please note that the MachineDeployment and MachineSet replicas field has special defaulting logic to provide a smooth integration with the autoscaler.
> The replica field is defaulted based on the autoscaler min and max size annotations. The goal is to pick a default value which is inside
> the [min size, max size] range so the autoscaler can take control of the replicas field.
>
> The defaulting logic is as follows:
>
> * if the autoscaler min size and max size annotations are set:
>   * if it's a new MachineDeployment or MachineSet, use min size
>   * if the replicas field of the old MachineDeployment or MachineSet is < min size, use min size
>   * if the replicas field of the old MachineDeployment or MachineSet is > max size, use max size
>   * if the replicas field of the old MachineDeployment or MachineSet is in the [min size, max size] range, keep the value from the oldMD or oldMS
> * otherwise, use 1

## Using ClusterClass

If you are using [ClusterClass], autoscaler annotations can be managed from the `Cluster` resource.

```diff
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-docker-cluster
spec:
  ...
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
        metadata:
+         annotations:
+           cluster.x-k8s.io/cluster-api-autoscaler-node-group-max-size: "5"
+           cluster.x-k8s.io/cluster-api-autoscaler-node-group-min-size: "0"
```

> [!NOTE]
> Rules for Defaulting MachineDeployment replicas field described above also apply here.

> [!NOTE]
> If a cluster is using a [ClusterClass], direct changes to the metadata in the `MachineDeployment`
> resources will be overridden with the corresponding metadata from the `Cluster` resource.

[Scaling Replicas]: ./scaling.md
[ClusterClass]: ../configuration/cluster-class/index.md
