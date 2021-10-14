# Metadata propagation
Cluster API controllers implement consistent metadata (labels & annotations) propagation across the core API resources.
This behaviour tries to be consistent with kubernetes apps/v1 Deployment and ReplicaSet.
New providers should behave accordingly fitting within the following pattern: 

## KubeadmControlPlane
Top-level labels and annotations do not propagate at all.
- `.labels` => Not propagated.
- `.annotations` => Not propagated.

MachineTemplate labels and annotations propagate to Machines, InfraMachines and BootstrapConfigs.
- `.spec.machineTemplate.metadata.labels` => `Machine.labels`, `InfraMachine.labels`, `BootstrapConfig.labels`
- `.spec.machineTemplate.metadata.annotations` => `Machine.annotations`, `InfraMachine.annotations`, `BootstrapConfig.annotations`

## MachineDeployment
Top-level labels do not propagate at all.
Top-level annotations propagate to MachineSets top-level annotations.
- `.labels` => Not propagated.
- `.annotations` => MachineSet.annotations

Template labels propagate to MachineSets top-level and MachineSets template metadata.
Template annotations propagate to MachineSets template metadata.
- `.spec.template.metadata.labels` => `MachineSet.labels`, `MachineSet.spec.template.metadata.labels`
- `.spec.template.metadata.annotations` => `MachineSet.spec.template.metadata.annotations`

## MachineSet
Top-level labels and annotations do not propagate at all.
- `.labels` => Not propagated.
- `.annotations` => Not propagated.

Template labels and annotations propagate to Machines, InfraMachines and BootstrapConfigs.
- `.spec.template.metadata.labels` => `Machine.labels`, `InfraMachine.labels`, `BootstrapConfig.labels`
- `.spec.template.metadata.annotations` => `Machine.annotations`, `InfraMachine.annotations`, `BootstrapConfig.labels`
