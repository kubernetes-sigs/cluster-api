# IPAM Provider Specification

## Overview

The IPAM provider is responsible for handling the IP addresses for the machines in a cluster.

IPAM providers are optional when using Cluster API. Infrastructure providers need to [implement explicit support](#infrastructure-provider) to be usable in conjunction with IPAM providers.

<aside class="note">

Note that the IPAM contract is single-stack. If you need both v4 and v6 addresses, two pools and two IPAddressClaims are necessary.

</aside>

## Data Types

An IPAM provider must define one or more API types for IP address pools. The types:

1. Must belong to an API group served by the Kubernetes apiserver 
2. Must be implemented as a CustomResourceDefinition. 
   The CRD name must have the format produced by `sigs.k8s.io/cluster-api/util/contract.CalculateCRDName(Group, Kind)`. 
3. Must have the standard Kubernetes "type metadata" and "object metadata"
4. Should have a status.conditions field with the following:
   1. A Ready condition to represent the overall operational state of the component. It can be based on the summary of more detailed conditions existing on the same object, e.g. instanceReady, SecurityGroupsReady conditions.

## Behaviour

IPAM providers must handle any IPAddressClaim resources that reference IP address pools that are managed by the provider and create an IPAddress resource for it. IPAddressClaims are usually created by infrastructure providers.

### IPAM Provider

An IPAM provider must watch for new, updated and deleted IPAddressClaims that reference an IP address pool that is manged by the provider in their `spec.poolRef` field.

#### Normal IPAddressClaim

1. If the IPAddressClaim does not reference a pool managed by the provider in it's `spec.poolRef`, abort the reconciliation.
2. If the related Cluster is paused, abort reconciliation
   1. The related Cluster is referenced using the `spec.clusterName` field or a `cluster.x-k8s.io/cluster-name` label (the latter is deprecated).
   2. If the paused field is empty and the `cluster.x-k8s.io/paused` annotation is not present, reconciliation can continue.
   3. If the referenced cluster is not found, abort reconciliation.
   4. If the referenced cluster has `spec.paused` set or a `cluster.x-k8s.io/paused` annotation, skip reconciliation
3. Add any required provider-specific finalziers (you probably need one)
4. Allocate an IP address for the claim
5. Create an IPAddress object
   1. It should have the same name as the claim.
   2. It must have a owner reference with `controller: true` and `blockOwnerDeletion: true` to the Claim
   3. It must have a owner reference with `controller: false` and `blockOwnerDeletion: true` to the referenced Pool
   4. It should have a Finalizer that prevents accidental deletion, e.g. `ipam.cluster.x-k8s.io/protect-address`.
6. Set the `status.addressRef` on the IPAddressClaim to the created IPAddress

#### Deleted IPAddressClaim

1. If the related Cluster is paused, abort reconciliation (see 2. above)
2. Deallocate the IP address
3. Delete the IPAddress object
   1. Remove any Finalizers that were set to prevent deletion
4. Remove the Finalizer from the claim

#### Clusterctl Move

In order for Pools to be moved alongside clusters, they need to have a `cluster.x-k8s.io/cluster-name` label.

### Infrastructure Provider

In order to consume IP addresses from an IP address pool, an IPAddressClaim resource needs to be created, which will then be fulfilled with an IPAddress resource. Since the IPAddressClaim needs to reference an IP pool, you'll need to add a property to your infrastructure Machine that allows to specify the pool.

<aside class="note">

Note that the IPAM contract is single-stack. If you need both v4 and v6 addresses, two pools and two IPAddressClaims are necessary.

</aside>

1. Create an IPAddressClaim
   1. The `spec.poolRef` must reference the pool you want to use
   2. It should have an owner reference to the infrastructure Machine (or the intermediate resource) it is created for (required to support `clusterctl move`). The reference should have `controller: true` and `blockOwnerDeletion: true` set.
   3. It's `spec.clusterName` field should be set (or it should have a `cluster.x-k8s.io/cluster-name` label)
   4. Ideally it's name is derived from the infrastructure Machine's name
2. Wait until an IP is allocated, ideally by watching the IPAddressClaim and waiting for `status.addressRef` to be set
3. Fetch the IPAddress resource which contains the allocated address

When the infrastructure Machine is deleted, the claim should be deleted as well. The infrastructure Machine deletion should be blocked until the claim is deleted (handled by the API server if the owner relation is set up correctly).
