# Experimental Feature: MachinePool (alpha)

<aside class="note warning">

<h1> Action Required </h1>

The API group for MachinePools was renamed from `exp.cluster.x-k8s.io` to `cluster.x-k8s.io` as part of v1alpha4. Previously created MachinePool objects under the `exp.cluster.x-k8s.io` group will not be supported and will no longer reconcile.

In order to migrate your existing clusters using the experimental MachinePool feature, it is recommended to either create a new cluster and migrate workloads, or migrate your existing MachinePool objects using a tool like [Kubernetes CustomResourceDefinition Migration Tool](https://github.com/vmware/crd-migration-tool).

</aside>


The `MachinePool` feature provides a way to manage a set of machines by defining a common configuration, number of desired machine replicas etc. similar to `MachineDeployment`,
except `MachineSet` controllers are responsible for the lifecycle management of the machines for `MachineDeployment`, whereas in `MachinePools`,
each infrastructure provider has a specific solution for orchestrating these `Machines`.

**Feature gate name**: `MachinePool`

**Variable name to enable/disable the feature gate**: `EXP_MACHINE_POOL`

Infrastructure providers can support this feature by implementing their specific `MachinePool` such as `AzureMachinePool`.

More details on `MachinePool` can be found at:
[MachinePool CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20190919-machinepool-api.md)

For developer docs on the MachinePool controller, see [here](./../../developer/architecture/controllers/machine-pool.md).

## MachinePools vs MachineDeployments

Although MachinePools provide a similar feature to MachineDeployments, MachinePools do so by leveraging an InfraMachinePool which corresponds 1:1 with a resource like VMSS on Azure or Autoscaling Groups on AWS which we treat as a black box. When a MachinePool is scaled up, the InfraMachinePool scales itself up and populates its provider ID list based on the response from the infrastructure provider. On the other hand, a when a MachineDeployment is scaled up, new Machines are created which then create an individual InfraMachine, which corresponds to a VM in any infrastructure provider.

| MachinePools                                                                                                                                                        | MachineDeployments                                                                                                                     |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Creates new instances through a single infrastructure resource like VMSS in Azure or Autoscaling Groups in AWS.                                                     | Creates new instances by creating new Machines, which create individual VM instances on the infra provider.                            |
| Set of instances is orchestrated by the infrastructure provider.                                                                                                    | Set of instances is orchestrated by Cluster API using a MachineSet.                                                                    |
| Each MachinePool corresponds 1:1 with an associated InfraMachinePool.                                                                                               | Each MachineDeployment includes a MachineSet, and for each replica, it creates a Machine and InfraMachine.                             |
| Each MachinePool requires only a single BootstrapConfig.                                                                                                            | Each MachineDeployment uses an InfraMachineTemplate and a BootstrapConfigTemplate, and each Machine requires a unique BootstrapConfig. |
| Maintains a list of instances in the `providerIDList` field in the MachinePool spec. This list is populated based on the response from the infrastructure provider. | Maintains a list of instances through the Machine resources owned by the MachineSet.                                                   |