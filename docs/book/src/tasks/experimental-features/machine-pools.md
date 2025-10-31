# Experimental Feature: MachinePool (beta)

The `MachinePool` feature provides a way to manage a set of machines by defining a common configuration, number of desired machine replicas etc. similar to `MachineDeployment`,
except `MachineSet` controllers are responsible for the lifecycle management of the machines for `MachineDeployment`, whereas in `MachinePools`,
each infrastructure provider has a specific solution for orchestrating these `Machines`.

**Feature gate name**: `MachinePool`

**Variable name to enable/disable the feature gate**: `EXP_MACHINE_POOL`

Infrastructure providers can support this feature by implementing their specific `MachinePool` such as `AzureMachinePool`.

More details on `MachinePool` can be found at:
[MachinePool CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20190919-machinepool-api.md)

For developer docs on the MachinePool controller, see [here](./../../developer/core/controllers/machine-pool.md).

## MachinePools vs MachineDeployments

Although MachinePools provide a similar feature to MachineDeployments, MachinePools do so by leveraging an InfraMachinePool which corresponds 1:1 with a resource like VMSS on Azure or Autoscaling Groups on AWS which we treat as a black box. When a MachinePool is scaled up, the InfraMachinePool scales itself up and populates its provider ID list based on the response from the infrastructure provider. On the other hand, when a MachineDeployment is scaled up, new Machines are created which then create an individual InfraMachine, which corresponds to a VM in any infrastructure provider.

| MachinePools                                                                                                                                                        | MachineDeployments                                                                                                                     |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Creates new instances through a single infrastructure resource like VMSS in Azure or Autoscaling Groups in AWS.                                                     | Creates new instances by creating new Machines, which create individual VM instances on the infra provider.                            |
| Set of instances is orchestrated by the infrastructure provider.                                                                                                    | Set of instances is orchestrated by Cluster API using a MachineSet.                                                                    |
| Each MachinePool corresponds 1:1 with an associated InfraMachinePool.                                                                                               | Each MachineDeployment includes a MachineSet, and for each replica, it creates a Machine and InfraMachine.                             |
| Each MachinePool requires only a single BootstrapConfig.                                                                                                            | Each MachineDeployment uses an InfraMachineTemplate and a BootstrapConfigTemplate, and each Machine requires a unique BootstrapConfig. |
| Maintains a list of instances in the `providerIDList` field in the MachinePool spec. This list is populated based on the response from the infrastructure provider. | Maintains a list of instances through the Machine resources owned by the MachineSet.                                                   |

## MachinePool provider implementations

The following Cluster API infrastructure providers have implemented support for MachinePools:

| Provider | Implementations | Status | MachinePool Machines support |
| --- | --- | --- | --- |
| [AWS](https://cluster-api-aws.sigs.k8s.io/topics/machinepools.html) | `AWSManagedMachinePool`<br> `AWSMachinePool`<br>`ROSAMachinePool` | Implemented | Yes; has support for deletion of single machine |
| [Azure](https://capz.sigs.k8s.io/self-managed/machinepools) | `AzureASOManagedMachinePool`<br> `AzureManagedMachinePool`<br> `AzureMachinePool` | Implemented | Yes; unknown support for deletion of single machine |
| [GCP](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/pull/1506) | `GCPMachinePool` | In Progress | Unknown |
| [OCI](https://oracle.github.io/cluster-api-provider-oci/managed/managedcluster.html) | `OCIManagedMachinePool`<br> `OCIMachinePool` | Implemented | Yes; doesn't have support for deletion of single machine |
| [Scaleway](https://github.com/scaleway/cluster-api-provider-scaleway/blob/main/docs/scalewaymanagedmachinepool.md) | `ScalewayManagedMachinePool` | Implemented | No |

Providers may support the deletion of single machine pool `Machine` objects. That allows, for example, using `MachineHealthCheck` to remediate machines that became unhealthy.
