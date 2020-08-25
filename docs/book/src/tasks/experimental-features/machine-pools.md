# Experimental Feature: MachinePool (alpha)

`MachinePool` feature provides a way to manage a set of machines by defining a common configuration, number of desired machine replicas etc. similar to `MachineDeployment`,
except `MachineSet` controllers are responsible for the lifecycle management of the machines for `MachineDeployment`, whereas in `MachinePools`,
each infrastructure provider has a specific solution for orchestrating these `Machines`.

**Feature gate name**: `MachinePool`

**Variable name to enable/disable the feature gate**: `EXP_MACHINE_POOL`

Infrastructure providers can support this feature by implementing their specific `MachinePool` such as `AzureMachinePool`.

More details on `MachinePool` can be found at:
[MachinePool CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20190919-machinepool-api.md)
