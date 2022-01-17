# Upgrade a Cluster using Managed Topology

The `spec.topology` field added to the Cluster object as part of ClusterClass allows changes made on the Cluster to be propagated across all relevant objects. This turns a Kubernetes cluster upgrade into a one-touch operation.
Let's assume we have created a CAPD cluster with ClusterClass and Kubernetes v1.21.2 (as documented in the [Quick Start guide]). Looking at the cluster, the version of the control plane and the MachineDeployments is v1.21.2.

```bash
> kubectl get kubeadmcontrolplane,machinedeployments

NAME                                                                              CLUSTER                   INITIALIZED   API SERVER AVAILABLE   REPLICAS   READY   UPDATED   UNAVAILABLE   AGE     VERSION
kubeadmcontrolplane.controlplane.cluster.x-k8s.io/clusterclass-quickstart-XXXX    clusterclass-quickstart   true          true                   1          1       1         0             2m21s   v1.21.2

NAME                                                                             CLUSTER                   REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE     AGE     VERSION
machinedeployment.cluster.x-k8s.io/clusterclass-quickstart-linux-workers-XXXX    clusterclass-quickstart   1          1       1         0             Running   2m21s   v1.21.2
```

To update the Cluster the only change needed is to the `version` field under `spec.topology` in the Cluster object.


Change `1.21.2` to `1.22.0` as below.

```bash
kubectl patch cluster clusterclass-quickstart --type json --patch '[{"op": "replace", "path": "/spec/topology/version", "value": "v1.22.0"}]'
```
**Important Note**: A +2 minor Kubernetes version upgrade is not allowed in Cluster Topologies. This is to align with existing control plane providers, like KubeadmControlPlane provider, that limit a +2 minor version upgrade. Example: Upgrading from `1.21.2` to `1.23.0` is not allowed.

The upgrade will take some time to roll out as it will take place machine by machine with older versions of the machines only being removed after healthy newer versions come online.

To watch the update progress run:

```bash
watch kubectl get kubeadmcontrolplane,machinedeployments
```

After a few minutes the upgrade will be complete and the output will be similar to:

```bash
NAME                                                                              CLUSTER                   INITIALIZED   API SERVER AVAILABLE   REPLICAS   READY   UPDATED   UNAVAILABLE   AGE     VERSION
kubeadmcontrolplane.controlplane.cluster.x-k8s.io/clusterclass-quickstart-XXXX    clusterclass-quickstart   true          true                   1          1       1         0             7m29s   v1.22.0

NAME                                                                             CLUSTER                   REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE     AGE     VERSION
machinedeployment.cluster.x-k8s.io/clusterclass-quickstart-linux-workers-XXXX    clusterclass-quickstart   1          1       1         0             Running   7m29s   v1.22.0
```
