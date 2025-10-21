# Experimental Feature: MachinePool (beta)

**Feature gate name**: `MachinePool`

**Variable name to enable/disable the feature gate**: `EXP_MACHINE_POOL`

## Table of Contents

- [Introduction](#introduction)
- [What is a MachinePool?](#what-is-a-machinepool)
- [Why MachinePool?](#why-machinepool)
- [When to use MachinePool vs MachineDeployment](#when-to-use-machinepool-vs-machinedeployment)
- [Enabling MachinePool](#enabling-machinepool)
- [MachinePool provider implementations](#machinepool-provider-implementations)
- [Additional Resources](#additional-resources)

## Introduction

Cluster API (CAPI) manages Kubernetes worker nodes primarily through Machine, MachineSet, and MachineDeployment objects. These primitives manage nodes individually (Machines), and have served well across a wide variety of providers.

However, many infrastructure providers already offer first-class abstractions for groups of compute instances (AWS: Auto Scaling Groups (ASG), Azure: Virtual Machine Scale Sets (VMSS), or GCP: Managed Instance Groups (MIG)). These primitives natively support scaling, rolling upgrades, and health management.

MachinePool brings these provider features into Cluster API by introducing a higher-level abstraction for managing a group of machines as a single unit.

## What is a MachinePool?

A MachinePool is a Cluster API resource representing a group of worker nodes. Instead of reconciling each machine individually, CAPI delegates lifecycle management to the infrastructure provider.

- **MachinePool (core API)**: defines desired state (replicas, Kubernetes version, bootstrap template, infrastructure reference).
- **InfrastructureMachinePool (provider API)**: provides an implementation that backs a pool. A provider may offer more than one type depending on how it is managed. For example:
  - `AWSMachinePool`: self-managed ASG
  - `AWSManagedMachinePool`: EKS managed node group
  - `AzureMachinePool`: VM Scale Set
  - `AzureManagedMachinePool`: AKS managed node pool
  - `GCPManagedMachinePool`: GKE managed node pool
  - `OCIManagedMachinePool`: OKE managed node pool
  - `ScalewayManagedMachinePool`: Scaleway Kapsule node pool
- **Bootstrap configuration**: still applies (e.g., kubeadm configs), ensuring that new nodes join the cluster with the correct setup.

The MachinePool controller coordinates between the Cluster API core and provider-specific implementations:

- Reconciles desired replicas with the infrastructure pool.
- Matches provider IDs from the infrastructure resource with Kubernetes Nodes in the workload cluster.
- Updates MachinePool status (ready replicas, conditions, etc.)

## Why MachinePool?

### Leverage provider primitives

Most cloud providers already manage scaling, instance replacement, and health monitoring at the group level. MachinePool lets CAPI delegate lifecycle operations instead of duplicating that logic.

**Example:**
- AWS Auto Scaling Groups replace failed nodes automatically.
- Azure VM Scale Sets support rolling upgrades with configurable surge/availability strategies.

### Simplify upgrades and scaling

Upgrades and scaling events are managed at the pool level:
- Update Kubernetes version or bootstrap template → cloud provider handles rolling replacement.
- Scale up/down replicas → provider adjusts capacity.

This provides more predictable, cloud-native semantics compared to reconciling many individual Machine objects.

### Autoscaling integration

MachinePool integrates with the Cluster Autoscaler in the same way that MachineDeployments do. In practice, the autoscaler treats a MachinePool as a node group, enabling scale-up and scale-down decisions based on cluster load.

### Tradeoffs and limitations

While powerful, MachinePool comes with tradeoffs:

- **Infrastructure provider complexity**: requires infrastructure providers to implement and maintain an InfrastructureMachinePool type.
- **Less per-machine granularity**: you cannot configure each node individually; the pool defines a shared template.
  > **Note**: While this is typically true, certain cloud providers do offer flexibility. 
  > **Example**: AWS allows `AWSMachinepool.spec.mixedInstancesPolicy.instancesDistribution` while Azure allows `AzureMachinePool.spec.orchestrationMode`.
- **Complex reconciliation**: node-to-providerID matching introduces edge cases (delays, inconsistent states).
- **Draining**: The cloud resources for MachinePool may not necessarily support draining of Kubernetes worker nodes. For example, with an AWSMachinePool, AWS would normally terminate instances as quickly as possible. To solve this, tools like `aws-node-termination-handler` combined with ASG lifecycle hooks (defined in `AWSMachine.spec.lifecycleHooks`) must be installed, and is not a built-in feature of the infrastructure provider (CAPA in this example).
- **Maturity**: The MachinePool API is still considered experimental/beta.

## When to use MachinePool vs MachineDeployment

Both MachineDeployment and MachinePool are valid options for managing worker nodes in Cluster API. The right choice depends on your infrastructure provider's capabilities and your operational requirements.

### Use MachinePool when:

- **Cloud provider supports scaling group primitives**: AWS Auto Scaling Groups, Azure Virtual Machine Scale Sets, GCP Managed Instance Groups, OCI Compute Instances, Scaleway Kapsule. These resources natively handle scaling, rolling upgrades, and health checks.
- **You want to leverage cloud provider-level features**: MachinePool enables direct use of cloud-native upgrade strategies (e.g., surge, maxUnavailable) and autoscaling behaviors.

### Use MachineDeployment when:

- **The provider does not support scaling groups**: Common in environments such as bare metal, vSphere, or Docker.
- **You need fine-grained per-machine control**: MachineDeployments allow unique bootstrap configurations, labels, and taints across different MachineSets.
- **You prefer maturity and portability**: MachineDeployment is stable, GA, and supported across all providers. MachinePool remains experimental in some implementations.

## Enabling MachinePool

Starting from Cluster API v1.7, MachinePool is enabled by default. No additional configuration is needed.

For Cluster API versions prior to v1.7, you need to set the `EXP_MACHINE_POOL` environment variable:

```bash
export EXP_MACHINE_POOL=true
clusterctl init
```

Or when upgrading an existing management cluster:

```bash
export EXP_MACHINE_POOL=true
clusterctl upgrade
```

## MachinePool provider implementations

The following Cluster API infrastructure providers have implemented support for MachinePools:

| Provider | Implementations | Status |
| --- | --- | --- |
| [AWS](https://cluster-api-aws.sigs.k8s.io/topics/machinepools.html) | `AWSManagedMachinePool`<br> `AWSMachinePool`<br>`ROSAMachinePool` | Implemented, MachinePoolMachines supported |
| [Azure](https://capz.sigs.k8s.io/self-managed/machinepools) | `AzureASOManagedMachinePool`<br> `AzureManagedMachinePool`<br> `AzureMachinePool` | Implemented, MachinePoolMachines supported |
| [GCP](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/pull/1506) | `GCPMachinePool` | In Progress |
| [OCI](https://oracle.github.io/cluster-api-provider-oci/managed/managedcluster.html) | `OCIManagedMachinePool`<br> `OCIMachinePool` | Implemented, MachinePoolMachines supported |
| [Scaleway](https://github.com/scaleway/cluster-api-provider-scaleway/blob/main/docs/scalewaymanagedmachinepool.md) | `ScalewayManagedMachinePool` | Implemented |

## Additional Resources

- **Design Document**: [MachinePool CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20190919-machinepool-api.md)
- **Developer Documentation**: [MachinePool Controller](./../../developer/core/controllers/machine-pool.md)
