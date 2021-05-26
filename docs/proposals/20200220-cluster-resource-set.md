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
last-updated: 2020-08-05
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

To achieve this, ClusterResourceSet CRD is introduced that will be responsible for applying a set resources defined by users to the matching clusters (`label selectors` will be used to select clusters that the ClusterResourceSet resources will applied to.)


### Goals

- Provide a means to specify a set of resources to apply automatically to newly-created and existing Clusters. Resources will be applied only once.
- Support additions to the resource list by applying the new added resources to both new and existing matching clusters.
- Provide a way to see which ClusterResourceSets are applied to a particular cluster using a new CRD, `ClusterResourceSetBinding`.
- Support both json and yaml resources.

### Non-Goals/Future Work

- Replace or compete with the Cluster Addons subproject.
- Support deletion of resources from clusters. Deleting a resource from a ClusterResourceSet or deleting a ClusterResourceSet does not result in deletion of those resources from clusters.
- Lifecycle management of the installed resources (such as CNI).
- Support reconciliation of resources on resource hash change and/or periodically. This can be a future enhancement work.


## Proposal

### User Stories

#### Story 1

As someone creating multiple clusters, I want some/all my clusters to have a CNI provider of my choosing installed automatically, so I don’t have to manually repeat the installation for each new cluster.

#### Story 2

As someone creating multiple clusters, I want some/all my clusters to have a StorageClass installed automatically, so I don't have to manually repeat the installation for each new cluster.

#### Story 3

As someone creating multiple clusters and using ClusterResourceSet to install some resources, I want to see which resources are applied to my clusters, when they are applied, and if applied successfully.

### Implementation Details/Notes/Constraints

#### Data model changes to existing API types

None. We are planning to implement this feature without modifying any of the existing structure to minimize the footprint of ClusterResourceSet Controller. This enhancement will follow Kubernetes’s feature-gate structure and will be under the experimental package with its APIs, and enabled/disabled with a feature gate. 

#### ClusterResourceSet Object Definition

This is the CRD that has a set of components (resources) to be applied to clusters that match the label selector in it. The label selector cannot be empty.

The resources field is a list of `Secrets`/`ConfigMaps` which should be in the same namespace with `ClusterResourceSet`. The clusterSelector field is a Kubernetes [label selector](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#resources-that-support-set-based-requirements) that matches against labels on clusters (only the clusters in the same namespace with the ClusterResourceSet resource).
ClusterResourceSet is namespace-scoped, all resources and clusters needs to be in the same namespace as the ClusterResourceSet.
*Sample ClusterResourceSet YAML*

```yaml
---
apiVersion: addons.cluster.x-k8s.io/v1alpha3
kind: ClusterResourceSet
metadata:
 name: crs1
 namespace: default
spec:
 mode: "ApplyOnce"
 clusterSelector:
   matchLabels:
     cni: calico
 resources:
   - name: db-secret
     kind: Secret
   - name: calico-addon
     kind: ConfigMap
```

Initially, the only supported mode will be `ApplyOnce` and it will be the default mode if no mode is provided. In the future, we may consider adding a `Reconcile` mode that reapplies the resources on resource hash change and/or periodically.
If ClusterResourceSet resources will be managed by an operator after they are applied by ClusterResourceSet controller, "ApplyOnce" mode must be used so that reconciliation on those resources can be delegated to the operator.

Each item in the resources specifies a kind (must be either ConfigMap or Secret) and a name. Each referenced ConfigMap/Secret contains yaml/json content as value.
`ClusterResourceSet` object will be added as owner to its resources.

*** Secrets as Resources***

Both `Secrets` and `ConfigMaps` `data` fields can be a list of key-value pairs. Any key is acceptable, and as value, there can be multiple objects in yaml or json format.

For preventing all secrets to be reached by all clusters in a namespace, only secrets with type `addons.cluster.x-k8s.io/resource-set` can be accessed by ClusterResourceSet controller.
Secrets are preferred if the data includes sensitive information.

An easy way to create resource `Secrets` is to have a yaml or json file with the components.

E.g., this is `db.yaml` that has multiple objects:
```
kind: Secret
apiVersion: v1
metadata:
 name: mysql-access
 namespace: system
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
 name: db-admin
 namespace: system
```

We can create a secret that has these components in its data field to be used in `ClusterResourceSet`:

` #kubectl create secret generic db-secret --from-file=db.yaml --type=addons.cluster.x-k8s.io/resource-set`

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: db-secret
type: addons.cluster.x-k8s.io/resource-set
stringData:
  db.yaml: |-
    kind: Secret
    apiVersion: v1
    metadata:
     name: mysql-access
     namespace: system
    ---
    kind: ClusterRole
    apiVersion: rbac.authorization.k8s.io/v1
    metadata:
     name: db-admin
     namespace: system
```

*** ConfigMaps as Resources***

Similar to `Secrets`, `ConfigMaps` can be created using a yaml/json file: `kubectl create configmap calico-addon --from-file=calico1.yaml,calico2.yaml`
Multiple keys in the data field and then multiple objects in each value are supported.
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: calico-addon
data:
  calico1.yaml: |-
     kind: Secret
     apiVersion: v1
     metadata:
      name: calico-secret1
      namespace: mysecrets
      ---
     kind: Secret
     apiVersion: v1
     metadata:
      name: calico-secret2
      namespace: mysecrets
  calico2.yaml: |-
     kind: ConfigMap
     apiVersion: v1
     metadata:
      name: calico-configmap
      namespace: myconfigmaps
```

#### ClusterResourceSetBinding Object Definition
The resources in `ClusterResourceSet` will be applied to matching clusters.
There is many-to-many mapping between Clusters and `ClusterResourceSets`: Multiple `ClusterResourceSets` can match with a cluster; and multiple clusters can match with a single `ClusterResourceSet`.
To keep information on which resources applied to which clusters, a new CRD is used, `ClusterResourceSetBinding` will be created in the management cluster. There will be one `ClusterResourceSetBinding` per workload cluster.
ClusterResourceBinding's name will be same with the `Cluster` name. Both `Cluster` and the matching `ClusterResourceSets` will be added as owners to the ClusterResourceBinding.

Example `ClusterResourceBinding` object:
```yaml
apiVersion: v1
kind: ClusterResourceBinding
metadata:
  name: <cluster-name>
  namespace: <cluster-namespace>
 ownerReferences:
  - apiVersion: cluster.x-k8s.io/v1alpha3
    kind: Cluster
    name: <cluster-name>
    uid: e3a503a8-9be1-4264-8fa2-d536532687f9
  - apiVersion: addons.cluster.x-k8s.io/v1alpha3
    blockOwnerDeletion: true
    controller: true
    kind: ClusterResourceSet
    name: crs1
    uid: 62c77639-92d8-46d2-ba21-a880f62f7719
spec:
  bindings:
  - clusterResourceSetName: crs1
    resources:
    - applied: true
      hash: sha256:a3473f4e92ee5a2277ff37d5c559666d61d24332a497b554e65ae18e82727245
      kind: Secret
      lastAppliedTime: "2020-07-02T05:47:38Z"
      name: db-secret
    - applied: true
      hash: sha256:c1d0dc7e51bb05945a2f99e6745dc4b1043f8a03f37ad21391fe92353a02066e
      kind: ConfigMap
      lastAppliedTime: "2020-07-02T05:47:39Z"
      name: calico-addon
```
When a cluster is deleted, the associated `ClusterResourceBinding` will also be cleaned up.
When a `ClusterResourceSet` is deleted, it will be removed from the `bindings` list of all `ClusterResourceBindings` that it is listed.

`ClusterResourceSet` will use `ClusterResourceSetBinding` to decide to apply a new resource or retry to apply an old one. In `ApplyOnce` mode, if a `resource/applied` is true,
 that resource will never be reapplied. If applying a resource is failed, `ClusterResourceSet` controller will reconcile it and use the `controller-runtime`'s exponential back-off to retry applying failed resources.
In case of new resource addition to a `ClusterResourceSet`, that `ClusterResourceSet` will be reconciled immediately and the new resource will be applied to all matching clusters because
the new resource does not exist in any `ClusterResourceBinding` lists.

When the same resource exist in multiple `ClusterResourceSets`, only the first one will be applied but the resource will appear as applied in all `ClusterResourceSets` in the `ClusterResourceSetsBinding/bindings`.
Similarly, if a resource is manually created in the workload cluster, when a `ClusterResourceSet` is applied with that resource, it will not update the existing resource to avoid any overwrites but in `ClusterResourceSetBinding`, that resource will show as applied.

As a note, if providing different values for some fields in the resources for different clusters is needed such as CNIs podCIDRs, some templating mechanism for variable substitution in the resources can be used, but not provided by `ClusterResourceSet`.

### Risks and Mitigations

## Alternatives

The Alternatives section is used to highlight and record other possible approaches to delivering the value proposed by a proposal.

## Upgrade Strategy

This is an experimental feature supported by a new CRD and controller so there is no need to handle upgrades for existing clusters.

## Additional Details

### Test Plan [optional]

Extensive unit testing for all the cases supported when applying `ClusterResourceSet` resources.
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
