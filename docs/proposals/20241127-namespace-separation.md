---
title: Multiple Instances of The Same Provider
authors:
  - "@marek-veber"
reviewers:
  - "@marek-veber"
creation-date: 2024-11-27
last-updated: 2025-01-15
status: provisional
see-also:
replaces:
superseded-by:
---

# Support running multiple instances of the same provider, each one watching different namespaces 

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Glossary](#glossary)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals/Future Work](#non-goalsfuture-work)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
    - [Story 1](#story-1)
    - [Story 2](#story-2)
  - [Requirements (Optional)](#requirements-optional)
    - [Functional Requirements](#functional-requirements)
      - [FR1](#fr1)
      - [FR2](#fr2)
    - [Non-Functional Requirements](#non-functional-requirements)
      - [NFR1](#nfr1)
      - [NFR2](#nfr2)
  - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
  - [Security Model](#security-model)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
- [Upgrade Strategy](#upgrade-strategy)
- [Additional Details](#additional-details)
  - [Test Plan [optional]](#test-plan-optional)
  - [Graduation Criteria [optional]](#graduation-criteria-optional)
  - [Version Skew Strategy [optional]](#version-skew-strategy-optional)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Summary
We need to run multiple CAPI instances in one cluster and divide the namespaces to be watched by given instances.

We want and consider:
- each CAPI instance:
  - is running in separate namespace and is using its own service account
  - can select by command the line arguments the list of namespaces:
    - to watch -  e.g.: `--namespace <ns1> --namespace <ns2>`
    - to exclude from watching - e.g.: `--excluded-namespace <ns1> --excluded-namespace <ns2>`
- we are not supporting multiple versions of CAPI
- all running CAPI-instances:
  - are using the same container image (same version of CAPI)
  - are sharing global resources:
    - CRDs:
      - cluster.x-k8s.io:
        - addons.cluster.x-k8s.io: clusterresourcesetbindings, clusterresourcesets
        - cluster.x-k8s.io: clusterclasses, clusters, machinedeployments, machinehealthchecks, machinepools, machinesets, machines
        - ipam.cluster.x-k8s.io: ipaddressclaims, ipaddresses
        - runtime.cluster.x-k8s.io: extensionconfigs
      - NOTE: the web-hooks are pointing from the CRDs into the first instance only
    - the `ClusterRole/capi-aggregated-manager-role`
    - the `ClusterRoleBinding/capi-manager-rolebinding` to bind all service accounts for CAPI instances (e.g. `capi1-system:capi-manager`, ..., `capiN-system:capi-manager`) to the `ClusterRole`

References:
* https://cluster-api.sigs.k8s.io/developer/core/support-multiple-instances

The proposed PRs implementing such a namespace separation:
* https://github.com/kubernetes-sigs/cluster-api/pull/11397 extend the commandline  option `--namespace=<ns1, …>`
* https://github.com/kubernetes-sigs/cluster-api/pull/11370 add the new commandline option `--excluded-namespace=<ns1, …>`

## Motivation
Our motivation is to have a provisioning cluster which is provisioned cluster at the same time while using hierarchical structure of clusters.
Two namespaces are used by management cluster and the rest of namespaces are watched by CAPI manager to manage other managed clusters.

Our enhancement is also widely required many times from the CAPI community:
* https://github.com/kubernetes-sigs/cluster-api/issues/11192
* https://github.com/kubernetes-sigs/cluster-api/issues/11193

### Goals
We need to extend the existing feature to limit watching on specified namespace.
We need to run multiple CAPI controller instances:
- each watching only specified namespaces: `capi1-system`, …, `capi$(N-1)-system`
- and the last resort instance to watch the rest of namespaces excluding the namespaces already watched by previously mentioned instances   

This change is only a small and strait forward update of the existing feature to limit watching on specified namespace by commandline `--namespace <ns>`


### Non-Goals/Future Work
Non-goals:
* it's not necessary to work with the different versions of CRDs, we consider to:
  * use same version of CAPi (the same container image):
  * share the same CRDs
* the contract and RBAC need to be solved on specific provider (AWS, AZURE, ...)


## Proposal
We are proposing to:
* enable to select multiple namespaces: add `--namespace=<ns1, …>` to extend `--namespace=<ns>` to watch on selected namespaces
  * the code change is only extending an existing hash with one item to multiple items
  * the maintenance complexity shouldn't be extended here
* add the new commandline option `--excluded-namespace=<ens1, …>` to define list of excluded namespaces
  * the code change is only setting an option `Cache.Options.DefaultFieldSelector` to disable matching with any of specified namespace's names
  * the maintenance complexity shouldn't be extended a lot here

### A deployment example
Let's consider an example how to deploy multiple instances:

#### Global resources:
* CRDs (*.cluster.x-k8s.io) - webhooks will point into first instance, e.g.:
  ```yaml
    spec:
      conversion:
        strategy: Webhook
        webhook:
          clientConfig:
            service:
              name: capi-webhook-service
              namespace: capi1-system
              path: /convert
  ```
* `ClusterRole/capi-aggregated-manager-role`
* the `ClusterRoleBinding/capi-manager-rolebinding` binding both service accounts `capi1-system:capi-manager` and `capi2-system:capi-manager` to the cluster role
   ```yaml
   subjects:
   - kind: ServiceAccount                                                                                                                                                       
     name: capi-manager                                                                                                                                                         
     namespace: capi2-system
   - kind: ServiceAccount                                                                                                                                                       
     name: capi-manager                                                                                                                                                         
     namespace: capi1-system  
   ```

#### Namespace `capi1-system`
* `ServiceAccount/capi-manager`
* `Role/capi-leader-election-role`, `RoleBinding/capi-leader-election-rolebinding`
* `MutatingWebhookConfiguration/capi-mutating-webhook-configuration`, `ValidatingWebhookConfiguration/capi-validating-webhook-configuration`
* `Service/capi-webhook-service`
* `Deployment/capi-controller-manager` with the extra args:
  ```yaml
  containers:     
  - args:             
    - --namespace=rosa-a
    - --namespace=rosa-b
  ```

#### Namespace `capi2-system`
* `ServiceAccount/capi-manager`
* `Role/capi-leader-election-role`, `RoleBinding/capi-leader-election-rolebinding`
* `MutatingWebhookConfiguration/capi-mutating-webhook-configuration`, `ValidatingWebhookConfiguration/capi-validating-webhook-configuration`
* `Service/capi-webhook-service`
* `Deployment/capi-controller-manager` with the extra args:
  ```yaml
  containers:     
  - args:             
    - --excluded-namespace=rosa-a
    - --excluded-namespace=rosa-b
  ```

### User Stories
We need to deploy two CAPI instances in the same cluster and divide the list of namespaces to assign some well known namespaces to be watched from the first instance and rest of them to assign to the second instace.

#### Story 1 - RedHat Hierarchical deployment using CAPI
Provisioning cluster which is also provisioned cluster at the same time while using hierarchical structure of clusters.
Two namespaces are used by management cluster and the rest of namespaces are watched by CAPI manager to manage other managed clusters.

RedHat Jira Issues:
* [ACM-15441](https://issues.redhat.com/browse/ACM-15441) - CAPI required enabling option for watching multiple namespaces,
* [ACM-14973](https://issues.redhat.com/browse/ACM-14973) - CAPI controllers should enabling option to ignore namespaces


#### Functional Requirements

##### FR1 - watch multiple namespaces
* It's possible to select a namespace using `--namespace <ns>` to select the namespace to watch now.
* We need to select multiple namespaces using `--namespace <ns1>` ... `--namespace <nsN>` to watch multiple namespaces.

##### FR2 - watch on all namespaces excluding multiple namespaces
* We need to selected excluded namespaces using `--excluded-namespace <ens1>` ... `--namespace <ensN>` to watch on all namespaces excluding selected namespaces.

#### Non-Functional Requirements

We consider that all CAPI instances:
- are using the same container image
- are sharing CRDs and ClusterRole

### Implementation Details/Notes/Constraints

There is a hash `watchNamespaces` of `cache.Config{}` objects [here](https://github.com/kubernetes-sigs/cluster-api/pull/11397/files#diff-2873f79a86c0d8b3335cd7731b0ecf7dd4301eb19a82ef7a1cba7589b5252261L319-R319):
```go
  var watchNamespaces map[string]cache.Config
  ...
  ctrlOptions := ctrl.Options{
      ...
      Cache: cache.Options{
          DefaultNamespaces: watchNamespaces,
          ....
      }
```

#### Current state:
* when `watchNamespaces` == `nil` then all namespaces are watched
* when `watchNamespaces` == `map[string]cache.Config{ watchNamespace: {} }` then only namespace specified by `watchNamespace` is watched

#### Watch on multiple namespaces
We are suggesting to [update](https://github.com/kubernetes-sigs/cluster-api/pull/11397/files#diff-2873f79a86c0d8b3335cd7731b0ecf7dd4301eb19a82ef7a1cba7589b5252261R320-R323) `watchNamespace` to a `watchNamespacesList` list:
```go
   if watchNamespacesList != nil {
        watchNamespaces = map[string]cache.Config{}
        for _, watchNamespace := range watchNamespacesList {
            watchNamespaces[watchNamespace] = cache.Config{}
        }
    }
```
then the namespaces contained in `watchNamespacesList` will be watched by the instance.

#### Exclude watching on selected namespaces
1. We are suggesting to [create the fieldSelector condition](https://github.com/kubernetes-sigs/cluster-api/pull/11370/files#diff-2873f79a86c0d8b3335cd7731b0ecf7dd4301eb19a82ef7a1cba7589b5252261R331-R339) matching all namespaces excluding the list defined in `watchExcludedNamespaces`:
   ```go
    var fieldSelector fields.Selector
    if watchExcludedNamespaces != nil {
        var conditions []fields.Selector
        for i := range watchExcludedNamespaces {
            conditions = append(conditions, fields.OneTermNotEqualSelector("metadata.namespace", watchExcludedNamespaces[i]))
        }
        fieldSelector = fields.AndSelectors(conditions...)
    }
   ```
2. Then we can use the `fieldSelector` to set the `DefaultFieldSelector` value:
    ```go
        ctrlOptions := ctrl.Options{
            ...
            Cache: cache.Options{
                DefaultFieldSelector: fieldSelector,
            ....
        }
    ```
and then the namespaces contained in `watchNamespacesList` will be excluded from watching by the instance.


### Security Model

A service account will be created for each namespace with CAPI instance.
In the simple deployment example we are considering that all CAPI-instances will share the one cluster role `capi-aggregated-manager-role` so all CAPI's service accounts will be bound using then cluster role binding `capi-manager-rolebinding`.
We can also use multiple cluster roles and grant the access more granular only to the namespaces watched by the instance. 

### Risks and Mitigations


- What are the risks of this proposal and how do we mitigate? Think broadly.
- How will UX be reviewed and by whom?
- How will security be reviewed and by whom?
- Consider including folks that also work outside the SIG or subproject.

## Alternatives

The `Alternatives` section is used to highlight and record other possible approaches to delivering the value proposed by a proposal.

## Upgrade Strategy

We don't expect any changes while upgrading.

## Additional Details

### Test Plan

Expectations:
* create only E2E testcases using kind
  * deploy two instances into `capi1-system` and `capi2-system` namespaces
  * crate three namespaces `watch1`, `watch2`, `watch3` for the watching
  * test correct watching: 
    * watch on all namespace (without `--namespace` and `--excluded-namespace`) 
    * watch on namespaces `watch1`, `watch2` (only events in `watch1` and `watch1` no events from `watch3` are accepted):
      * using `--namespace watch1` and `--namespace watch2`  
      * using `--excluded-namespace watch3`


## Implementation History

<!--
- [ ] MM/DD/YYYY: Proposed idea in an issue or [community meeting]
- [ ] MM/DD/YYYY: Compile a Google Doc following the CAEP template (link here)
- [ ] MM/DD/YYYY: First round of feedback from community
- [ ] MM/DD/YYYY: Present proposal at a [community meeting]
- [ ] MM/DD/YYYY: Open proposal PR

[community meeting](https://www.youtube.com/watch?v=COcM5bpbusI)
-->
