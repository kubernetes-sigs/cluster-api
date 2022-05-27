# Using the Cluster Autoscaler

This section applies only to worker Machines. Cluster Autoscaler is a tool that automatically adjusts the size of the Kubernetes cluster based
on the utilization of Pods and Nodes in your cluster. For more general information about the
Cluster Autoscaler, please see the
[project documentation](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler).

The following instructions are a reproduction of the Cluster API provider specific documentation
from the [Autoscaler project documentation](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/cloudprovider/clusterapi).

{{#embed-github repo:"kubernetes/autoscaler" path:"cluster-autoscaler/cloudprovider/clusterapi/README.md" }}
