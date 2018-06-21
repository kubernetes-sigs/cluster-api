# Cluster API GCP Cloud Provider

## Cluster Deletion

This guide explains how to delete all resources that were created as part of
your Cluster API Kubernetes cluster.

If your cluster was created using the gcp-deployer tool, see the 
[gcp-deployer docs](../../gcp-deployer/README.md).

1. Remember the service accounts that were created for your cluster

   ```bash
   export MASTER_SERVICE_ACCOUNT=$(kubectl get cluster -o=jsonpath='{.items[0].metadata.annotations.gce\.clusterapi\.k8s\.io\/service-account-k8s-master}')
   export WORKER_SERVICE_ACCOUNT=$(kubectl get cluster -o=jsonpath='{.items[0].metadata.annotations.gce\.clusterapi\.k8s\.io\/service-account-k8s-worker}')
   export INGRESS_CONTROLLER_SERVICE_ACCOUNT=$(kubectl get cluster -o=jsonpath='{.items[0].metadata.annotations.gce\.clusterapi\.k8s\.io\/service-account-k8s-ingress-controller}')
   export MACHINE_CONTROLLER_SERVICE_ACCOUNT=$(kubectl get cluster -o=jsonpath='{.items[0].metadata.annotations.gce\.clusterapi\.k8s\.io\/service-account-k8s-machine-controller}')
   ```

1. Remember the name and zone of the master VM and the name of the cluster

   ```bash
   export CLUSTER_NAME=$(kubectl get cluster -o=jsonpath='{.items[0].metadata.name}')
   export MASTER_VM_NAME=$(kubectl get machines -l set=master | awk '{print $1}' | tail -n +2)
   export MASTER_VM_ZONE=$(kubectl get machines -l set=master -o=jsonpath='{.items[0].metadata.annotations.gcp-zone}')
   ```

1. Delete all of the node Machines in the cluster. Make sure to wait for the
corresponding Nodes to be deleted before moving onto the next step. After this
step, the master node will be the only remaining node.

   ```bash
   kubectl delete machines -l set=node
   kubectl get nodes
   ```

1. Delete any Kubernetes objects that may have created GCE resources on your
behalf, make sure to run these commands for each namespace that you created:

   ```bash
   # See ingress controller docs for information about resources created for
   # ingress objects: https://github.com/kubernetes/ingress-gce
   kubectl delete ingress --all

   # Services can create a GCE load balancer if the type of the service is
   # LoadBalancer. Additionally, both types LoadBalancer and NodePort will
   # create a firewall rule in your project.
   kubectl delete svc --all

   # Persistent volume claims can create a GCE disk if the type of the pvc
   # is gcePersistentDisk.
   kubectl delete pvc --all
   ```

1. Delete the VM that is running your cluster's control plane

   ```bash
   gcloud compute instances delete --zone=$MASTER_VM_ZONE $MASTER_VM_NAME
   ```

1. Delete the roles and service accounts that were created for your cluster

   ```bash
   ./delete-service-accounts.sh
   ```

1. Delete the Firewall rules that were created for the cluster

   ```bash
   gcloud compute firewall-rules delete $CLUSTER_NAME-allow-cluster-internal
   gcloud compute firewall-rules delete $CLUSTER_NAME-allow-api-public
   ```