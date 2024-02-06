# Provider contract

The __Cluster API contract__ defines a set of rules a provider is expected to comply with in order to interact with Cluster API.
Those rules can be in the form of CustomResourceDefinition (CRD) fields and/or expected behaviors to be implemented.

Different rules apply to each provider type and for each different resource that is expected to interact with "core" Cluster API.

- Infrastructure provider
  - Contract rules for [InfraCluster](infra-cluster.md) resource
  - Contract rules for [InfraMachine](infra-machine.md) resource
  - Contract rules for InfraMachinePool resource (TODO)
  
- Bootstrap provider
  - Contract rules for [BootstrapConfig](bootstrap-config.md) resource

- Control plane provider
  - Contract rules for [ControlPlane](control-plane.md) resource

- IPAM provider
  - Contract rules for [IPAM](ipam.md) resources
  
- Addon Providers
  - [Cluster API Add-On Orchestration](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220712-cluster-api-addon-orchestration.md)

- Runtime Extensions Providers
  - [Experimental Feature: Runtime SDK (alpha)](https://cluster-api.sigs.k8s.io/tasks/experimental-features/runtime-sdk/)
  
Additional rules must be considered for a provider to work with the [clusterctl CLI](clusterctl.md).

## Improving and contributing to the contract

The definition of the contract between Cluster API and providers may be changed in future versions of Cluster API. 
The Cluster API maintainers welcome feedback and contributions to the contract in order to improve how it's defined, 
its clarity and visibility to provider implementers and its suitability across the different kinds of Cluster API providers. 
To provide feedback or open a discussion about the provider contract please [open an issue on the Cluster API](https://github.com/kubernetes-sigs/cluster-api/issues/new?assignees=&labels=&template=feature_request.md) 
repo or add an item to the agenda in the [Cluster API community meeting](https://git.k8s.io/community/sig-cluster-lifecycle/README.md#cluster-api).
