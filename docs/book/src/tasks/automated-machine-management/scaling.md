# Scaling Nodes

This section applies only to worker Machines. You can add or remove compute capacity for your cluster workloads by creating or removing Machines. A Machine expresses intent to have a Node with a defined form factor.

Machines can be owned by scalable resources i.e. MachineSet and MachineDeployments.

You can scale MachineSets and MachineDeployments in or out by expressing intent via `.spec.replicas` or updating the scale subresource e.g `kubectl scale machinedeployment foo --replicas=5`.

If you need to prioritize which Machines get deleted during scale-down, add the `cluster.x-k8s.io/delete-machine` label to the Machine. KCP or a MachineSet will delete labeled control plane or worker Machines first, and this label has top priority over all delete policies.

**Note**: The label only affects MachineSet scale-down; in a MachineDeployment, the choice of MachineSet to scale-down may bypass labeled Machines.

When you delete a Machine directly or by scaling down, the same process takes place in the same order:
- The Node backed by that Machine will try to be drained indefinitely and will wait for any volume to be detached from the Node unless you specify a `.spec.nodeDrainTimeout`.
  - CAPI uses default [kubectl draining implementation](https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/) with `-–ignore-daemonsets=true`. If you needed to ensure DaemonSets eviction you'd need to do so manually by also adding proper taints to avoid rescheduling.
- The infrastructure backing that Node will try to be deleted indefinitely.
- Only when the infrastructure is gone, the Node will try to be deleted indefinitely unless you specify `.spec.nodeDeletionTimeout`.
