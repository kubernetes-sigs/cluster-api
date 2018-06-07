# Cluster API vSphere Provider

## Delete Cluster

This guide explains how to delete all resources that were created as part of
your Cluster API Kubernetes cluster.

The provider currently only creates resources specified in the named machine
config map. The default examples only provision VMs. If your custom HCL config
configures load balancers, or provisions other resources, amend this guide as
necessary.

1. Delete all of the node `Machine`s in the cluster. Make sure to wait for the corresponding Nodes to be deleted before moving onto the next step. After this step, the master node will be the only remaining node.
   
```bash
kubectl get nodes

# For each non-master node (See the "ROLES" column)
kubectl delete machines $MACHINE-NAME
```

2. Delete the VM that is running your cluster's control plane. You can either do this from the vCenter UI or using govc.

```bash
govc vm.destroy $MACHINE-NAME
```
