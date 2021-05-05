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
[MachinePool CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20190919-machinepool-api.md)

For developer docs on the MachinePool controller, see [here](./../../developer/architecture/controllers/machine-pool.md).
