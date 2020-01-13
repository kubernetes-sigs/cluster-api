# Support Managed Kubernetes Provider

## Glossary
The lexicon used in this document is described in more detail [here](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/book/src/reference/glossary.md). Any discrepancies should be rectified in the main Cluster API glossary.

- **MKP** - Managed Kubernetes Provider

## Summary

In current CAPI, users can create Kubernetes cluster by a series of [providers](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/book/src/reference/providers.md#infrastructure), the providers interact with cloud infrastructure resource such as machine, LB, security groups etc directly and are generally referred to as infrastructure provider. The infrastructure provider usually works by create machines on the cloud or bare metal and turning them into Kubernetes master nodes (control plane) or worker nodes, finally expose them as workload/target Kubernetes cluster to end user for using.

The most of public clouds provider not only provide infrastructure service but also provide Kubernetes as a service (K8SaaS) as known as MKP . For example, [AWS provider](https://github.com/kubernetes-sigs/cluster-api-provider-aws) depends on Amazon EC2 service build Kubernetes cluster, actually Amazon Elastic Kubernetes Service (Amazon EKS) can achieve this as well. This proposal outlines adding a new approach to CAPI by leveraging MKP like EKS, IKS, EKS, GKE and AKS etc to create and manage Kubernetes cluster directly. The operations to provision infrastructure specific resource and turning resource to Kubernetes node are provided by MKP itself, no cloud infrastructure resource interactions and bootstrap process (kubeadm by default) get involved in CAPI anymore.

MKP is an enhancement and optional against current infrastructure provider, that means user still can use current Cluster API infrastructure provider but have another choice to manage workload/target Kubernetes cluster, since not every cloud provider will have MKP functionality.

## Motivation

The public clouds provider have invested a significant amount of time optimizing and reliable of operation to manage Kubernetes cluster. The current CAPI provider provisioning and turning solution has a lot of steps and interactions with infrastructure layer which may lead error-prone. Allowing users of CAPI to leverage the optimization approach exposed by MKP could prove beneficial.

**Potential benefits include:**
- Faster cluster provisioning
- Improved provisioning success rates
- Automatic cluster operations for create/delete/upgrade/scaling if supported by cloud provider
- Deeply integrated with cloud provider on storage, network, LB, security and HA
- Improved user experience
   - less YAML files to maintain

### Goals

-  Enhance existing CAPI providers for leveraging MKP to managing Kubernetes cluster.
  - Add [EKS](https://aws.amazon.com/eks/) support to [AWS provider](https://github.com/kubernetes-sigs/cluster-api-provider-aws) 
  - Add [IKS](https://www.ibm.com/cloud/container-service/) support to [IBM Cloud provider](https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud)
  - Add [AKS](https://azure.microsoft.com/en-in/services/kubernetes-service/) support to [Azure provider](https://github.com/kubernetes-sigs/cluster-api-provider-azure)
  - Add [GKE](https://cloud.google.com/kubernetes-engine/) support to [GCP provider](https://github.com/kubernetes-sigs/cluster-api-provider-gcp)
  - Add [DOKS](https://www.digitalocean.com/products/kubernetes/) support to [DigitalOcean provider](https://github.com/kubernetes-sigs/cluster-api-provider-digitalocean)
  - Add [ACK](https://www.alibabacloud.com/product/kubernetes) support to [Alibaba Cloud](https://github.com/oam-oss/cluster-api-provider-alicloud) 

### Non-goals/Future Work

- To integrate with the Kubernetes cluster autoscaler.

## Proposal

This proposal introduces MKP for the purpose of delegating the management of Kubernetes cluster to existing infrastructure [provider](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/book/src/reference/providers.md#infrastructure), after that, the providers in `Goals` section will have 2 ways manage Kubernetes cluster according to different user case.

It should have 3 approaches to extend existing [providers](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/book/src/reference/providers.md#infrastructure) to have MKP functionality: 
 1.  Reuse `infrastructureRef`from `Cluster` but have new MKP implementation:  for example, [awscluster_controller.go](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/master/controllers/awscluster_controller.go) VS `ekscluster_controller.go`
 2.  Implement MKP based controller Control Plane to fill the gap from `Non-Goals/Future Work` section in [kubeadm-based-control-plane.md](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20191017-kubeadm-based-control-plane.md), but cover both control plane and worker load plane.
 3.  (**I  preferred**)Create individual entity `managedK8SRef` to `Cluster` against `infrastructureRef`with new controller: for example, [awscluster_controller.go](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/master/controllers/awscluster_controller.go) VS `ekscluster_controller.go`

 
## Implementation Details/Notes/Constraints

The new entity `managedK8SRef` is added to `Cluster` against `infrastructureRef` aim to describe workload/target Kubernetes cluster specification,  take [AWS provider](https://github.com/kubernetes-sigs/cluster-api-provider-aws)  as an example,  the `EKSCluster` is MKP implementation for AWS, other cloud MKPs will have `IKSCluster`,  `AKSCluster` etc accordingly.   

- `Cluster` controller `Reconcile` function from CAPI will sync up with latest`EKSCluster` status and generate `kubeconfig`
- `EKSCluster` controller `Reconcile` function from [AWS provider](https://github.com/kubernetes-sigs/cluster-api-provider-aws)  will handle EKS cluster life cycle management: create/scaling up/scaling down/delete.

```
kind: Cluster
apiVersion: cluster.x-k8s.io/v1alpha3
metadata:
  name: my-cluster
  namespace: default
spec:
  clusterNetwork:
    pods:
      cidrBlocks: ["192.168.0.0/16"]
    services:
      cidrBlocks: ["10.96.0.0/12"]
  managedK8SRef:
    kind: EKSCluster
    apiVersion: managedk8s.cluster.x-k8s.io/v1alpha1
    name: my-ekscluster
    namespace: default
---
apiVersion: managedk8s.cluster.x-k8s.io/v1alpha1
kind: EKSCluster
metadata:
  name: my-ekscluster
  namespace: default
spec:
  region: antarctica-1
  version: 1.14
  nodeType: t3.medium
  nodes: 1
  nodesMin: 1
  nodesMax: 2
```

### User Stories

- As a cluster operator, I would like to build a Kubernetes cluster leverage MKP.
