---
title: ClusterResourceSet
authors:
 - "@sedefsavas"
reviewers:
 - "@vincepri"
 - "@detiber"
 - "@ncdc"
 - "@fabriziopandini"
creation-date: 2020-02-20
last-updated: 2020-05-11
status: experimental
---

# ClusterResourceSet

## Table of Contents
* [ClusterResourceSet](#clusterresourceset)
  * [Table of Contents](#table-of-contents)
  * [Glossary](#glossary)
  * [Summary](#summary)
  * [Motivation](#motivation)
     * [Goals](#goals)
     * [Non-Goals/Future Work](#non-goalsfuture-work)
  * [Proposal](#proposal)
     * [User Stories](#user-stories)
        * [Story 1](#story-1)
        * [Story 2](#story-2)
        * [Story 3](#story-3)
     * [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
        * [Data model changes](#data-model-changes)
        * [ClusterResourceSet Object Definition](#clusterresourceset-object-definition)
     * [Risks and Mitigations](#risks-and-mitigations)
  * [Alternatives](#alternatives)
  * [Upgrade Strategy](#upgrade-strategy)
  * [Additional Details](#additional-details)
     * [Test Plan [optional]](#test-plan-optional)
     * [Graduation Criteria [optional]](#graduation-criteria-optional)
  * [Implementation History](#implementation-history)

  
## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

## Summary

Provide a mechanism for applying resources in a cluster once it is created.

## Motivation

Clusters created by Cluster API are minimally functional. For instance,they do not have a container networking interface (CNI), which is required for pod-to-pod networking, or any StorageClasses, which are required for dynamic persistent volume provisioning.
Users today must manually add these components to every cluster they create.

Having a mechanism to apply an initial set of default resources after clusters are created makes clusters created with Cluster API functional and ready for workloads from the beginning, without requiring additional user intervention.

To achieve this, ClusterResourceSet CRD is introduced that will be responsible for applying a set resources defined by users to the matching clusters.

### Goals

- Provide a means to specify a set of resources to apply automatically to newly-created and existing Clusters. Resources will be applied only once.
- Support additions to the resource list by applying the new added resources to both new and existing matching clusters.
- Support both json and yaml resources.

### Non-Goals/Future Work

- Replace or compete with the Cluster Addons subproject.
- Support deletion of resources from clusters. Deleting a resource from a ClusterResourceSet or deleting a ClusterResourceSet does not result in deletion of those resources from clusters.
- Lifecycle management of the installed resources (such as CNI).
- Support reconciliation of resources on resource hash change and/or periodically. This can be a future enhancement work.


## Proposal

### User Stories

#### Story 1

As someone creating multiple clusters, I want all my clusters to have a CNI provider of my choosing installed automatically, so I don’t have to manually repeat the installation for each new cluster.

#### Story 2

As someone creating multiple clusters, I want all my clusters to have a StorageClass installed automatically, so I don't have to manually repeat the installation for each new cluster.

#### Story 3

As someone creating multiple clusters, I want to be able to provide different values for some fields in the resources for different clusters. For example, CNIs podCIDRs  may be required to be distinct for each cluster, hence some templating mechanism for variable substitution in the resources is needed.

### Implementation Details/Notes/Constraints

#### Data model changes

None. We are planning to implement this feature without modifying any of the existing structure to minimize the footprint of ClusterResourceSet Controller. This enhancement will follow Kubernetes’s feature-gate structure and will be under the experimental package with its APIs, and enabled/disabled with a feature gate. 

#### ClusterResourceSet Object Definition

This is the CRD that is used to have a set of components that will be applied to clusters that match the label selector in it.

The resources field is a list of secrets/configmaps in the same namespace. The clusterSelector field is a Kubernetes [label selector](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#resources-that-support-set-based-requirements) that matches against labels on clusters (in any namespace).

*Sample ClusterResourceSet YAML*

```yaml
---
apiVersion: cluster.x-k8s.io/v1alpha3
kind: ClusterResourceSet
metadata:
 name: postcreate-conf
 namespace: default
spec:
 mode: "ApplyOnce"
 clusterSelector:
   matchLabels:
     postcreatelabelcni: calico
 resources:
   - name: calico-addon
     kind: Secret
   - name: network-policy-addon
     kind: ConfigMap
status:
 LastUpdated:  "2020-05-05T08:24:17Z"
```

Initially, the only supported mode will be `ApplyOnce` and it will be the default mode if no mode is provided. In the future, we may consider adding a `Reconcile` mode that reapplies the resources on resource hash change and/or periodically.
If ClusterResourceSet resources will be managed by an operator after they are applied by ClusterResourceSet controller, "ApplyOnce" mode must be used so that reconciliation on those resources can be delegated to the operator.

Each item in the resources specifies a kind (must be either ConfigMap or Secret) and a name. Each referenced ConfigMap/Secret  contains yaml/json content as value. The key to that content is “value”.

*Sample Secret Format*
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: calico-addon
type: Opaque
stringData:
  value: |-
    kind: ConfigMap
    apiVersion: v1
    metadata:
     name: calico-conf
```

The resources in ClusterResourceSet will be applied to matching clusters.
There is many-to-many mapping between Clusters and ClusterResourceSets: Multiple ClusterResourceSets can match with a cluster; and multiple clusters can match with a single ClusterResourceSet.
A configmap will be created in the management cluster to keep track of which resources are applied by ClusterResourceSet resources. There will be one configmap per workload cluster.

Example:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: capi-crs-my-cluster
  namespace: capi-crs-my-cluster-namespace
data:
  <ClusterResourceSet-name1>:   
    <secret-name1>:
      hash: <>
      status: success
      error: ""
      lastAppliedTime: "2020-04-05T08:24:17Z"
    <configmap-name1>:
      hash: <>
      status: failed
      error: "some error"
      lastAppliedTime: "2020-05-05T08:24:17Z"
  <ClusterResourceSet-name2>:  
    <secret-name2>:
      hash: <>
      status: success
      error: ""
      lastAppliedTime: "2020-04-05T08:24:17Z"
  Status: InProgress/Completed
```
Status will be `Completed` when all matching ClusterResourceSet reconciles are completed for that cluster. In case of new resource addition to a matching ClusterResourceSet, Status becomes `InProgress`
Also, the errors / overall progress will be tracked in the ClusterResourceSet’s status.

### Risks and Mitigations

## Alternatives

The Alternatives section is used to highlight and record other possible approaches to delivering the value proposed by a proposal.

## Upgrade Strategy

This is an experimental feature supported by a new CRD and controller so there is no need to handle upgrades for existing clusters.

## Additional Details

### Test Plan [optional]

Extensive unit testing for all the cases supported when applying ClusterResourceSet resources.
e2e testing as part of the cluster-api e2e test suite.

### Graduation Criteria [optional]

This proposal will follow all maturity stages (alpha, beta, GA) and then may be merged with cluster-api apis and controllers.

## Implementation History
- [x] 02/20/2020: Compile a [CAEP Google Doc] following the CAEP template
- [x] 02/26/2020: Present proposal at a [community meeting]
- [x] 05/11/2020: Open proposal PR

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1fQNlqsDkvEggWFi51GVxOglL2P1Bvo2JhZlMhm2d-Co/edit
[CAEP Google Doc]: https://docs.google.com/document/d/1lWLGN66roMjXL49gKO6Dhwe7yzCnvgrCtjR9mu4rvPc/edit?ts=5eb07925#
[issue 2395]:  https://github.com/kubernetes-sigs/cluster-api/issues/2395
[POC]: https://github.com/sedefsavas/cluster-api/pull/3
