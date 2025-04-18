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

# Enable adoption in advance multi tenant or sharding scenarios

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Summary](#summary)
  - [Paradigm 1: Isolated Cluster Management](#paradigm-1-isolated-cluster-management)
  - [Paradigm 2: Centralized Cluster Management](#paradigm-2-centralized-cluster-management)
  - [Challenge: Coexistence of Both Paradigms](#challenge-coexistence-of-both-paradigms)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals/Future Work](#non-goalsfuture-work)
- [Proposal](#proposal)
  - [A deployment example](#a-deployment-example)
    - [Global resources:](#global-resources)
    - [Namespace `capi1-system`](#namespace-capi1-system)
    - [Namespace `capi2-system`](#namespace-capi2-system)
  - [User Stories](#user-stories)
    - [Story 1 - Hierarchical deployment using CAPI:](#story-1---hierarchical-deployment-using-capi)
    - [Story 2 - Isolated Cluster Management](#story-2---isolated-cluster-management)
    - [Story 3 - Centralized Cluster Management](#story-3---centralized-cluster-management)
    - [Story 4 - combination of Isolated and Centralized Cluster Management](#story-4---combination-of-isolated-and-centralized-cluster-management)
    - [Story 5 - Two different versions of CAPI (out of scope):](#story-5---two-different-versions-of-capi-out-of-scope)
    - [Functional Requirements](#functional-requirements)
      - [FR1 - watch multiple namespaces](#fr1---watch-multiple-namespaces)
      - [FR2 - watch on all namespaces excluding multiple namespaces](#fr2---watch-on-all-namespaces-excluding-multiple-namespaces)
    - [Non-Functional Requirements](#non-functional-requirements)
  - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [Current state:](#current-state)
    - [Watch on multiple namespaces](#watch-on-multiple-namespaces)
    - [Exclude watching on selected namespaces](#exclude-watching-on-selected-namespaces)
  - [Security Model](#security-model)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
- [Upgrade Strategy](#upgrade-strategy)
- [Additional Details](#additional-details)
  - [Test Plan](#test-plan)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Summary
As a Service Provider/Consumer, a management cluster is used to provision and manage the lifecycle of Kubernetes clusters using the Kubernetes Cluster API (CAPI).
Two distinct paradigms coexist to address different operational and security requirements.

### Paradigm 1: Isolated Cluster Management
Each Kubernetes cluster operates its own suite of CAPI controllers, targeting specific namespaces as a hidden implementation engine.
This paradigm avoids using webhooks and prioritizes isolation and granularity.

**Key Features**:

- **Granular Lifecycle Management**: Independent versioning and upgrades for each cluster's CAPI components.
- **Logging and Metrics**: Per-cluster logging, forwarding, and metric collection.
- **Resource Isolation**: Defined resource budgets for CPU, memory, and storage on a per-cluster basis.
- **Security Requirements**:
  - **Network Policies**: Per-cluster isolation using tailored policies.
  - **Cloud Provider Credentials**: Each cluster uses its own set of isolated credentials.
  - **Kubeconfig Access**: Dedicated access controls for kubeconfig per cluster.

In the current state there is the option `--namespace=<ns>` and CAPI can watch only one specified namespace while using this option or all namespaces without this option.
The extension to enable the existing command-line option `--namespace=<ns1, …>` define multiple namespaces is proposed in this PR [#11397](https://github.com/kubernetes-sigs/cluster-api/pull/11397).

### Paradigm 2: Centralized Cluster Management
This paradigm manages multiple Kubernetes clusters using a shared, centralized suite of CAPI controllers. It is designed for scenarios with less stringent isolation requirements.

**Characteristics**:

- Operates under simplified constraints compared to [Paradigm 1](#paradigm-1-isolated-cluster-management).
- Reduces management overhead through centralization.
- Prioritizes ease of use and scalability over strict isolation.

The addition of the new command-line option `--excluded-namespace=<ns1, …>` is proposed in this PR [#11370](https://github.com/kubernetes-sigs/cluster-api/pull/11370).

### Challenge: Coexistence of Both Paradigms
To enable [Paradigm 1](#paradigm-1-isolated-cluster-management) and [Paradigm 2](#paradigm-2-centralized-cluster-management) to coexist within the same management cluster, the following is required:

- **Scope Restriction**: [Paradigm 2](#paradigm-2-centralized-cluster-management) must have the ability to restrict its scope to avoid interference with resources owned by [Paradigm 1](#paradigm-1-isolated-cluster-management).
- **Resource Segregation**: [Paradigm 2](#paradigm-2-centralized-cluster-management) must be unaware of CAPI resources managed by [Paradigm 1](#paradigm-1-isolated-cluster-management) to prevent cross-contamination and conflicts.

This coexistence strategy ensures both paradigms can fulfill their respective use cases without compromising operational integrity.

## Motivation
For multi-tenant environment a cluster is used as provision-er using different CAPI providers using CAPI requires careful consideration of namespace isolation
to maintain security and operational boundaries between tenants. In such setups, it is essential to configure the CAPI controller instances
to either watch or exclude specific groups of namespaces based on the isolation requirements.
This can be achieved by setting up namespace-scoped controllers or applying filters, such as label selectors, to define the namespaces each instance should monitor.
By doing so, administrators can ensure that the activities of one tenant do not interfere with others, while also reducing the resource overhead by limiting the scope of CAPI operations.
This approach enhances scalability, security, and manageability, making it well-suited for environments with strict multi-tenancy requirements.

References (related issues):

* https://github.com/kubernetes-sigs/cluster-api/issues/11192
* https://github.com/kubernetes-sigs/cluster-api/issues/11193
* https://github.com/kubernetes-sigs/cluster-api/issues/7775

### Goals
There are some restrictions while using multiple providers, see: https://cluster-api.sigs.k8s.io/developer/core/support-multiple-instances
But we need to:

1. extend the existing "cache configuration" feature `--namespace=<ns>` (limit watching a single namespace) to limit watching on multiple namespaces.
2. add new feature to watch on all namespaces except selected ones.
3. then we run multiple CAPI controller instances:
   - each watching only specified namespaces: `capi1-system`, …, `capi$(N-1)-system`
   - and the last resort instance to watch the rest of namespaces excluding the namespaces already watched by previously mentioned instances

### Non-Goals/Future Work
Non-goals:

* It's not necessary to work with all versions of CAPI/CRDs, we consider:
  * All instances share the same CRDs.
  * All instances must be compatible with the deployed CRDs.
* The contract and RBAC need to be solved on specific provider (AWS, AZURE, ...)
* It is not supported to configure two instances to watch the same namespace:
  * the `--namespace=nsX` option can only be used by a single instance.
* if the same namespace is both excluded and included by the same instance, it will be excluded from being watched by that instance.


## Proposal
We are proposing to:

* enable to select multiple namespaces: add `--namespace=<ns1, …>` to extend `--namespace=<ns>` to watch on selected namespaces
  * the code change involves extending an existing hash to accommodate multiple items.
  * This change is only a small and straightforward update of the existing feature to limit watching on specified namespace. The maintenance complexity shouldn't be extended here
* add the new commandline option `--excluded-namespace=<ens1, …>` to define list of excluded namespaces
  * the code [change](https://github.com/kubernetes-sigs/cluster-api/pull/11370/files#diff-c4604297ff388834dc8c6de30c847f50548cd6dd4b2f546c433b234a27ad4631R263) is only setting an option `Cache.Options.DefaultFieldSelector` to disable matching with any of specified namespace's names
  * the maintenance complexity shouldn't be extended a lot here

Note:

* There is also the existing `--watch-filter=<...>` option, which is used for event filtering, whereas `--namespace=<...>` and `--excluded-namespace=<...>` configure the cache.

Our objectives include:

- Each CAPI instance runs in a separate namespace and uses its own service account.
  - Namespaces can be specified through command-line arguments:
    - To watch: e.g., `--namespace <ns1> --namespace <ns2>`
    - To exclude from watching: e.g., `--excluded-namespace <ns1> --excluded-namespace <ns2>`
- We don't need to support for multiple CAPI versions, but:
  - All instances must be compatible with the deployed CRDs.
  - CRDs are deployed only for the newest CAPI instance (selecting one instance with the newest version).
  - All conversion webhooks in CRDs point to the newest CAPI instance.
  - `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration` are deployed only for the newest CAPI instance and point to it.
  - If instances use different API versions, conversion must be handled correctly by the conversion webhooks.
- All running CAPI instances share global resources:
  - CRDs:
    - `cluster.x-k8s.io`:
      - `addons.cluster.x-k8s.io`: `clusterresourcesetbindings`, `clusterresourcesets`
      - `cluster.x-k8s.io`: `clusterclasses`, `clusters`, `machinedeployments`, `machinehealthchecks`, `machinepools`, `machinesets`, `machines`
      - `ipam.cluster.x-k8s.io`: `ipaddressclaims`, `ipaddresses`
      - `runtime.cluster.x-k8s.io`: `extensionconfigs`
    - NOTE: Webhooks from CRDs point to `Service/capi-webhook-service` of the newest instance only.
  - MutatingWebhookConfiguration and ValidatingWebhookConfiguration point to `Service/capi-webhook-service` of the newest instance only.
- Cluster roles and access management:
  - The default CAPI deployment defines a global cluster role:
    - `ClusterRole/capi-aggregated-manager-role`
    - `ClusterRoleBinding/capi-manager-rolebinding`, binding the service account `<instance-namespace>:capi-manager` for a CAPI instance to the `ClusterRole`
  - In [Paradigm 1: Isolated Cluster Management](#paradigm-1-isolated-cluster-management), we can define a separate cluster role for each instance, granting access only to the namespaces watched by that instance.
  - In [Paradigm 2: Centralized Cluster Management](#paradigm-2-centralized-cluster-management), access to all namespaces is required, as defined in the default CAPI deployment cluster role.
 
### A deployment example
Let's consider an example how to deploy multiple instances for the [Paradigm 1+2](#challenge-coexistence-of-both-paradigms) 

#### Global resources:

* CRDs (*.cluster.x-k8s.io) - webhooks will point into the newest CAPI instance instance, e.g.:
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
* `Service/capi-webhook-service` ... this is unused and probably can be removed
* `Deployment/capi-controller-manager` with the extra args:
  ```yaml
  containers:     
  - args:             
    - --excluded-namespace=rosa-a
    - --excluded-namespace=rosa-b
  ```

### User Stories
#### Story 1 - Hierarchical deployment using CAPI:
In [OCM](https://github.com/open-cluster-management-io/ocm) environment there is `hub Cluster` managing multiple `klusterlets` (managed clusters/ spoke clusters).

See the `Cluster namespace` term definition on [this](https://open-cluster-management.io/docs/concepts/architecture/) page, cite:

* OCM, for each of the `klusterlet` we will be provisioning a dedicated namespace for the managed cluster and grants sufficient RBAC permissions so that the `klusterlet` can persist some data in the hub cluster.
* This dedicated namespace is the `cluster namespace` which is majorly for saving the prescriptions from the hub.
  e.g. we can create ManifestWork in a `cluster namespace` in order to deploy some resources towards the corresponding cluster.
  Meanwhile, the cluster namespace can also be used to save the uploaded stats from the `klusterlet` e.g. the healthiness of an addon, etc.

We need to deploy CAPI instance into the `hub Cluster` watching multiple `cluster namespaces` but exclude `hub-cluster-machines` namespace.

#### Story 2 - Isolated Cluster Management
We need to limit the list of namespaces to watch. It's possible to do this now, but only on one namespace and we need to watch on multiple namespaces by one instance.

#### Story 3 - Centralized Cluster Management
We need to exclude the list of namespaces from watch to reduces management overhead through centralization.

#### Story 4 - combination of Isolated and Centralized Cluster Management
We need to deploy multiple CAPI instances in the same cluster and divide the list of namespaces to assign certain well-known namespaces to be watched from the given instances and define an instance to watch on the rest of them.
E.g.:

* instance1 (deployed in `capi1-system`) is watching `ns1.1`, `ns1.2`, ... `ns1.n1`
* instance2 (deployed in `capi2-system`) is watching `ns2.1`, `ns2.2`, ... `ns2.n2`
* ...
* last-resort instance (deployed in `capiR-system`) is watching the rest of namespaces


#### Story 5 - Two different versions of CAPI (out of scope):
<!-- Jay Pipes -  https://kubernetes.slack.com/archives/C8TSNPY4T/p1723645630888909 --> 

The reason we want to use two instances in one cluster is because we have:
  * a single undercloud cluster that uses metal3 CAPI for the baremetal machine management of that undercloud cluster 
  * and kubevirt CAPI for launching workload clusters on top of that undercloud cluster, using the undercloud cluster's baremetal compute machines to house the workload cluster's VM nodes.

The workload clusters spawned with kubevirt CAPI need to be on a different release cadence than the metal3 CAPI (because our customers want to use the latest/greatest Kubernetes ASAP).
The team that manages the undercloud/baremetal stuff are currently burdened with having to upgrade CAPI frequently
(in order to support the latest kubevirt CAPI providers for the latest Kubernetes releases) and would prefer not to have to upgrade all the time.

With multiple CAPI instances, we need to shard the resources handled by each instance, ideally along namespace boundaries.


#### Functional Requirements

##### FR1 - watch multiple namespaces
* It's possible to select a namespace using `--namespace <ns>` to select the namespace to watch now.
* We need to select multiple namespaces using `--namespace <ns1>` ... `--namespace <nsN>` to watch multiple namespaces.

##### FR2 - watch on all namespaces excluding multiple namespaces
* We need to selected excluded namespaces using `--excluded-namespace <ens1>` ... `--namespace <ensN>` to watch on all namespaces excluding selected namespaces.

#### Non-Functional Requirements

We consider that all CAPI instances:

- are sharing CRDs, MutatingWebhookConfiguration, ValidatingWebhookConfiguration

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

We do not expect any changes while upgrading.

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
