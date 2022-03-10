# Changing a ClusterClass

## Selecting a strategy

When planning a change to a ClusterClass, users should always take into consideration
how those changes might impact the existing Clusters already using the ClusterClass, if any.

There are two strategies for defining how a ClusterClass change rolls out to existing Clusters: 

- Roll out ClusterClass changes to existing Cluster in a controlled/incremental fashion.
- Roll out ClusterClass changes to all the existing Cluster immediately.

The first strategy is the recommended choice for people starting with ClusterClass; it
requires the users to create a new ClusterClass with the expected changes, and then
[rebase](#rebase) each Cluster to use the newly created ClusterClass.

By splitting the change to the ClusterClass and its rollout
to Clusters into separate steps the user will reduce the risk of introducing unexpected
changes on existing Clusters, or at least limit the blast radius of those changes
to a small number of Clusters already rebased (in fact it is similar to a canary deployment).

The second strategy listed above instead requires changing a ClusterClass "in place", which can
be simpler and faster than creating a new ClusterClass. However, this approach
means that changes are immediately propagated to all the Clusters already using the 
modified ClusterClass. Any operation involving many Clusters at the same time has intrinsic risks,
and it can impact heavily on the underlying infrastructure in case the operation triggers 
machine rollout across the entire fleet of Clusters.

However, regardless of which strategy you are choosing to implement your changes to a ClusterClass, 
please make sure to:

- [Plan ClusterClass changes](#planning-clusterclass-changes) before applying them.
- Understand what [Compatibility Checks](#compatibility-checks) are and how to prevent changes
  that can lead to non-functional Clusters.

If instead you are interested in understanding more about which kind of  
effects you should expect on the Clusters, or if you are interested in additional details
about the internals of the topology reconciler you can start reading the notes in the
[Plan ClusterClass changes](#planning-clusterclass-changes) documentation or looking at the [reference](#reference)
documentation at the end of this page.

## Changing ClusterClass templates

Templates are an integral part of a ClusterClass, and thus the same considerations
described in the previous paragraph apply. When changing
a template referenced in a ClusterClass users should also always plan for how the
change should be propagated to the existing Clusters and choose the strategy that best
suits expectations.

According to the [Cluster API operational practices](../../updating-machine-templates.md),
the recommended way for updating templates is by template rotation:
- Create a new template
- Update the template reference in the ClusterClass
- Delete the old template

<aside class="note">
<h1>In place template mutations</h1>

In case a provider supports in place template mutations, the Cluster API topology controller
will adapt to them during the next reconciliation, but the system is not watching for those changes. 
Meaning, when the underlying template is updated the changes
may not be reflected immediately, however they will be picked up during the next full reconciliation.
The maximum time for the next full reconciliation is equal to the CAPI controller
sync period (defaults to 10 minutes).

</aside>

<aside class="note warning">
<h1>Reusing templates across ClusterClasses</h1>

As already discussed in [writing a cluster class](write-clusterclass.md), while it is technically possible to
re-use a template across ClusterClasses, this practice is not recommended because it makes it difficult
to reason about the impact of changing such a template can have on existing Clusters.

</aside>

Also in case of changes to the ClusterClass templates, please make sure to:

- [Plan ClusterClass changes](#planning-clusterclass-changes) before applying them.
- Understand what [Compatibility Checks](#compatibility-checks) are and how to prevent changes
  that can lead to non-functional Clusters.

You can learn more about this reading the notes in the [Plan ClusterClass changes](#planning-clusterclass-changes) documentation or
looking at the [reference](#reference) documentation at the end of this page.

## Rebase

Rebasing is an operational practice for transitioning a Cluster from one ClusterClass to another,
and the operation can be triggered by simply changing the value in `Cluster.spec.topology.class`.

Also in this case, please make sure to:

- [Plan ClusterClass changes](#planning-clusterclass-changes) before applying them.
- Understand what [Compatibility Checks](#compatibility-checks) are and how to prevent changes
  that can lead to non-functional Clusters.

You can learn more about this reading the notes in the [Plan ClusterClass changes](#planning-clusterclass-changes) documentation or
looking at the [reference](#reference) documentation at the end of this page.

## Compatibility Checks

When changing a ClusterClass, the system validates the required changes according to
a set of "compatibility rules" in order to prevent changes which would lead to a non-functional
Cluster, e.g. changing the InfrastructureProvider from AWS to Azure.

If the proposed changes are evaluated as dangerous, the operation is rejected.

<aside class="note warning">
<h1>Warning</h1>

In the current implementation there are no compatibility rules for changes to provider
templates, so you should refer to the provider documentation to avoid
potentially dangerous changes on those objects.

</aside>

For additional info see [compatibility rules](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20210526-cluster-class-and-managed-topologies.md#clusterclass-compatibility)
defined in the ClusterClass proposal.

## Planning ClusterClass changes

It is highly recommended to always generate a plan for ClusterClass changes before applying them,
no matter if you are creating a new ClusterClass and rebasing Clusters or if you are changing
your ClusterClass in place.

The clusterctl tool provides a new alpha command for this operation, [clusterctl alpha topology plan](../../../clusterctl/commands/alpha-topology-plan.md).

The output of this command will provide you all the details about how those changes would impact
Clusters, but the following notes can help you to understand what you should
expect when planning your ClusterClass changes:

- Users should expect the resources in a Cluster (e.g. MachineDeployments) to behave consistently
  no matter if a change is applied via a ClusterClass or directly as you do in a Cluster without
  a ClusterClass. In other words, if someone changes something on a KCP object triggering a
  control plane Machines rollout, you should expect the same to happen when the same change
  is applied to the KCP template in ClusterClass.

- User should expect the Cluster topology to change consistently irrespective of how the change has been
  implemented inside the ClusterClass; in other words, if you change a template field "in place", if you
  rotate the template referenced in the ClusterClass by pointing to a new template with the same field
  changed, or if you change the same field via a patch, the effects on the Cluster are the same.

- Users should expect the Cluster topology to change consistently irrespective of how the change has been
  applied to the ClusterClass. In other words, if you change a template field "in place",  or if you
  rotate the template referenced in the ClusterClass by pointing to a new template with the same field
  changed, or if you change the same field via a patch, the effects on the Cluster are the same.

See [reference](#reference) for more details.

## Reference

### Effects on the Clusters

The following table documents the effects each ClusterClass change can have on a Cluster.

| Changed field                                   | Effects on Clusters                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
|-------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| infrastructure.ref                              | Corresponding InfrastructureCluster objects are updated (in place update).                                                                                                                                                                                                                                                                                                                                                                                                                     |
| controlPlane.metadata                           | If labels/annotations are added, changed or deleted the ControlPlane objects are updated (in place update).<br /><br /> In case of KCP, corresponding controlPlane Machines are updated (rollout) only when adding or changing labels or annotations; deleted label should be removed manually from machines or they will go away automatically at the next machine rotation.      |
| controlPlane.ref                                | Corresponding ControlPlane objects are updated (in place update). <br /> If updating ControlPlane objects implies changes in the spec, the corresponding ControlPlane Machines are updated accordingly (rollout).                                                                                                                                                                                                                                                                                          |
| controlPlane.machineInfrastructure.ref          | If the referenced template has changes only in metadata labels or annotations, the corresponding InfrastructureMachineTemplates are updated (in place update). <br /> <br />If the referenced template has changes in the spec:<br />  - Corresponding InfrastructureMachineTemplate are rotated (create new, delete old)<br />  - Corresponding ControlPlane objects are updated with the reference to the newly created template (in place update)<br />  - The corresponding controlPlane Machines are updated accordingly (rollout). |
| workers.machineDeployments                      | If a new MachineDeploymentClass is added, no changes are triggered to the Clusters. <br />If an existing MachineDeploymentClass is changed, effect depends on the type of change (see below).                                                                                                                                                                                                                                        |
| workers.machineDeployments[].metadata           | If labels/annotations are added, changed or deleted the MachineDeployment objects are updated (in place update) and corresponding worker Machines are updated (rollout).       |
| workers.machineDeployments[].bootstrap.ref      | If the referenced template has changes only in metadata labels or annotations, the corresponding BootstrapTemplates are updated (in place update).<br /> <br />If the referenced template has changes in the spec:<br />  -  Corresponding BootstrapTemplate are rotated (create new, delete old). <br />  - Corresponding MachineDeployments objects are updated with the reference to the newly created template (in place update). <br />  - The corresponding worker machines are updated accordingly (rollout)                        |
| workers.machineDeployments[].infrastructure.ref | If the referenced template has changes only in metadata labels or annotations, the corresponding InfrastructureMachineTemplates are updated (in place update). <br /> <br />If the referenced template has changes in the spec:<br />  -  Corresponding InfrastructureMachineTemplate are rotated (create new, delete old).<br />  -  Corresponding MachineDeployments objects are updated with the reference to the newly created template (in place update). <br />  - The corresponding worker Machines are updated accordingly (rollout) |

### How the topology controller reconciles template fields

The topology reconciler enforces values defined in the ClusterClass templates into the topology
owned objects in a Cluster.

A simple way to understand this is to `kubectl get -o json` templates referenced in a ClusterClass;
then you can consider the topology reconciler to be authoritative on all the values
under `spec`. Being authoritative means that the user cannot manually change those values in
the object derived from the template in a specific Cluster (and if they do so the value gets reconciled
to the value defined in the ClusterClass).

<aside class="note">
<h1>What about patches?</h1>

The considerations above apply also when using patches, the only difference being that the
authoritative fields should be determined by applying patches on top of the `kubectl get -o json` output. 

</aside>

A corollary of the behaviour described above is that it is technically possible to change non-authoritative
fields in the object derived from the template in a specific Cluster, but we advise against using the possibility
or making ad-hoc changes in generated objects unless otherwise needed for a workaround. It is always
preferable to improve ClusterClasses by supporting new Cluster variants in a reusable way.

