# Owner References


Cluster API uses [Kubernetes owner references](https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/) to track relationships between objects. These references are used for Kubernetes garbage collection, which is the basis of Cluster deletion in CAPI. They are also used places where the ownership hierarchy is important, for example when using `clusterctl move`.

CAPI uses owner references in an opinionated way. The following guidelines should be considered:
1. Objects should always be created with an owner reference to prevent leaking objects. Initial ownerReferences can be replaced later where another object is a more appropriate owner.
2. Owner references should be re-reconciled if they are lost for an object. This is required as some tools - e.g. velero - may delete owner references on objects.
3. Owner references should be kept to the most recent apiVersion.
   - This ensures garbage collection still works after an old apiVersion is no longer served.
4. Owner references should not be added unless required.
   - Multiple owner references on a single object should be exceptional.


  

## Owner reference relationships in Cluster API

The below tables map out the a reference for ownership relationships for the objects in a Cluster API cluster.
Providers may implement their own ownership relationships which may or may not map directly to the below tables. 
These owner references are almost all tested in an [end-to-end test](https://github.com/kubernetes-sigs/cluster-api/blob/caaa74482b51fae777334cd7a29595da1c06481e/test/e2e/quick_start_test.go#L31). Lack of testing is noted where this is not the case. CAPI Providers can take advantage of the e2e test framework to ensure their owner references are predictable, documented and stable.

 Kubernetes core types

| type      | Owner               | Note                                     |
|-----------|---------------------|------------------------------------------|
| Secret    | KubeadmControlPlane | For cluster certificates                 |
| Secret    | KubeadmConfig       | For bootstrap secrets                    |
| Secret    | ClusterResourceSet  | When created by CRS. Not covered in e2e. |
| ConfigMap | ClusterResourceSet  | When created by CRS                      |

## Core types

| type                | Owner               | Note                       |
|---------------------|---------------------|----------------------------|
| ExtensionConfig     | None                |                            |
| ClusterClass        | None                |                            |
| Cluster             | None                |                            |
| MachineDeployments  | Cluster             |                            |
| MachineSet          | MachineDeployment   |                            |
| Machine             | MachineSet          | When created by MachineSet |
| Machine             | KubeadmControlPlane | When created by KCP        |
| MachineHealthChecks | Cluster             |                            |



## Experimental types
| type                       | Owner              | Note |
|----------------------------|--------------------|------|
| ClusterResourcesSet        | None               |      |
| ClusterResourcesSetBinding | ClusterResourceSet |      |
| MachinePool                | Cluster            |      |


## KubeadmControlPlane types
| type                        | Owner        | Note |
|-----------------------------|--------------|------|
| KubeadmControlPlane         | Cluster      |      |
| KubeadmControlPlaneTemplate | ClusterClass |      |
    

## Kubeadm bootstrap types
| type                  | Owner        | Note                                      |
|-----------------------|--------------|-------------------------------------------|
| KubeadmConfig         | Machine      | When created for Machine                  |
| KubeadmConfig         | MachinePool  | When created for MachinePool              |
| KubeadmConfigTemplate | Cluster      | When referenced in MachineDeployment spec |
| KubeadmConfigTemplate | ClusterClass | When referenced in ClusterClass           |

## Infrastructure provider types
| type                          | Owner        | Note                                        |
|-------------------------------|--------------|---------------------------------------------|
| InfrastructureMachine         | Machine      |                                             |
| InfrastructureMachineTemplate | Cluster      | When created by cluster topology controller |
| InfrastructureMachineTemplate | ClusterClass | When referenced in a ClusterClass           |
| InfrastructureCluster         | Cluster      |                                             |
| InfrastructureClusterTemplate | ClusterClass |                                             |
| InfrastructureMachinePool     | MachinePool  |                                             |


