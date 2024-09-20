# MachineSet

A MachineSet is an abstraction over Machines.

Its main responsibilities are:
* Adopting unowned Machines that aren't assigned to a MachineSet
* Adopting unmanaged Machines that aren't assigned a Cluster
* Booting a group of N machines
  * Monitoring the status of those booted machines

![](../../../images/cluster-admission-machineset-controller.png)

## In-place propagation
Changes to the following fields of MachineSet are propagated in-place to the Machine without needing a full rollout:
- `.spec.template.metadata.labels`
- `.spec.template.metadata.annotations`
- `.spec.template.spec.nodeDrainTimeout`
- `.spec.template.spec.nodeDeletionTimeout`
- `.spec.template.spec.nodeVolumeDetachTimeout`

Changes to the following fields of MachineSet are propagated in-place to the InfrastructureMachine and BootstrapConfig:
- `.spec.template.metadata.labels`
- `.spec.template.metadata.annotations`

Note: Changes to these fields will not be propagated to Machines that are marked for deletion (example: because of scale down).
