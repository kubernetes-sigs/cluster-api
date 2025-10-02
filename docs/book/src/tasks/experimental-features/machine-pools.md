# Experimental Feature: MachinePool (beta)

The `MachinePool` feature provides a way to manage a set of machines by leveraging infrastructure provider scaling groups (e.g., AWS Auto Scaling Groups, Azure VM Scale Sets) rather than managing individual machines through MachineDeployments.

**Feature gate name**: `MachinePool`

**Variable name to enable/disable the feature gate**: `EXP_MACHINE_POOL`

## Overview

Infrastructure providers can support this feature by implementing their specific `MachinePool` such as `AWSMachinePool` or `AzureMachinePool`.

📖 **For comprehensive information about MachinePool concepts, use cases, and comparisons with MachineDeployment**, see the [MachinePool Concepts Guide](../../concepts/machinepool.md).

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

| Provider | Implementations | Status | Documentation |
| --- | --- | --- | --- |
| AWS | `AWSManagedMachinePool`<br> `AWSMachinePool` | Implemented, MachinePoolMachines supported | https://cluster-api-aws.sigs.k8s.io/topics/machinepools.html|
| Azure | `AzureASOManagedMachinePool`<br> `AzureManagedMachinePool`<br> `AzureMachinePool` | Implemented, MachinePoolMachines supported | https://capz.sigs.k8s.io/self-managed/machinepools |
| GCP | `GCPMachinePool` | In Progress | https://github.com/kubernetes-sigs/cluster-api-provider-gcp/pull/1506 |
| OCI | `OCIManagedMachinePool`<br> `OCIMachinePool` | Implemented, MachinePoolMachines supported | https://oracle.github.io/cluster-api-provider-oci/managed/managedcluster.html |
| Scaleway | `ScalewayManagedMachinePool` | Implemented | https://github.com/scaleway/cluster-api-provider-scaleway/blob/main/docs/scalewaymanagedmachinepool.md |

## Additional Resources

- **Design Document**: [MachinePool CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20190919-machinepool-api.md)
- **Developer Documentation**: [MachinePool Controller](./../../developer/core/controllers/machine-pool.md)