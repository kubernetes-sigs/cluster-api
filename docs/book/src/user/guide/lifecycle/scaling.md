# Scale Replicas

In order to scale up or down the number of Machine replicas in a Cluster, you have to edit the resource controlling those
replicas, which might be one of the `ControlPlane`, `MachineDeployment` or `MachinePool` resources
belonging to a Cluster.

> [!TIP]
> If you are using [ClusterClass], both control plane and worker replicas can be managed from the `Cluster` resource.
> Cluster API will take care of propagating changes to the underlying `ControlPlane`, `MachineDeployment` or `MachinePool` automatically.
>
> See [Using ClusterClass](#using-clusterclass).

## Scaling Control Plane Replicas

In order to scale up or down the number of control plane Machine replicas in a Cluster you have to edit the `ControlPlane` resource.

E.g. If you are using the [kubeadm control plane provider].

```diff
---
apiVersion: controlplane.cluster.x-k8s.io/v1beta2
kind: KubeadmControlPlane
metadata:
  name: my-cluster-controlplane
spec:
  ...
+ replicas: 5
- replicas: 3
  version: v1.34.0
```

> [!TIP]
> You can also use `kubectl scale KubeadmControlPlane my-cluster-controlplane --replicas=5`.

If you need to prioritize which Machines get deleted during scale-down, add the `cluster.x-k8s.io/delete-machine` label to the Machine.

## Scaling Worker Replicas

In order to scale up or down the number of worker Machine replicas in a Cluster you have to edit the `MachineDeployment` or `MachinePool` resources.

E.g. If you are using the `MachineDeployment`.

```diff
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: MachineDeployment
metadata:
  name: my-cluster-md-0
spec:
  clusterName: my-cluster
+ replicas: 10
- replicas: 5
  template:
    ...
```

> [!TIP]
> You can also use `kubectl scale MachineDeployment my-cluster-md-0 --replicas=10`.

If you need to prioritize which Machines get deleted during scale-down, add the `cluster.x-k8s.io/delete-machine` label to the Machine.

> [!NOTE]
> The label only affects MachineSet scale-down; in a MachineDeployment, the choice of MachineSet to scale-down may bypass labeled Machines.

## Using ClusterClass

If you are using [ClusterClass], both control plane and worker replicas can be managed from the `Cluster` resource

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
+     replicas: 5
-     replicas: 3
    workers:
      machineDeployments:
      - class: default-worker
        name: md-0
+       replicas: 10
-       replicas: 5
```

Cluster API will take care of propagating changes to the underlying `ControlPlane`, `MachineDeployment` or `MachinePool` automatically.

> [!NOTE]
> If a cluster is using a [ClusterClass], direct changes to the replicas number in the `ControlPlane`, `MachineDeployment` or `MachinePool`
> resources will be overridden with the corresponding replica number from the `Cluster` resource.

> [!NOTE]
> `kubectl scale` cannot be used on `Cluster` using a [ClusterClass] (because it has multiple replica fields and it is not possible to target a specific one).

[ClusterClass]: ../configuration/cluster-class/index.md
[kubeadm control plane provider]: ../configuration/control-plane/kubeadm-control-plane/index.md
