# Metadata propagation
Cluster API controllers implement consistent metadata (labels & annotations) propagation across the core API resources.
This behaviour tries to be consistent with Kubernetes apps/v1 Deployment and ReplicaSet.
New providers should behave accordingly fitting within the following pattern:

![](../../images/metadata-propagation.jpg)

## Cluster Topology
ControlPlaneTopology labels are labels and annotations are continuously propagated to ControlPlane top-level labels and annotations
and ControlPlane MachineTemplate labels and annotations.
- `.spec.topology.controlPlane.metadata.labels` => `ControlPlane.labels`, `ControlPlane.spec.machineTemplate.metadata.labels`
- `.spec.topology.controlPlane.metadata.annotations` => `ControlPlane.annotations`, `ControlPlane.spec.machineTemplate.metadata.annotations`

MachineDeploymentTopology labels and annotations are continuously propagated to MachineDeployment top-level labels and annotations
and MachineDeployment MachineTemplate labels and annotations.
- `.spec.topology.machineDeployments[i].metadata.labels` => `MachineDeployment.labels`, `MachineDeployment.spec.template.metadata.labels`
- `.spec.topology.machineDeployments[i].metadata.annotations` => `MachineDeployment.annotations`, `MachineDeployment.spec.template.metadata.annotations`

## ClusterClass
ControlPlaneClass labels are labels and annotations are continuously propagated to ControlPlane top-level labels and annotations
and ControlPlane MachineTemplate labels and annotations.
- `.spec.controlPlane.metadata.labels` => `ControlPlane.labels`, `ControlPlane.spec.machineTemplate.metadata.labels`
- `.spec.controlPlane.metadata.annotations` => `ControlPlane.annotations`, `ControlPlane.spec.machineTemplate.metadata.annotations`
Note: ControlPlaneTopology labels and annotations take precedence over ControlPlaneClass labels and annotations.

MachineDeploymentClass labels and annotations are continuously propagated to MachineDeployment top-level labels and annotations
and MachineDeployment MachineTemplate labels and annotations.
- `.spec.workers.machineDeployments[i].template.metadata.labels` => `MachineDeployment.labels`, `MachineDeployment.spec.template.metadata.labels`
- `.spec.worker.machineDeployments[i].template.metadata.annotations` => `MachineDeployment.annotations`, `MachineDeployment.spec.template.metadata.annotations`
Note: MachineDeploymentTopology labels and annotations take precedence over MachineDeploymentClass labels and annotations.

## KubeadmControlPlane
Top-level labels and annotations do not propagate at all.
- `.labels` => Not propagated.
- `.annotations` => Not propagated.

MachineTemplate labels and annotations continuously propagate to new and existing Machines, InfraMachines and BootstrapConfigs.
- `.spec.machineTemplate.metadata.labels` => `Machine.labels`, `InfraMachine.labels`, `BootstrapConfig.labels`
- `.spec.machineTemplate.metadata.annotations` => `Machine.annotations`, `InfraMachine.annotations`, `BootstrapConfig.annotations`

## MachineDeployment
Top-level labels do not propagate at all.
Top-level annotations continuously propagate to MachineSets top-level annotations.
- `.labels` => Not propagated.
- `.annotations` => MachineSet.annotations

Template labels continuously propagate to MachineSets top-level and MachineSets template metadata.
Template annotations continuously propagate to MachineSets template metadata.
- `.spec.template.metadata.labels` => `MachineSet.labels`, `MachineSet.spec.template.metadata.labels`
- `.spec.template.metadata.annotations` => `MachineSet.spec.template.metadata.annotations`

## MachineSet
Top-level labels and annotations do not propagate at all.
- `.labels` => Not propagated.
- `.annotations` => Not propagated.

Template labels and annotations continuously propagate to new and existing Machines, InfraMachines and BootstrapConfigs.
- `.spec.template.metadata.labels` => `Machine.labels`, `InfraMachine.labels`, `BootstrapConfig.labels`
- `.spec.template.metadata.annotations` => `Machine.annotations`, `InfraMachine.annotations`, `BootstrapConfig.annotations`

## Machine
Top-level labels and annotations that meet a specific criteria are propagated to the Node labels and annotations.
- `.labels.[label-meets-criteria]` => `Node.labels`
- `.annotations.[annotation-meets-criteria]` => `Node.annotations`

Labels that meet at least one of the following criteria are always propagated to the Node:
- Has `node-role.kubernetes.io` as prefix.
- Belongs to `node-restriction.kubernetes.io` domain.
- Belongs to `node.cluster.x-k8s.io` domain.

In addition, any labels that match at least one of the regexes provided by the `--additional-sync-machine-labels` flag on the manager will be synced from the Machine to the Node.

Annotations that meet at least one of the following criteria are always propagated to the Node:
- Belongs to `node.cluster.x-k8s.io` domain

In addition, any annotations that match at least one of the regexes provided by the `--additional-sync-machine-annotations` flag on the manager will be synced from the Machine to the Node.

## Patches

While this is not technically metadata propagation, it is worth to notice that when using Cluster API managed topologies,
by using patches it is also possible to manage labels and annotations in resources that are originated from templates linked to the ClusterClass. More specifically:

Patches for ControlPlaneTemplates, InfraClusterTemplates, MachinePoolTemplates and BootstrapConfigTemplates (only if referenced from a MachinePool class):
- Changes to `.spec.template.metadata.{labels|annotations}` will be reflected in `.metadata.{labels|annotations}` of the corresponding generated object. 
  e.g. `KubeadmControlPlaneTemplate.spec.template.metadata.labels` --> `KubeadmControlPlane.metadata.labels`

Patches for InfraMachineTemplates, BootstrapConfigTemplates (except when referenced from a MachinePool)
- Changes to `.metadata.{labels|annotations}` will be reflected in `.metadata.{labels|annotations}` of the corresponding generated template.
  e.g. `VSphereMachineTemplate.metadata.labels` --> `VSphereMachineTemplate.metadata.labels`
- Changes to `.spec.template.metadata.{labels|annotations}` will be reflected in `.spec.template.metadata.{labels|annotations}` of the corresponding generated template.
  e.g. `VSphereMachineTemplate.spec.template.metadata.labels` --> `VSphereMachineTemplate.spec.template.metadata.labels`
