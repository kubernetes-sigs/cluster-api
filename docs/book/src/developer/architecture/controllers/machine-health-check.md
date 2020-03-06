# MachineHealthCheck

A MachineHealthCheck is responsible for remediating unhealthy [Machines](./machine.md).

Its main responsibilities are:
* Checking the health of Nodes in [target clusters] against a list of unhealthy conditions
* Remediating Machine's for Nodes determined to be unhealthy

![](../../../images/machinehealthcheck-controller.png)

<!-- links -->
[target clusters]: ../../../reference/glossary.md#target-cluster
