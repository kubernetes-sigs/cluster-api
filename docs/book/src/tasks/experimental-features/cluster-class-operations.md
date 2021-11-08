# Clusterclass operations
Clusterclass and managed topologies can be used to control a number of advanced behaviors in Cluster API. 

##  Changing a ClusterClass
When you change a ClusterClass, the system validates the required changes according to the [compatibility rules defined in the ClusterClass proposal](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/202105256-cluster-class-and-managed-topologies.md#clusterclass-compatibility).

According to [Cluster API operational practices](https://cluster-api.sigs.k8s.io/tasks/updating-machine-templates.html), the recommended way for updating templates is by template rotation (create a new template, update the template reference in the ClusterClass, and then delete the old template).

<aside class="note warning">

<h1>Warning</h1>

Changing a ClusterClass triggers changes on all the Clusters using the ClusterClass.

</aside>

If changes are evaluated as potentially leading to a non-functional Cluster, the operation is rejected. It is important to note that the current implementation ensures only a minimal set of compatibility rules are applied; most importantly, there are no provider specific rules at present, so you should refer to the provider documentation for preventing potentially dangerous changes on your infrastructure.


Once the changes are applied, the topology controller reacts as described in the following table.

| Changed field                                   | Effects on Clusters                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
|-------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| infrastructure.ref                              | Corresponding InfrastructureCluster objects are updated (in place update).                                                                                                                                                                                                                                                                                                                                                                                                                     |
| controlPlane.metadata                           | If labels/annotations are added, changed or deleted the ControlPlane objects are updated (in place update).<br /><br /> In case of KCP, corresponding controlPlane Machines are updated (rollout) only when adding or changing labels or annotations; deleted label should be removed manually from machines or they will go away automatically at the next machine rotation.      |
| controlPlane.ref                                | Corresponding ControlPlane objects are updated (in place update). <br /> If updating ControlPlane objects implies changes in the spec, the corresponding ControlPlane Machines are updated accordingly (rollout).                                                                                                                                                                                                                                                                                          |
| controlPlane.machineInfrastructure.ref          | If the referenced template has changes only in metadata labels or annotations, the corresponding InfrastructureMachineTemplates are updated (in place update). <br /> <br />If the referenced template has changes in the spec:<br />  - Corresponding InfrastructureMachineTemplate are rotated (create new, delete old)<br />  - Corresponding ControlPlane objects are updated with the reference to the newly created template (in place update)<br />  - The corresponding controlPlane Machines are updated accordingly (rollout). |
| workers.machineDeployments                      | If a new MachineDeploymentClass is added, no changes are triggered to the Clusters. <br />If an existing MachineDeploymentClass is changed, effect depends on the type of change (see below).  <br /><br />Note: Deleting an existing MachineDeploymentClass is not supported.                                                                                                                                                                                                                                       |
| workers.machineDeployments[].metadata           | If labels/annotations are added, changed or deleted the MachineDeployment objects are updated (in place update) and corresponding worker Machines are updated (rollout).       |
| workers.machineDeployments[].bootstrap.ref      | If the referenced template has changes only in metadata labels or annotations, the corresponding BootstrapTemplates are updated (in place update).<br /> <br />If the referenced template has changes in the spec:<br />  -  Corresponding BootstrapTemplate are rotated (create new, delete old). <br />  - Corresponding MachineDeployments objects are updated with the reference to the newly created template (in place update). <br />  - The corresponding worker machines are updated accordingly (rollout)                        |
| workers.machineDeployments[].infrastructure.ref | If the referenced template has changes only in metadata labels or annotations, the corresponding InfrastructureMachineTemplates are updated (in place update). <br /> <br />If the referenced template has changes in the spec:<br />  -  Corresponding InfrastructureMachineTemplate are rotated (create new, delete old).<br />  -  Corresponding MachineDeployments objects are updated with the reference to the newly created template (in place update). <br />  - The corresponding worker Machines are updated accordingly (rollout) |


Note: In case a provider supports in place template mutations, the Cluster API topology controller will adapt to them at the next reconciliation, but the system is not watching for those specific changes. When the underlying template is updated in this way the changes may not be reflected immediately, but will be put in place at the next full reconciliation. The maximum time for the next reconciliation to take place is related to the CAPI controller sync period - 10 minutes by default. 


