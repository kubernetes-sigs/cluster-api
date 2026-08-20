# Using the Cluster Autoscaler

This section applies only to worker Machines. Cluster Autoscaler is a tool that automatically adjusts the size of the Kubernetes cluster based
on the utilization of Pods and Nodes in your cluster. For more general information about the
Cluster Autoscaler, please see the
[project documentation](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler).

The following instructions are a reproduction of the Cluster API provider specific documentation
from the [Autoscaler project documentation](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/cloudprovider/clusterapi).

{{#embed-github repo:"kubernetes/autoscaler" path:"cluster-autoscaler/cloudprovider/clusterapi/README.md" }}

<aside class="note warning">

<h1>Defaulting of the MachineDeployment, MachineSet replicas field</h1>

Please note that the MachineDeployment and MachineSet replicas field has special defaulting logic to provide a smooth integration with the autoscaler.
The replica field is defaulted based on the autoscaler min and max size annotations.The goal is to pick a default value which is inside
the (min size, max size) range so the autoscaler can take control of the replicase field.

The defaulting logic is as follows:
* if the autoscaler min size and max size annotations are set:
  * if it's a new MachineDeployment or MachineSet, use min size
  * if the replicas field of the old MachineDeployment or MachineSet is < min size, use min size
  * if the replicas field of the old MachineDeployment or MachineSet is > max size, use max size
  * if the replicas field of the old MachineDeployment or MachineSet is in the (min size, max size) range, keep the value from the oldMD or oldMS
* otherwise, use 1
</aside>
