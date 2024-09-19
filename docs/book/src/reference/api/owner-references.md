# Owner References

Cluster API uses [Kubernetes owner references](https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/) to track relationships between objects. These references are used
for Kubernetes garbage collection, which is also used for Cluster deletion in CAPI. They are also used places where 
the ownership hierarchy is important, for example when using `clusterctl move`.

CAPI uses owner references in an opinionated way. The following guidelines should be considered:
1. Objects should always be created with an owner reference to prevent leaking objects. Initial ownerReferences can be  
   replaced later where another object is a more appropriate owner.
2. Owner references should be re-reconciled if they are lost for an object. This is required as some tools - e.g. velero -
   may delete owner references on objects.
3. Owner references should be kept to the most recent apiVersion.
   - This ensures garbage collection still works after an old apiVersion is no longer served.
4. Owner references should not be added unless required.
   - Multiple owner references on a single object should be exceptional.

## Owner reference relationships in Cluster API

The below tables map out the a reference for ownership relationships for the objects in a Cluster API cluster. The tables
are identical for classy and non-classy clusters.

Providers may implement their own ownership relationships which may or may not map directly to the below tables. 
These owner references are almost all tested in an [end-to-end test](https://github.com/kubernetes-sigs/cluster-api/blob/caaa74482b51fae777334cd7a29595da1c06481e/test/e2e/quick_start_test.go#L31). Lack of testing is noted where this is not the case. 
CAPI Providers can take advantage of the e2e test framework to ensure their owner references are predictable, documented and stable.

## Kubernetes core types

| type      | Owner               | Controller | Note                                       |
|-----------|---------------------|------------|--------------------------------------------|
| Secret    | KubeadmControlPlane | yes        | For cluster certificates                   |
| Secret    | KubeadmConfig       | yes        | For bootstrap secrets                      |
| Secret    | ClusterResourceSet  | no         | When referenced by CRS. Not tested in e2e. |
| ConfigMap | ClusterResourceSet  | no         | When referenced by CRS                     |

## Core types

| type                | Owner               | Controller | Note                       |
|---------------------|---------------------|------------|----------------------------|
| ExtensionConfig     | None                |            |                            |
| ClusterClass        | None                |            |                            |
| Cluster             | None                |            |                            |
| MachineDeployments  | Cluster             | no         |                            |
| MachineSet          | MachineDeployment   | yes        |                            |
| Machine             | MachineSet          | yes        | When created by MachineSet |
| Machine             | KubeadmControlPlane | yes        | When created by KCP        |
| MachineHealthChecks | Cluster             | no         |                            |

## Experimental types
| type                       | Owner              | Controller | Note                     |
|----------------------------|--------------------|------------|--------------------------|
| ClusterResourcesSet        | None               |            |                          |
| ClusterResourcesSetBinding | ClusterResourceSet | no         | May have many CRS owners |
| MachinePool                | Cluster            | no         |                          |

## KubeadmControlPlane types
| type                        | Owner        | Controller | Note |
|-----------------------------|--------------|------------|------|
| KubeadmControlPlane         | Cluster      | yes        |      |
| KubeadmControlPlaneTemplate | ClusterClass | no         |      |

## Kubeadm bootstrap types
| type                  | Owner        | Controller | Note                                            |
|-----------------------|--------------|------------|-------------------------------------------------|
| KubeadmConfig         | Machine      | yes        | When created for Machine                        |
| KubeadmConfig         | MachinePool  | yes        | When created for MachinePool                    |
| KubeadmConfigTemplate | Cluster      | no         | When referenced in MachineDeployment spec       |
| KubeadmConfigTemplate | ClusterClass | no         | When referenced in ClusterClass                 |

## Infrastructure provider types
| type                          | Owner        | Controller | Note                                        |
|-------------------------------|--------------|------------|---------------------------------------------|
| InfrastructureMachine         | Machine      | yes        |                                             |
| InfrastructureMachineTemplate | Cluster      | no         | When created by cluster topology controller |
| InfrastructureMachineTemplate | ClusterClass | no         | When referenced in a ClusterClass           |
| InfrastructureCluster         | Cluster      | yes        |                                             |
| InfrastructureClusterTemplate | ClusterClass | no         |                                             | 
| InfrastructureMachinePool     | MachinePool  | yes        |                                             |
