---
title: Improving status in CAPI resources
authors:
- "@fabriziopandini"
reviewers:
- "@neolit123"
- "@enxebre"
- "@JoelSpeed"
- "@vincepri"
- "@sbueringer"
- "@chrischdi"
- "@peterochodo"
- "@zjs"
creation-date: 2024-07-17
last-updated: 2024-09-16
status: implementable
see-also:
- [Proposal about custom Cluster API conditions (superseded by this document)](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20200506-conditions.md)
- [Kubernetes API guidelines](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [Kubernetes API deprecation rules](https://kubernetes.io/docs/reference/using-api/deprecation-policy/#fields-of-rest-resources)
---

# Improving status in CAPI resources

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Summary](#summary)
- [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
    - [Readiness and Availability](#readiness-and-availability)
    - [Transition to Kubernetes API conventions aligned conditions](#transition-to-kubernetes-api-conventions-aligned-conditions)
    - [Phases field/print column](#phases-fieldprint-column)
    - [Changes to Machine resource](#changes-to-machine-resource)
      - [Machine Status](#machine-status)
        - [Machine (New)Conditions](#machine-newconditions)
      - [Machine Spec](#machine-spec)
      - [Machine Print columns](#machine-print-columns)
    - [Changes to MachineSet resource](#changes-to-machineset-resource)
      - [MachineSet Status](#machineset-status)
        - [MachineSet (New)Conditions](#machineset-newconditions)
      - [MachineSet Spec](#machineset-spec)
      - [MachineSet Print columns](#machineset-print-columns)
    - [Changes to MachineDeployment resource](#changes-to-machinedeployment-resource)
      - [MachineDeployment Status](#machinedeployment-status)
        - [MachineDeployment (New)Conditions](#machinedeployment-newconditions)
      - [MachineDeployment Spec](#machinedeployment-spec)
      - [MachineDeployment Print columns](#machinedeployment-print-columns)
    - [Changes to Cluster resource](#changes-to-cluster-resource)
      - [Cluster Status](#cluster-status)
        - [Cluster (New)Conditions](#cluster-newconditions)
      - [Cluster Spec](#cluster-spec)
      - [Cluster Print columns](#cluster-print-columns)
    - [Changes to KubeadmControlPlane (KCP) resource](#changes-to-kubeadmcontrolplane-kcp-resource)
      - [KubeadmControlPlane Status](#kubeadmcontrolplane-status)
        - [KubeadmControlPlane (New)Conditions](#kubeadmcontrolplane-newconditions)
      - [KubeadmControlPlane Print columns](#kubeadmcontrolplane-print-columns)
    - [Changes to MachinePool resource](#changes-to-machinepool-resource)
      - [MachinePool Status](#machinepool-status)
        - [MachinePool (New)Conditions](#machinepool-newconditions)
      - [MachinePool Spec](#machinepool-spec)
      - [MachinePool Print columns](#machinepool-print-columns)
    - [Changes to Cluster API contract](#changes-to-cluster-api-contract)
      - [Contract for infrastructure providers](#contract-for-infrastructure-providers)
        - [InfrastructureCluster](#infrastructurecluster)
        - [InfrastructureMachine](#infrastructuremachine)
      - [Contract for bootstrap providers](#contract-for-bootstrap-providers)
      - [Contract for control plane providers](#contract-for-control-plane-providers)
    - [Example use cases](#example-use-cases)
    - [Security Model](#security-model)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Alternatives](#alternatives)
  - [Upgrade Strategy](#upgrade-strategy)
  - [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Summary

This document defines how status across all Cluster API resources is going to evolve in the v1beta2 API version, focusing on
improving usability and consistency across different resources in CAPI and with the rest of the ecosystem.

# Motivation

The Cluster API community recognizes that nowadays most users are rightfully focused on
building higher level systems, offerings, and applications on these platforms.

However, as the focus shifted away, most of the users don’t have time to become deep experts on Cluster API.

This trend is blurring the lines between different Cluster API components; between Cluster API and Kubernetes, and tools
like Helm, Flux, Argo, and so on.

This proposal focused on Cluster API's resource status which must become simpler to understand, more consistent with
Kubernetes, and ideally with the entire ecosystem.

### Goals

- Review and standardize the usage of the concept of readiness across Cluster API resources.
  - Drop or amend improper usage of readiness
  - Make the concept of Machine readiness extensible, thus allowing providers or external systems to inject their readiness checks.
- Review and standardize the usage of the concept of availability across Cluster API resources.
  - Make the concept of Cluster availability extensible, thus allowing providers or external systems to inject their availability checks.
- Bubble up more information about both control plane and worker Machines, ensuring consistency across Cluster API resources.
  - Bubble up conditions about Machine readiness to control plane, MachineDeployment, MachinePool.
  - Standardize replica counters on control plane, MachineDeployment, MachinePool.
  - Ensure the Cluster resource will have enough information about controlled objects, which is crucial for users
    relying on managed topologies (where the Cluster resource is the single point of control for the entire hierarchy of objects)
- Introduce missing signals about connectivity to workload clusters, thus enabling to mark all the conditions
  depending on such connectivity with status Unknown after a certain amount of time.
- Introduce a cleaner signal about Cluster API resources lifecycle transitions, e.g. scaling up or updating.
- Ensure everything in status can be used as a signal informing monitoring tools/automation on top of Cluster API
  about lifecycle transitions/state of the Cluster and the underlying components as well.

### Non-Goals

- Resolving all the idiosyncrasies that exists in Cluster API, core Kubernetes, the rest of the ecosystem.
  (Let’s stay focused on Cluster API and keep improving incrementally).
- To change fundamental way how the Cluster API contract with infrastructure, bootstrap and control providers currently works
  (by using status fields).

## Proposal

This proposal groups a set of changes to status fields in Cluster API resources.

Proposed changes are designed to introduce benefits for Cluster API users as soon as possible, but considering the
API deprecations rules, it is required to go through a multi-step transition to reach the desired shape of the API resources.
Such transition is detailed in the following paragraphs.

At high level, proposed changes to status fields can be grouped in three sets of changes:

Some of those changes could be considered straight forward, e.g.

- K8s resources do not have a concept similar to "terminal failure" in Cluster API resources, and users approaching
  the project are struggling with this idea. In some cases also provider's implementers are struggling with it.
  Accordingly, Cluster API resources are dropping `FailureReason` and `FailureMessage` fields.
  Like in K8s objects, "terminal failures" should be surfaced using conditions, with a well documented type/reason representing
  a "terminal failure"; it is up to consumers to treat them accordingly. There is no special treatment for these conditions within Cluster API.
- Bubble up more information about Machines to the owner resource (control plane, MachineSet, MachineDeployment, MachinePool)
  and then to the Cluster.

Some other changes require a little bit more context, which is provided in following paragraphs:

- Review and standardize the usage of the concept of readiness and availability to align to K8s API conventions /
  conditions used in core K8s objects like `Pod`, `Node`, `Deployment`, `ReplicaSet` etc.
- Transition to K8s API conventions fully aligned condition types/condition management (and thus deprecation of
  the Cluster API "custom" guidelines for conditions).

The last set of changes is a consequence of the above changes, or small improvements to address feedback received
over time; changes in this group will be detailed case by case in the following paragraphs, a few examples:

- Change the semantic of ReadyReplicas counters to use Machine's Ready condition instead of Node's Ready condition.
  (so everywhere Ready is used for a Machine it always means the same thing)
- Add a new condition monitoring the status of the connectivity to workload clusters (`RemoteConnectionProbe`).

In order to keep making progress on this proposal, the first iteration will be focused on:

- Machines
- MachineSets
- MachineDeployments
- MachinePools
- KubeadmControlPlane (ControlPlanes)
- Clusters

Other resources will be added as soon as there is agreement on the general direction.

Overall, the union of all those changes is expected to greatly improve status fields, conditions, replica counters
and print columns.

These improvements are expected to provide benefit to users interacting with the system, using monitoring tools, and
building higher level systems or products on top of Cluster API.

### Readiness and Availability

The [condition CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20200506-conditions.md) in Cluster API introduced very strict requirements about `Ready` conditions, mandating it
to exists on all resources and also mandating that `Ready` must be computed as the summary of all other existing
conditions.

However, over time Cluster API maintainers recognized several limitations of the “one fits all”, strict approach.

E.g., higher level abstractions in Cluster API are designed to remain operational during lifecycle operations,
for instance a MachineDeployment is operational even if it is rolling out.

But the use cases above were hard to combine with the strict requirement to have all the conditions true, and
as a result today Cluster API resources barely have conditions surfacing that lifecycle operations are happening, or where
those conditions are defined they have a semantic which is not easy to understand, like e.g. `Resized` or `MachinesSpecUpToDate`.

E.g., when you look at higher level abstractions in Cluster API like Clusters, MachineDeployments and ControlPlanes, readiness
might be confusing, because those resources usually accept a certain degree of not readiness, e.g. MachineDeployments are
usually ok even if a few Machines are not ready (up to `MaxUnavailable`).

In order to address this problem, Cluster API is going to align to K8s API conventions. As a consequence, the `Ready`
condition won't have to exist on all resources anymore. Nor when it exists, it will be required to include
all the existing conditions when calculating the `Ready` condition.

As a consequence, we will continue to use the ready condition *only* where it makes sense, and with a well-defined
semantic that conveys important information to the users (vs applying "blindly" the same formula everywhere).

The most important effect of this change is the definition of a new semantic for the Machine's `Ready` condition, that
will now clearly represent the "machine can host workloads" (prior art Kubernetes nodes are ready when "node can host pods").
To improve the benefit of this change:

- This proposal is ensuring that whenever Machine ready is used, it always means the same thing (e.g. ready replica counters)
- This proposal is also changing contract fields where ready was used to represent initial provisioning of infrastructure
  or bootstrap secrets (so ready had different meanings).

All in all, Machine's Ready concept should be much more clear, consistent, intuitive after proposed changes.
But there is more.

This proposal is also dropping the `Ready` condition from higher level abstractions in Cluster API.

Instead, where not already present, this proposal is introducing a new `Available` condition that better represents
the fact that those objects are operational even if there is a certain degree of not readiness / disruption in the system
or if lifecycle operations are happening (prior art: `Available` condition in K8s Deployments).

Last but not least:

- With the changes to the semantic of `Ready` and `Available` conditions, it is now possible to add conditions to
  surface ongoing lifecycle operations, e.g. scaling up.
- As suggested by K8s API conventions, this proposal is also making sure all conditions are consistent and have
  uniform meaning across all resource types
- Additionally, we are enforcing the same consistency for replica counters and other status fields.

### Transition to Kubernetes API conventions aligned conditions

Kubernetes is undergoing a long term effort of standardizing usage of conditions across all resource types, and the
transition to the v1beta2 API version is a great opportunity for Cluster API to align to this effort.

The value of this transition is substantial, because the differences that exist today are really confusing for users.
These differences are also making it harder for ecosystem tools to build on top of Cluster API, and in some cases
even confusing new (and old) contributors.

With this proposal Cluster API will close the gap with Kubernetes API conventions in regard to:
- Polarity: Condition type names should make sense for humans; neither positive nor negative polarity can be recommended
  as a general rule (already implemented by [#10550](https://github.com/kubernetes-sigs/cluster-api/pull/10550))
- Use of the `Reason` field is required (currently in Cluster API reasons is added only when condition are false)
- Controllers should apply their conditions to a resource the first time they visit the resource, even if the status is `Unknown`.
  (currently Cluster API controllers add conditions at different stages of the reconcile loops). Please note that:
  - If more than one controller adds conditions to the same resources, conditions managed by the different controllers will be
    applied at different times.
  - Kubernetes API conventions account for exceptions to this rule; for known conditions, the absence of a condition status should
    be interpreted the same as `Unknown`, and typically indicates that reconciliation has not yet finished.
- Cluster API is also dropping its own `Condition` type and will start using `metav1.Conditions` from the Kubernetes API.

The last point also has another implication, which is the removal of the `Severity` field which is currently used
to determine priority when merging conditions into the `Ready` summary condition.

However, considering all the work to clean up and improve readiness and availability, now dropping the `Severity` field
is not an issue anymore. Let's clarify this with an example:

When Cluster API will compute Machine `Ready` there will be a very limited set of conditions
to merge (see [next paragraph](#machine-newconditions)). Considering this, it will be probably simpler and more informative
for users if we surface all relevant messages instead of arbitrarily dropping some of them as we are doing
today by inferring merge priority from the `Severity` field.

In case someone wants a more sophisticated control over the process of merging conditions, the new version of the
condition utils in Cluster API will allow developers to plug in custom functions to compute merge priority
for a condition, e.g. by looking at status, reason, time since the condition transitioned, etc.

### Phases field/print column

K8s API conventions suggest to deprecate and remove `phase` fields from status.

However, Cluster API maintainers decided to not align to this recommendation because there is consensus that
existing `phase` fields provide valuable information to users.

### Changes to Machine resource

#### Machine Status

Following changes are implemented to Machine's status:

- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions

Below you can find the relevant fields in MachineStatus v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type MachineStatus struct {

    // Initialization provides observations of the Machine initialization process.
    // NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Machine provisioning.
    // The value of those fields is never updated after provisioning is completed.
    // Use conditions to monitor the operational state of the Machine.
    // +optional
    Initialization *MachineInitializationStatus `json:"initialization,omitempty"`
    
    // Conditions represent the observations of a Machine's current state.
    // +optional
    // +listType=map
    // +listMapKey=type
    // +kubebuilder:validation:MaxItems=32
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    
    // Other fields...
    // NOTE: `FailureReason`, `FailureMessage`, `BootstrapReady`, `InfrastructureReady` fields won't be there anymore
}

// MachineInitializationStatus provides observations of the Machine initialization process.
type MachineInitializationStatus struct {

    // BootstrapDataSecretCreated is true when the bootstrap provider reports that the Machine's boostrap secret is created.
    // NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Machine provisioning.
    // The value of this field is never updated after provisioning is completed.
    // Use conditions to monitor the operational state of the Machine's BootstrapSecret.
    // +optional
    BootstrapDataSecretCreated bool `json:"bootstrapDataSecretCreated"`
    
    // InfrastructureProvisioned is true when the infrastructure provider reports that the Machine's infrastructure is fully provisioned.
    // NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Machine provisioning.
    // The value of this field is never updated after provisioning is completed.
    // Use conditions to monitor the operational state of the Machine's infrastructure.
    // +optional
    InfrastructureProvisioned bool `json:"infrastructureProvisioned"`
}
```

| v1beta1 (tentative Dec 2024)  | v1beta2 (tentative Apr 2025)                               | v1beta2 after v1beta1 removal (tentative Apr 2026) |
|-------------------------------|------------------------------------------------------------|----------------------------------------------------|
|                               | `Initialization` (new)                                     | `Initialization`                                   |
| `BootstrapReady`              | `Initialization.BootstrapDataSecretCreated` (renamed)      | `Initialization.BootstrapDataSecretCreated`        |
| `InfrastructureReady`         | `Initialization.InfrastructureProvisioned` (renamed)       | `Initialization.InfrastructureProvisioned`         |
| `V1Beta2` (new)               | (removed)                                                  | (removed)                                          |
| `V1Beta2.Conditions` (new)    | `Conditions` (renamed)                                     | `Conditions`                                       |
|                               | `Deprecated.V1Beta1` (new)                                 | (removed)                                          |
| `FailureReason` (deprecated)  | `Deprecated.V1Beta1.FailureReason` (renamed) (deprecated)  | (removed)                                          |
| `FailureMessage` (deprecated) | `Deprecated.V1Beta1.FailureMessage` (renamed) (deprecated) | (removed)                                          |
| `Conditions` (deprecated)     | `Deprecated.V1Beta1.Conditions` (renamed) (deprecated)     | (removed)                                          |
| other fields...               | other fields...                                            | other fields...                                    |

Notes:
- The `V1Beta2` struct is going to be added to in v1beta1 types in order to provide a preview of changes coming with the v1beta2 types, but without impacting the semantic of existing fields. 
  Fields in the `V1Beta2` will be promoted to status top level fields in the v1beta2 types.
- The `Deprecated` struct is going to exist in v1beta2 types only until v1beta1 removal (9 months or 3 minor releases after v1beta2 is released/v1beta1 is deprecated, whichever is longer).
  Fields in this struct are used for supporting down conversions, thus providing users relying on v1beta1 APIs additional buffer time to pick up the new changes.

##### Machine (New)Conditions

| Condition              | Note                                                                                                                                                                                                                                                          |
|------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Available`            | True if at the machine is Ready for at least MinReadySeconds, as defined by the Machine's MinReadySeconds field                                                                                                                                               |
| `Ready`                | True if the Machines is not deleted, Machine's `BootstrapConfigReady`, `InfrastructureReady`, `NodeHealthy` and `HealthCheckSucceeded` (if present) are true; if other conditions are defined in `spec.readinessGates`, these conditions must be true as well |
| `UpToDate`             | True if the Machine spec matches the spec of the Machine's owner resource, e.g KubeadmControlPlane or MachineDeployment                                                                                                                                       |
| `BootstrapConfigReady` | Mirrors the corresponding `Ready` condition from the Machine's BootstrapConfig resource                                                                                                                                                                       |
| `InfrastructureReady`  | Mirrors the corresponding `Ready` condition from the Machine's Infrastructure resource                                                                                                                                                                        |
| `NodeHealthy`          | True if the Machine's Node is ready and it does not report MemoryPressure, DiskPressure and PIDPressure                                                                                                                                                       |
| `NodeReady`            | True if the Machine's Node is ready                                                                                                                                                                                                                           |
| `HealthCheckSucceeded` | True if MHC instances targeting this machine report the Machine is healthy according to the definition of healthy present in the spec of the MachineHealthCheck object                                                                                        |
| `OwnerRemediated`      | Only present if MHC instances targeting this machine determine that the controller owning this machine should perform remediation                                                                                                                             |
| `Deleting`             | If Machine is deleted, this condition surfaces details about progress in the machine deletion workflow                                                                                                                                                        |
| `Paused`               | True if the Machine or the Cluster it belongs to are paused                                                                                                                                                                                                   |

> To better evaluate proposed changes, below you can find the list of current Machine's conditions:
> Ready, InfrastructureReady, BootstrapReady, NodeHealthy, PreDrainDeleteHookSucceeded, VolumeDetachSucceeded, DrainingSucceeded.
> Additionally:
> - The MachineHealthCheck controller adds the HealthCheckSucceeded and the OwnerRemediated conditions.
> - The KubeadmControlPlane adds the APIServerPodHealthy, ControllerManagerPodHealthy, SchedulerPodHealthy, EtcdPodHealthy, EtcdMemberHealthy conditions.

Notes:
- This proposal introduces a mechanism for extending the meaning of Machine readiness, `ReadinessGates` (see [changes to Machine.Spec](#machine-spec)).
- While `Ready` is the main signal for machines operational state, higher level abstractions in Cluster API like e.g.
  MachineDeployment are relying on the concept of Machine's `Availability`, which can be seen as readiness + stability.
  In order to standardize this concept across different higher level abstractions, this proposal is surfacing `Availability`
  condition at Machine level as well as adding a new `MinReadySeconds` field (see [changes to Machine.Spec](#machine-spec))
  that will be used to compute this condition.
- Similarly, this proposal is standardizing the concept of Machine's `UpToDate` condition, however in this case it will be up to
  the Machine's owner controllers to set this condition.
- Conditions like `NodeReady` and `NodeHealthy` which depend on the connection to the remote cluster will benefit
  from the new `RemoteConnectionProbe` condition at cluster level (see [Cluster (New)Conditions](#cluster-newconditions));
  more specifically those condition should be set to `Unknown` when after `lastRemoteConnectionProbeTime` plus the value
  defined in the `--remote-conditions-grace-period` flag.
- `HealthCheckSucceeded` and `OwnerRemediated` (or `ExternalRemediationRequestAvailable`) conditions are set by the
  MachineHealthCheck controller in case a MachineHealthCheck targets the machine.
- KubeadmControlPlane also adds additional conditions to Machines, but those conditions are not included in the table above
  for sake of simplicity (however they are documented in the KubeadmControlPlane paragraph).

#### Machine Spec

Machine's spec is going to be improved to allow 3rd party components to extend the semantic of the new Machine's `Ready` condition
as well as to standardize the concept of Machine's `Availability`.

Below you can find the relevant fields in MachineSpec v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```go
type MachineSpec struct {

    // MinReadySeconds is the minimum number of seconds for which a Machine should be ready before considering it available.
    // Defaults to 0 (Machine will be considered available as soon as the Machine is ready)
    // +optional
    MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

    // If specified, all conditions listed in ReadinessGates will be evaluated for Machine readiness.
    // A Machine is ready when `BootstrapConfigReady`, `InfrastructureReady`, `NodeHealthy` and `HealthCheckSucceeded` (if present) are "True"; 
    // if other conditions are defined in this field, those conditions should be "True" as well for the Machine to be ready.
    //
    // This field can be used e.g.
    // - By Cluster API control plane providers to extend the semantic of the Ready condition for the Machine they
    //   control, like the kubeadm control provider adding ReadinessGates for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc.
    // - By external controllers, e.g. responsible to install special software/hardware on the Machines
    //   to include the status of those components into ReadinessGates (by surfacing new conditions on Machines and
    //   adding them to ReadinessGates).
    //
    // +optional
    // +listType=map
    // +listMapKey=conditionType
    ReadinessGates []MachineReadinessGate `json:"readinessGates,omitempty"`

    // Other fields...
}

// MachineReadinessGate contains the type of a Machine condition to be used as readiness gates.
type MachineReadinessGate struct {
    // ConditionType refers to a condition in the Machine's condition list with matching type.
    // Note: Both Cluster API conditions or conditions added by 3rd party controllers can be used as readiness gates.
    ConditionType string `json:"conditionType"`
}
```

| v1beta1 (tentative Dec 2024) | v1Beta2 (tentative Apr 2025) | v1beta2 after v1beta1 removal (tentative Apr 2026) |
|------------------------------|------------------------------|----------------------------------------------------|
|                              | `MinReadySeconds` (renamed)  | `MinReadySeconds`                                  |
| `ReadinessGates` (new)       | `ReadinessGates`             | `ReadinessGates`                                   |
| other fields...              | other fields...              | other fields...                                    |

Notes:
- As of today v1beta1 MachineDeployments, MachineSets, MachinePools already have a `spec.MinReadySeconds` field. 
  In v1beta2 those field are going to be migrated to MachineDeployments, MachineSets, MachinePools `spec.template.spec.MinReadySeconds` field, which is
  the `MinReadySeconds` field added in the first line of the table above.
- Similarly to Pod's `ReadinessGates`, also Machine's `ReadinessGates` accept only conditions with positive polarity;
  The Cluster API project might revisit this in the future to stay aligned with Kubernetes or if there are use cases justifying this change.
- Both `MinReadySeconds` and `ReadinessGates` should be treated as other in-place propagated fields (changing them should not trigger rollouts).

#### Machine Print columns

| Current       | To be                         |
|---------------|-------------------------------|
| `NAME`        | `NAME`                        |
| `CLUSTER`     | `CLUSTER`                     |
| `NODE NAME`   | `PAUSED` (new) (*)            |
| `PROVIDER ID` | `NODE NAME`                   |
| `PHASE`       | `PROVIDER ID`                 |
| `AGE`         | `READY` (new)                 |
| `VERSION`     | `AVAILABLE` (new)             |
|               | `UP-TO-DATE` (new)            |
|               | `PHASE`                       |
|               | `AGE`                         |
|               | `VERSION`                     |
|               | `OS-IMAGE` (new) (*)          |
|               | `KERNEL-VERSION` (new) (*)    |
|               | `CONTAINER-RUNTIME` (new) (*) |

(*) visible only when using `kubectl get -o wide`

Notes:
- Print columns are not subject to any deprecation rule, so it is possible to iteratively improve print columns without waiting for the next API version.
- During the implementation we are going to verify the resulting layout and eventually make final adjustments to the column list.
- During the implementation we are going to explore if it is possible to add `INTERNAL-IP` (new) (*), `EXTERNAL-IP` after `VERSION` / before `OS-IMAGE`.
  Might be something like `$.status.addresses[?(@.type == 'InternalIP')].address` works

### Changes to MachineSet resource

#### MachineSet Status

Following changes are implemented to MachineSet's status:

- Update `ReadyReplicas` counter to use the same semantic as Machine's `Ready` condition (today it is computed based on the Node `Ready` condition) and add missing `UpToDateReplicas`.
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions

Below you can find the relevant fields in MachineSetStatus v1beta2, after v1beta1 removal (end state).
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type MachineSetStatus struct {

    // The number of ready replicas for this MachineSet. A machine is considered ready when Machine's Ready condition is true.
    // +optional
    ReadyReplicas *int32 `json:"readyReplicas,omitempty"`
	
    // The number of available replicas for this MachineSet. A machine is considered available when Machine's Available condition is true.
    // +optional
    AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

    // The number of up-to-date replicas for this MachineSet. A machine is considered up-to-date when Machine's UpToDate condition is true.
    // +optional
    UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

    // Represents the observations of a MachineSet's current state.
    // +optional
    // +listType=map
    // +listMapKey=type
    // +kubebuilder:validation:MaxItems=32
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // Other fields...
    // NOTE: `FailureReason`, `FailureMessage` fields won't be there anymore
}
```

| v1beta1 (tentative Dec 2024)      | v1beta2 (tentative Apr 2025)                                  | v1beta2 after v1beta1 removal (tentative Apr 2026) |
|-----------------------------------|---------------------------------------------------------------|----------------------------------------------------|
| `V1Beta2` (new)                   | (removed)                                                     | (removed)                                          |
| `V1Beta2.Conditions` (new)        | `Conditions` (renamed)                                        | `Conditions`                                       |
| `V1Beta2.ReadyReplicas` (new)     | `ReadyReplicas` (renamed)                                     | `ReadyReplicas`                                    |
| `V1Beta2.AvailableReplicas` (new) | `AvailableReplicas` (renamed)                                 | `AvailableReplicas`                                |
| `V1Beta2.UpToDateReplicas` (new)  | `UpToDateReplicas` (renamed)                                  | `UpToDateReplicas`                                 |
|                                   | `Deprecated.V1Beta1` (new)                                    | (removed)                                          |
| `ReadyReplicas` (deprecated)      | `Deprecated.V1Beta1.ReadyReplicas` (renamed) (deprecated)     | (removed)                                          |
| `AvailableReplicas` (deprecated)  | `Deprecated.V1Beta1.AvailableReplicas` (renamed) (deprecated) | (removed)                                          |
| `FailureReason` (deprecated)      | `Deprecated.V1Beta1.FailureReason` (renamed) (deprecated)     | (removed)                                          |
| `FailureMessage` (deprecated)     | `Deprecated.V1Beta1.FailureMessage` (renamed) (deprecated)    | (removed)                                          |
| `Conditions` (deprecated)         | `Deprecated.V1Beta1.Conditions` (renamed) (deprecated)        | (removed)                                          |
| other fields...                   | other fields...                                               | other fields...                                    |

Notes:
- The `V1Beta2` struct is going to be added to in v1beta1 types in order to provide a preview of changes coming with the v1beta2 types, but without impacting the semantic of existing fields.
  Fields in the `V1Beta2` will be promoted to status top level fields in the v1beta2 types.
- The `Deprecated` struct is going to exist in v1beta2 types only until v1beta1 removal (9 months or 3 minor releases after v1beta2 is released/v1beta1 is deprecated, whichever is longer).
  Fields in this struct are used for supporting down conversions, thus providing users relying on v1beta1 APIs additional buffer time to pick up the new changes.
- This proposal is using `UpToDateReplicas` instead of `UpdatedReplicas`; This is a deliberated choice to avoid
  confusion between update (any change) and upgrade (change of the Kubernetes versions).
- Also `AvailableReplicas` will determine Machine's availability via Machine's `Available` condition instead of
  computing availability as of today (based on the Node `Ready` condition)

##### MachineSet (New)Conditions

| Condition          | Note                                                                                                        |
|--------------------|-------------------------------------------------------------------------------------------------------------|
| `MachinesReady`    | This condition surfaces detail of issues on the controlled machines, if any                                 |
| `MachinesUpToDate` | This condition surfaces details of controlled machines not up to date, if any                               |
| `ScalingUp`        | True if actual replicas < desired replicas                                                                  |
| `ScalingDown`      | True if actual replicas > desired replicas                                                                  |
| `Remediating`      | This condition surfaces details about ongoing remediation of the controlled machines, if any                |
| `Deleting`         | If MachineSet is deleted, this condition surfaces details about ongoing deletion of the controlled machines | 
| `Paused`           | True if this MachineSet or the Cluster it belongs to are paused                                             |

> To better evaluate proposed changes, below you can find the list of current MachineSet's conditions:
> Ready, MachinesCreated, Resized, MachinesReady.

Notes:
- Conditions like `ScalingUp`, `ScalingDown`, `Remediating` and `Deleting` are intended to provide visibility on the corresponding lifecycle operation.
  e.g. If the scaling down operation is being blocked by a machine having issues while deleting, this should surface with a reason/message in
  the `ScalingDown` condition.
- MachineSet conditions are intentionally mostly consistent with MachineDeployment conditions to help users troubleshooting.
- MachineSet is considered as a sort of implementation detail of MachineDeployments, so it doesn't have its own concept of availability.
  Similarly, this proposal is dropping the notion of MachineSet readiness because it is preferred to let users focus on Machines readiness.
- When implementing this proposal `MachinesUpToDate` condition will be `false` for older MachineSet, `true` for the current MachineSet; 
  in the future this might change in case Cluster API will start supporting in-place upgrades.
- `Remediating` for older MachineSets will report that remediation will happen as part of the regular rollout (Cluster API
  does not remediate Machines on old MachineSets, because those Machines are already scheduled for deletion).

#### MachineSet Spec

Following changes are implemented to MachineSet's spec:

- Remove `Spec.MinReadySeconds`, which is now part of Machine's spec (and thus exists in MachineSet as `Spec.Template.Spec.MinReadySeconds`).

Below you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

| v1beta1 (tentative Dec 2024) | v1beta2 (tentative Apr 2025)                   | v1beta2 after v1beta1 removal (tentative Apr 2026) |
|------------------------------|------------------------------------------------|----------------------------------------------------|
| `Spec.MinReadySeconds`       | `Spec.Template.Spec.MinReadySeconds` (renamed) | `Spec.Template.Spec.MinReadySeconds`               |
| other fields...              | other fields...                                | other fields...                                    |

#### MachineSet Print columns

| Current       | To be                   |
|---------------|-------------------------|
| `NAME`        | `NAME`                  |
| `CLUSTER`     | `CLUSTER`               |
| `DESIRED` (*) | `PAUSED` (new) (*)      |
| `REPLICAS`    | `DESIRED`               |
| `READY`       | `CURRENT` (renamed) (*) |
| `AVAILABLE`   | `READY` (updated)       |
| `AGE`         | `AVAILABLE` (updated)   |
| `VERSION`     | `UP-TO-DATE` (new)      |
|               | `AGE`                   |
|               | `VERSION`               |

(*) visible only when using `kubectl get -o wide`

Notes:
- Print columns are not subject to any deprecation rule, so it is possible to iteratively improve print columns without waiting for the next API version.
- During the implementation we are going to verify the resulting layout and eventually make final adjustments to the column list.
- During the implementation we should consider if to add columns for bootstrapRef and infraRef resource (same could apply to other resources)
- In k8s Deployment and ReplicaSet have different print columns for replica counters; this proposal enforces replicas
  counter columns consistent across all resources.

### Changes to MachineDeployment resource

#### MachineDeployment Status

Following changes are implemented to MachineDeployment's status:

- Align `UpdatedReplicas` to use Machine's `UpToDate` condition (and rename it accordingly to `UpToDateReplicas`)
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions

Below you can find the relevant fields in MachineDeploymentStatus v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type MachineDeploymentStatus struct {

    // The number of ready replicas for this MachineDeployment. A machine is considered ready when Machine's Ready condition is true.
    // +optional
    ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

    // The number of available replicas for this MachineDeployment. A machine is considered available when Machine's Available condition is true.
    // +optional
    AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

    // The number of up-to-date replicas targeted by this deployment. A machine is considered up-to-date when Machine's UpToDate condition is true.
    // +optional
    UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

    // Represents the observations of a MachineDeployment's current state.
    // +optional
    // +listType=map
    // +listMapKey=type
    // +kubebuilder:validation:MaxItems=32
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // Other fields...
    // NOTE: `FailureReason`, `FailureMessage` fields won't be there anymore
}
```

| v1beta1 (tentative Dec 2024)     | v1beta2 (tentative Apr 2025)                                  | v1beta2 after v1beta1 removal (tentative Apr 2026) |
|----------------------------------|---------------------------------------------------------------|----------------------------------------------------|
| `V1Beta2` (new)                  | (removed)                                                     | (removed)                                          |
| `V1Beta2.Conditions` (new)       | `Conditions` (renamed)                                        | `Conditions`                                       |
| `V1Beta2.ReadyReplicas` (new)    | `ReadyReplicas` (renamed)                                     | `ReadyReplicas`                                    |
| `V1Beta2.AvilableReplicas` (new) | `AvailableReplicas` (renamed)                                 | `AvailableReplicas`                                |
| `V1Beta2.UpToDateReplicas` (new) | `UpToDateReplicas` (renamed)                                  | `UpToDateReplicas`                                 |
|                                  | `Deprecated.V1Beta1` (new)                                    | (removed)                                          |
| `ReadyReplicas` (deprecated)     | `Deprecated.V1Beta1.ReadyReplicas` (renamed) (deprecated)     | (removed)                                          |
| `AvailableReplicas` (deprecated) | `Deprecated.V1Beta1.AvailableReplicas` (renamed) (deprecated) | (removed)                                          |
| `FailureReason` (deprecated)     | `Deprecated.V1Beta1.FailureReason` (renamed) (deprecated)     | (removed)                                          |
| `FailureMessage` (deprecated)    | `Deprecated.V1Beta1.FailureMessage` (renamed) (deprecated)    | (removed)                                          |
| `Conditions` (deprecated)        | `Deprecated.V1Beta1.Conditions` (renamed) (deprecated)        | (removed)                                          |
| `UpdatedReplicas` (deprecated)   | `Deprecated.V1Beta1.UpdatedReplicas` (renamed) (deprecated)   | (removed)                                          |
| other fields...                  | other fields...                                               | other fields...                                    |

Notes:
- The `V1Beta2` struct is going to be added to in v1beta1 types in order to provide a preview of changes coming with the v1beta2 types, but without impacting the semantic of existing fields.
  Fields in the `V1Beta2` will be promoted to status top level fields in the v1beta2 types.
- The `Deprecated` struct is going to exist in v1beta2 types only until v1beta1 removal (9 months or 3 minor releases after v1beta2 is released/v1beta1 is deprecated, whichever is longer).
  Fields in this struct are used for supporting down conversions, thus providing users relying on v1beta1 APIs additional buffer time to pick up the new changes.

##### MachineDeployment (New)Conditions

| Condition          | Note                                                                                                                                                                                                                                                                         |
|--------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Available`        | True if the MachineDeployment is not deleted and it has minimum availability according to parameters specified in the deployment strategy, e.g. If using RollingUpgrade strategy, availableReplicas must be greater or equal than desired replicas - MaxUnavailable replicas |
| `MachinesReady`    | This condition surfaces detail of issues on the controlled machines, if any                                                                                                                                                                                                  |
| `MachinesUpToDate` | This condition surfaces details of controlled machines not up to date, if any                                                                                                                                                                                                |
| `RollingOut`       | True if there is at least one machine not up to date                                                                                                                                                                                                                         |
| `ScalingUp`        | True if actual replicas < desired replicas                                                                                                                                                                                                                                   |
| `ScalingDown`      | True if actual replicas > desired replicas                                                                                                                                                                                                                                   |
| `Remediating`      | This condition surfaces details about ongoing remediation of the controlled machines, if any                                                                                                                                                                                 |
| `Deleting`         | If MachineDeployment is deleted, this condition surfaces details about ongoing deletion of the controlled machines                                                                                                                                                           |
| `Paused`           | True if this MachineDeployment or the Cluster it belongs to are paused                                                                                                                                                                                                       |

> To better evaluate proposed changes, below you can find the list of current MachineDeployment's conditions:
> Ready, Available.

Notes:
- Conditions like `ScalingUp`, `ScalingDown`, `Remediating` and `Deleting` are intended to provide visibility on the corresponding lifecycle operation.
  e.g. If the scaling down operation is being blocked by a machine having issues while deleting, this should surface as a reason/message in
  the `ScalingDown` condition.

#### MachineDeployment Spec

Following changes are implemented to MachineDeployment's spec:

- Remove `Spec.MinReadySeconds`, which is now part of Machine's spec (and thus exists in MachineDeployment as `Spec.Template.Spec.MinReadySeconds`).

Below you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

| v1beta1 (tentative Dec 2024) | v1beta2 (tentative Apr 2025)                   | v1beta2 after v1beta1 removal (tentative Apr 2026) |
|------------------------------|------------------------------------------------|----------------------------------------------------|
| `Spec.MinReadySeconds`       | `Spec.Template.Spec.MinReadySeconds` (renamed) | `Spec.Template.Spec.MinReadySeconds`               |
| other fields...              | other fields...                                | other fields...                                    |

#### MachineDeployment Print columns

| Current                 | To be                  |
|-------------------------|------------------------|
| `NAME`                  | `NAME`                 |
| `CLUSTER`               | `CLUSTER`              |
| `DESIRED` (*)           | `PAUSED` (new) (*)     |
| `REPLICAS`              | `DESIRED`              |
| `READY`                 | `CURRENT` (*)          |
| `UPDATED` (renamed)     | `READY`                |
| `UNAVAILABLE` (deleted) | `AVAILABLE` (new)      |
| `PHASE`                 | `UP-TO-DATE` (renamed) |
| `AGE`                   | `PHASE`                |
| `VERSION`               | `AGE`                  |
|                         | `VERSION`              |

(*) visible only when using `kubectl get -o wide`

Notes:
- Print columns are not subject to any deprecation rule, so it is possible to iteratively improve print columns without waiting for the next API version.
- During the implementation we are going to verify the resulting layout and eventually make final adjustments to the column list.

### Changes to Cluster resource

#### Cluster Status

Following changes are implemented to Cluster's status:

- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions
- Add replica counters to surface status of Machines belonging to this Cluster
- Surface information about ControlPlane connection heartbeat (see new conditions)

Below you can find the relevant fields in ClusterStatus v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type ClusterStatus struct {

    // Initialization provides observations of the Cluster initialization process.
    // NOTE: fields in this struct are part of the Cluster API contract and are used to orchestrate initial Cluster provisioning.
    // The value of those fields is never updated after provisioning is completed.
    // Use conditions to monitor the operational state of the Cluster's BootstrapSecret.
    // +optional
    Initialization *ClusterInitializationStatus `json:"initialization,omitempty"`
    
    // Represents the observations of a Cluster's current state.
    // +optional
    // +listType=map
    // +listMapKey=type
    // +kubebuilder:validation:MaxItems=32
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    
    // ControlPlane groups all the observations about Cluster's ControlPlane current state.
    // +optional
    ControlPlane *ClusterControlPlaneStatus `json:"controlPlane,omitempty"`
    
    // Workers groups all the observations about Cluster's Workers current state.
    // +optional
    Workers *WorkersStatus `json:"workers,omitempty"`
    
    // other fields
}

// ClusterInitializationStatus provides observations of the Cluster initialization process.
type ClusterInitializationStatus struct {

    // InfrastructureProvisioned is true when the infrastructure provider reports that Cluster's infrastructure is fully provisioned.
    // NOTE: this field is part of the Cluster API contract, and it is used to orchestrate provisioning.
    // The value of this field is never updated after provisioning is completed.
    // +optional
    InfrastructureProvisioned bool `json:"infrastructureProvisioned"`
    
    // ControlPlaneInitialized denotes when the control plane is functional enough to accept requests.
    // This information is usually used as a signal for starting all the provisioning operations that depends on
    // a functional API server, but do not require a full HA control plane to exists, like e.g. join worker Machines,
    // install core addons like CNI, CPI, CSI etc.
    // NOTE: this field is part of the Cluster API contract, and it is used to orchestrate provisioning.
    // The value of this field is never updated after provisioning is completed.
    // +optional
    ControlPlaneInitialized bool `json:"controlPlaneInitialized"`
}

// ClusterControlPlaneStatus groups all the observations about control plane current state.
type ClusterControlPlaneStatus struct {
    // Total number of desired control plane machines in this cluster.
    // +optional
    DesiredReplicas *int32 `json:"desiredReplicas,omitempty"`

    // Total number of control plane machines in this cluster.
    // +optional
    Replicas *int32 `json:"replicas,omitempty"`
    
    // The number of up-to-date control plane machines in this cluster. A machine is considered up-to-date when Machine's UpToDate condition is true.
    // +optional
    UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`
    
    // Total number of ready control plane machines in this cluster. A machine is considered ready when Machine's Ready condition is true.
    // +optional
    ReadyReplicas *int32 `json:"readyReplicas,omitempty"`
    
    // Total number of available control plane machines in this cluster. A machine is considered ready when Machine's Available condition is true.
    // +optional
    AvailableReplicas *int32 `json:"availableReplicas,omitempty"`
}

// WorkersStatus groups all the observations about workers current state.
type WorkersStatus struct {
    // Total number of desired worker machines in this cluster.
    // +optional
    DesiredReplicas *int32 `json:"desiredReplicas,omitempty"`

    // Total number of worker machines in this cluster.
    // +optional
    Replicas *int32 `json:"replicas,omitempty"`
    
    // The number of up-to-date worker machines in this cluster. A machine is considered up-to-date when Machine's UpToDate condition is true.
    // +optional
    UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`
    
    // Total number of ready worker machines in this cluster. A machine is considered ready when Machine's Ready condition is true.
    // +optional
    ReadyReplicas *int32 `json:"readyReplicas,omitempty"`
    
    // Total number of available worker machines in this cluster. A machine is considered ready when Machine's Available condition is true.
    // +optional
    AvailableReplicas *int32 `json:"availableReplicas,omitempty"`
}

// NOTE: `FailureReason`, `FailureMessage` fields won't be there anymore
```

| v1beta1 (tentative Dec 2024)                   | v1beta2 (tentative Apr 2025)                               | v1beta2 after v1beta1 removal (tentative Apr 2026) |
|------------------------------------------------|------------------------------------------------------------|----------------------------------------------------|
|                                                | `Initialization` (new)                                     | `Initialization`                                   |
| `InfrastructureReady`                          | `Initialization.InfrastructureProvisioned` (renamed)       | `Initialization.InfrastructureProvisioned`         |
| `ControlPlaneReady`                            | `Initialization.ControlPlaneInitialized` (renamed)         | `Initialization.ControlPlaneInitialized`           |
| `V1Beta2` (new)                                | (removed)                                                  | (removed)                                          |
| `V1Beta2.Conditions` (new)                     | `Conditions` (renamed)                                     | `Conditions`                                       |
| `V1Beta2.ControlPlane` (new)                   | `ControlPlane` (renamed)                                   | `ControlPlane`                                     |
| `V1Beta2.ControlPlane.DesiredReplicas` (new)   | `ControlPlane.DesiredReplicas` (renamed)                   | `ControlPlane.DesiredReplicas`                     |
| `V1Beta2.ControlPlane.Replicas` (new)          | `ControlPlane.Replicas` (renamed)                          | `ControlPlane.Replicas`                            |
| `V1Beta2.ControlPlane.ReadyReplicas` (new)     | `ControlPlane.ReadyReplicas` (renamed)                     | `ControlPlane.ReadyReplicas`                       |
| `V1Beta2.ControlPlane.UpToDateReplicas` (new)  | `ControlPlane.UpToDateReplicas` (renamed)                  | `ControlPlane.UpToDateReplicas`                    |
| `V1Beta2.ControlPlane.AvailableReplicas` (new) | `ControlPlane.AvailableReplicas` (renamed)                 | `ControlPlane.AvailableReplicas`                   |
| `V1Beta2.Workers` (new)                        | `Workers` (renamed)                                        | `Workers`                                          |
| `V1Beta2.Workers.DesiredReplicas` (new)        | `Workers.DesiredReplicas` (renamed)                        | `Workers.DesiredReplicas`                          |
| `V1Beta2.Workers.Replicas` (new)               | `Workers.Replicas` (renamed)                               | `Workers.Replicas`                                 |
| `V1Beta2.Workers.ReadyReplicas` (new)          | `Workers.ReadyReplicas` (renamed)                          | `Workers.ReadyReplicas`                            |
| `V1Beta2.Workers.UpToDateReplicas` (new)       | `Workers.UpToDateReplicas` (renamed)                       | `Workers.UpToDateReplicas`                         |
| `V1Beta2.Workers.AvailableReplicas` (new)      | `Workers.AvailableReplicas` (renamed)                      | `Workers.AvailableReplicas`                        |
|                                                | `Deprecated.V1Beta1` (new)                                 | (removed)                                          |
| `FailureReason` (deprecated)                   | `Deprecated.V1Beta1.FailureReason` (renamed) (deprecated)  | (removed)                                          |
| `FailureMessage` (deprecated)                  | `Deprecated.V1Beta1.FailureMessage` (renamed) (deprecated) | (removed)                                          |
| `Conditions` (deprecated)                      | `Deprecated.V1Beta1.Conditions` (renamed) (deprecated)     | (removed)                                          |
| other fields...                                | other fields...                                            | other fields...                                    |

Notes:
- The `V1Beta2` struct is going to be added to in v1beta1 types in order to provide a preview of changes coming with the v1beta2 types, but without impacting the semantic of existing fields.
  Fields in the `V1Beta2` will be promoted to status top level fields in the v1beta2 types.
- The `Deprecated` struct is going to exist in v1beta2 types only until v1beta1 removal (9 months or 3 minor releases after v1beta2 is released/v1beta1 is deprecated, whichever is longer).
  Fields in this struct are used for supporting down conversions, thus providing users relying on v1beta1 APIs additional buffer time to pick up the new changes.

##### Cluster (New)Conditions

| Condition                     | Note                                                                                                                                                                                                                                                                                       |
|-------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Available`                   | True if Cluster is not deleted, Cluster's `RemoteConnectionProbe`, `InfrastructureReady`, `ControlPlaneAvailable`, `WorkersAvailable`, `TopologyReconciled` (if present) conditions are true. if conditions are defined in `spec.availabilityGates`, those conditions must be true as well |
| `TopologyReconciled`          | True if the topology controller is working properly                                                                                                                                                                                                                                        |
| `InfrastructureReady`         | Mirror of Cluster's infrastructure `Ready` condition                                                                                                                                                                                                                                       |
| `ControlPlaneInitialized`     | True when the Cluster's control plane is functional enough to accept requests. This information is usually used as a signal for starting all the provisioning operations that depends on a functional API server, but do not require a full HA control plane to exists                     |
| `ControlPlaneAvailable`       | Mirror of Cluster's control plane `Available` condition                                                                                                                                                                                                                                    |
| `ControlPlaneMachinesReady`   | This condition surfaces detail of issues on control plane machines, if any                                                                                                                                                                                                                 |
| `ControlPlaneMachineUpToDate` | This condition surfaces details of control plane machines not up to date, if any                                                                                                                                                                                                           |
| `WorkersAvailable`            | Summary of MachineDeployment and MachinePool's `Available` conditions                                                                                                                                                                                                                      |
| `WorkerMachinesReady`         | This condition surfaces detail of issues on the worker machines, if any                                                                                                                                                                                                                    |
| `WorkerMachinesUpToDate`      | This condition surfaces details of worker machines not up to date, if any                                                                                                                                                                                                                  |
| `RemoteConnectionProbe`       | True when control plane can be reached; in case of connection problems, the condition turns to false only if the the cluster cannot be reached for 50s after the first connection problem is detected (or whatever period is defined in the `--remote-connection-grace-period` flag)       |
| `RollingOut`                  | Summary of `RollingOut` conditions from ControlPlane, MachineDeployments and MachinePools                                                                                                                                                                                                  |
| `ScalingUp`                   | Summary of `ScalingUp` conditions from ControlPlane, MachineDeployments, MachinePools and stand-alone MachineSets                                                                                                                                                                          |
| `ScalingDown`                 | Summary of `ScalingDown` conditions from ControlPlane, MachineDeployments, MachinePools and stand-alone MachineSets                                                                                                                                                                        |
| `Remediating`                 | This condition surfaces details about ongoing remediation of the controlled machines, if any                                                                                                                                                                                               |
| `Deleting`                    | If Cluster is deleted, this condition surfaces details about ongoing deletion of the cluster                                                                                                                                                                                               |
| `Paused`                      | True if Cluster and all the resources being part of it are paused                                                                                                                                                                                                                          |

> To better evaluate proposed changes, below you can find the list of current Cluster's conditions:
> Ready, InfrastructureReady, ControlPlaneReady, ControlPlaneInitialized, TopologyReconciled

Notes:
- Conditions like `ScalingUp`, `ScalingDown`, `Remediating` and `Deleting` are intended to provide visibility on the corresponding lifecycle operation.
  e.g. If the scaling down operation is being blocked by a Machine having issues while deleting, this should surface as a reason/message in
  the `ScalingDown` condition.
- `TopologyReconciled` exists only for classy clusters; this condition is managed by the topology reconciler.
- Cluster API is going to maintain a `lastRemoteConnectionProbeTime` and use it in combination with the
  `--remote-connection-grace-period` flag to avoid flakes on `RemoteConnectionProbe`.
- Similarly to `lastHeartbeatTime` in Kubernetes conditions, also `lastRemoteConnectionProbeTime` will not surface on the
  API in order to avoid costly, continuous reconcile events.
- The `ScalingUp` and `ScalingDown` condition on the Cluster are an aggregation of corresponding condition of controlled objects,
  because this helps in better understanding what is going on in the cluster.

#### Cluster Spec

Cluster's spec is going to be improved to allow 3rd parties to extend the semantic of the new Cluster's `Available` condition.

Below you can find the relevant fields in ClusterSpec v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type ClusterSpec struct {
    // AvailabilityGates specifies additional conditions to include when evaluating Cluster availability.
    // A Cluster is available if:
    // * Cluster's `RemoteConnectionProbe` and `TopologyReconciled` conditions are true and
    // * the control plane `Available` condition is true and
    // * all worker resource's `Available` conditions are true and
    // * all conditions defined in AvailabilityGates are true as well.
    // +optional
    // +listType=map
    // +listMapKey=conditionType
    AvailabilityGates []ClusterAvailabilityGate `json:"availabilityGates,omitempty"`

    // Other fields...
}

// ClusterAvailabilityGate contains the type of a Cluster condition to be used as availability gate.
type ClusterAvailabilityGate struct {
    // ConditionType refers to a condition in the Cluster's condition list with matching type.
    // Note: Both Cluster API conditions or conditions added by 3rd party controllers can be used as availability gates.
    ConditionType string `json:"conditionType"`
}
```

| v1beta1 (tentative Dec 2024) | v1Beta2 (tentative Apr 2025) | v1beta2 after v1beta1 removal (tentative Apr 2026) |
|------------------------------|------------------------------|----------------------------------------------------|
| `AvailabilityGates` (new)    | `AvailabilityGates`          | `AvailabilityGates`                                |
| other fields...              | other fields...              | other fields...                                    |

Notes:
- Similarly to Pod's `ReadinessGates`, also Cluster's `AvailabilityGates` accepts only conditions with positive polarity;
  The Cluster API project might revisit this in the future to stay aligned with Kubernetes or if there are use cases justifying this change.
- In future the Cluster API project might consider ways to make `AvailabilityGates` configurable at ClusterClass level, but
  this can be implemented as a follow-up.

#### Cluster Print columns

| Current         | To be                 |
|-----------------|-----------------------|
| `NAME`          | `NAME`                |
| `CLUSTER CLASS` | `CLUSTER CLASS`       |
| `PHASE`         | `PAUSED` (new) (*)    |
| `AGE`           | `AVAILABLE` (new)     |
| `VERSION`       | `CP_DESIRED` (new)    |
|                 | `CP_CURRENT`(new) (*) |
|                 | `CP_READY` (new) (*)  |
|                 | `CP_AVAILABLE` (new)  |
|                 | `CP_UP-TO-DATE` (new) |
|                 | `W_DESIRED` (new)     |
|                 | `W_CURRENT`(new) (*)  |
|                 | `W_READY` (new) (*)   |
|                 | `W_AVAILABLE` (new)   |
|                 | `W_UP-TO-DATE` (new)  |
|                 | `PHASE`               |
|                 | `AGE`                 |
|                 | `VERSION`             |

(*) visible only when using `kubectl get -o wide`

Notes:
- Print columns are not subject to any deprecation rule, so it is possible to iteratively improve print columns without waiting for the next API version.
- During the implementation we are going to verify the resulting layout and eventually make final adjustments to the column list.

### Changes to KubeadmControlPlane (KCP) resource

KubeadmControlPlane (KCP) is considered a reference implementation for control plane providers, so it is included in this
proposal even if it is not a core Cluster API resource.

#### KubeadmControlPlane Status

Following changes are implemented to KubeadmControlPlane's status:

- Update `ReadyReplicas` counter to use the same semantic Machine's `Ready` condition and add missing `UpToDateReplicas`.
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions

Below you can find the relevant fields in KubeadmControlPlaneStatus v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type KubeadmControlPlaneStatus struct {

    // The number of ready replicas for this ControlPlane. A machine is considered ready when Machine's Ready condition is true.
    // Note: In the v1beta1 API version a Machine was counted as ready when the node hosted on the Machine was ready, thus 
    // generating confusion for users looking at the Machine Ready condition.
    // +optional
    ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

    // The number of available replicas targeted by this ControlPlane. A machine is considered ready when Machine's Available condition is true.
    // +optional
    AvailableReplicas *int32 `json:"availableReplicas,omitempty"`
	
    // The number of up-to-date replicas targeted by this ControlPlane. A machine is considered ready when Machine's UpToDate condition is true.
    // +optional
    UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

    // Represents the observations of a ControlPlane's current state. 
    // +optional
    // +listType=map
    // +listMapKey=type
    // +kubebuilder:validation:MaxItems=32
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // Other fields...
    // NOTE: `Ready`, `FailureReason`, `FailureMessage` fields won't be there anymore
}
```

| v1beta1 (tentative Dec 2024)      | v1beta2 (tentative Apr 2025)                                | v1beta2 after v1beta1 removal (tentative Apr 2026) |
|-----------------------------------|-------------------------------------------------------------|----------------------------------------------------|
| `Ready` (deprecated)              | `Ready` (deprecated)                                        | (removed)                                          |
| `V1Beta2` (new)                   | (removed)                                                   | (removed)                                          |
| `V1Beta2.Conditions` (new)        | `Conditions` (renamed)                                      | `Conditions`                                       |
| `V1Beta2.ReadyReplicas` (new)     | `ReadyReplicas` (renamed)                                   | `ReadyReplicas`                                    |
| `V1Beta2.AvailableReplicas` (new) | `AvailableReplicas` (renamed)                               | `AvailableReplicas`                                |
| `V1Beta2.UpToDateReplicas` (new)  | `UpToDateReplicas` (renamed)                                | `UpToDateReplicas`                                 |
|                                   | `Deprecated.V1Beta1` (new)                                  | (removed)                                          |
| `ReadyReplicas` (deprecated)      | `Deprecated.V1Beta1.ReadyReplicas` (renamed) (deprecated)   | (removed)                                          |
| `FailureReason` (deprecated)      | `Deprecated.V1Beta1.FailureReason` (renamed) (deprecated)   | (removed)                                          |
| `FailureMessage` (deprecated)     | `Deprecated.V1Beta1.FailureMessage` (renamed) (deprecated)  | (removed)                                          |
| `Conditions` (deprecated)         | `Deprecated.V1Beta1.Conditions` (renamed) (deprecated)      | (removed)                                          |
| `UpdatedReplicas` (deprecated)    | `Deprecated.V1Beta1.UpdatedReplicas` (renamed) (deprecated) | (removed)                                          |
| other fields...                   | other fields...                                             | other fields...                                    |

Notes:
- The `V1Beta2` struct is going to be added to in v1beta1 types in order to provide a preview of changes coming with the v1beta2 types, but without impacting the semantic of existing fields.
  Fields in the `V1Beta2` will be promoted to status top level fields in the v1beta2 types.
- The `Deprecated` struct is going to exist in v1beta2 types only until v1beta1 removal (9 months or 3 minor releases after v1beta2 is released/v1beta1 is deprecated, whichever is longer).
  Fields in this struct are used for supporting down conversions, thus providing users relying on v1beta1 APIs additional buffer time to pick up the new changes.

##### KubeadmControlPlane (New)Conditions

| Condition                       | Note                                                                                                                                                                                                                                                                                                                            |
|---------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Available`                     | True if not delete, `CertificatesAvailable` is true, at least one Kubernetes API server, scheduler and controller manager control plane are healthy, and etcd has enough operational members to meet quorum requirements                                                                                                        |
| `Initialized`                   | True when the control plane is functional enough to accept requests. This information is usually used as a signal for starting all the provisioning operations that depend on a functional API server, but do not require a full HA control plane to exist.                                                                     |
| `CertificatesAvailable`         | True if all the cluster certificates exist.                                                                                                                                                                                                                                                                                     |
| `EtcdClusterHealthy`            | This condition surfaces issues to the etcd cluster hosted on machines managed by this object, if any. It is computed as aggregation of Machine's `EtcdMemberHealthy` conditions plus additional checks validating potential issues to etcd quorum                                                                               |
| `ControlPlaneComponentsHealthy` | This condition surfaces issues to Kubernetes control plane components hosted on machines managed by this object. It is computed as aggregation of Machine's `APIServerPodHealthy`, `ControllerManagerPodHealthy`, `SchedulerPodHealthy`, `EtcdPodHealthy` conditions plus additional checks on control plane machines and nodes |
| `MachinesReady`                 | This condition surfaces detail of issues on the controlled machines, if any. Please note this will include also `APIServerPodHealthy`, `ControllerManagerPodHealthy`, `SchedulerPodHealthy`, and if not using an external etcd also `EtcdPodHealthy`, `EtcdMemberHealthy`                                                       |
| `MachinesUpToDate`              | This condition surfaces details of controlled machines not up to date, if any                                                                                                                                                                                                                                                   |
| `RollingOut`                    | True if there is at least one machine not up to date                                                                                                                                                                                                                                                                            |
| `ScalingUp`                     | True if actual replicas < desired replicas                                                                                                                                                                                                                                                                                      |
| `ScalingDown`                   | True if actual replicas > desired replicas                                                                                                                                                                                                                                                                                      |
| `Remediating`                   | This condition surfaces details about ongoing remediation of the controlled machines, if any                                                                                                                                                                                                                                    |
| `Deleting`                      | If KubeadmControlPlane is deleted, this condition surfaces details about ongoing deletion of the controlled machines                                                                                                                                                                                                            |
| `Paused`                        | True if this resource or the Cluster it belongs to are paused                                                                                                                                                                                                                                                                   |

> To better evaluate proposed changes, below you can find the list of current KubeadmControlPlane's conditions:
> Ready, CertificatesAvailable, MachinesCreated, Available, MachinesSpecUpToDate, Resized, MachinesReady,
> ControlPlaneComponentsHealthy, EtcdClusterHealthy.

Notes:
- Conditions like `ScalingUp`, `ScalingDown`, `Remediating` and `Deleting` are intended to provide visibility on the corresponding lifecycle operation.
  e.g. If the scaling down operation is being blocked by a Machine having issues while deleting, this should surface as a reason/message in
  the `ScalingDown` condition.
- The KubeadmControlPlane controller is going to add `APIServerPodHealthy`, `ControllerManagerPodHealthy`, `SchedulerPodHealthy`,
  `EtcdPodHealthy`, `EtcdMemberHealthy`conditions to the controller machines. These conditions will also be defined as `readinessGates`
  for computing Machine's `Ready` condition.
- The KubeadmControlPlane controller is going to stop setting the `EtcdClusterHealthy` condition to true in case of external etcd.
  This will allow tools managing the external etcd instance to use the `EtcdClusterHealthy` condition to report back status into
  the KubeadmControlPlane if they want to.

#### KubeadmControlPlane Print columns

| Current                 | To be                  |
|-------------------------|------------------------|
| `NAME`                  | `NAME`                 |
| `CLUSTER`               | `CLUSTER`              |
| `DESIRED` (*)           | `PAUSED` (new) (*)     |
| `REPLICAS`              | `INITIALIZED` (new)    |
| `READY`                 | `DESIRED`              |
| `UPDATED` (renamed)     | `CURRENT` (*)          |
| `UNAVAILABLE` (deleted) | `READY`                |
| `AGE`                   | `AVAILABLE` (new)      |
| `VERSION`               | `UP-TO-DATE` (renamed) |
|                         | `AGE`                  |
|                         | `VERSION`              |

(*) visible only when using `kubectl get -o wide`

Notes:
- Print columns are not subject to any deprecation rule, so it is possible to iteratively improve print columns without waiting for the next API version.
- During the implementation we are going to verify the resulting layout and eventually make final adjustments to the column list.

### Changes to MachinePool resource

#### MachinePool Status

Following changes are implemented to MachinePool's status:

- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
- Update `ReadyReplicas` counter to use the same semantic Machine's `Ready` condition and add missing `UpToDateReplicas`.
- Align MachinePools replica counters to other CAPI resources
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions

Below you can find the relevant fields in MachinePoolStatus v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type MachinePoolStatus struct {

    // The number of ready replicas for this MachinePool. A machine is considered ready when Machine's Ready condition is true.
    // +optional
    ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

    // The number of available replicas for this MachinePool. A machine is considered available when Machine's Available condition is true.
    // +optional
    AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

    // The number of up-to-date replicas targeted by this MachinePool. A machine is considered available when Machine's  UpToDate condition is true.
    // +optional
    UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

    // Initialization provides observations of the MachinePool initialization process.
    // NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial MachinePool provisioning.
    // The value of those fields is never updated after provisioning is completed.
    // Use conditions to monitor the operational state of the MachinePool.
    // +optional
    Initialization *MachinePoolInitializationStatus `json:"initialization,omitempty"`
    
    // Conditions represent the observations of a MachinePool's current state.
    // +optional
    // +listType=map
    // +listMapKey=type
    // +kubebuilder:validation:MaxItems=32
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    
    // Other fields...
    // NOTE:`FailureReason`, `FailureMessage`, `BootstrapReady`, `InfrastructureReady` fields won't be there anymore
}

// MachinePoolInitializationStatus provides observations of the MachinePool initialization process.
type MachinePoolInitializationStatus struct {

    // BootstrapDataSecretCreated is true when the bootstrap provider reports that the MachinePool's boostrap data secret is created.
    // NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial MachinePool provisioning.
    // The value of this field is never updated after provisioning is completed.
    // Use conditions to monitor the operational state of the MachinePool's BootstrapSecret.
    // +optional
    BootstrapDataSecretCreated bool `json:"bootstrapDataSecretCreated"`
    
    // InfrastructureProvisioned is true when the infrastructure provider reports that the MachinePool's infrastructure is fully provisioned.
    // NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial MachinePool provisioning.
    // The value of this field is never updated after provisioning is completed.
    // Use conditions to monitor the operational state of the MachinePool's infrastructure.
    // +optional
    InfrastructureProvisioned bool `json:"infrastructureProvisioned"`
}
```

| v1beta1 (tentative Dec 2024)       | v1beta2 (tentative Apr 2025)                                  | v1beta2 after v1beta1 removal (tentative Apr 2026) |
|------------------------------------|---------------------------------------------------------------|----------------------------------------------------|
|                                    | `Initialization` (new)                                        | `Initialization`                                   |
| `BootstrapReady`                   | `Initialization.BootstrapDataSecretCreated` (renamed)         | `Initialization.BootstrapDataSecretCreated`        |
| `InfrastructureReady`              | `Initialization.InfrastructureProvisioned` (renamed)          | `Initialization.InfrastructureProvisioned`         |
| `V1Beta2` (new)                    | (removed)                                                     | (removed)                                          |
| `V1Beta2.Conditions` (new)         | `Conditions` (renamed)                                        | `Conditions`                                       |
| `V1Beta2.UpToDateReplicas` (new)   | `UpToDateReplicas` (renamed)                                  | `UpToDateReplicas`                                 |
| `V1Beta2.ReadyReplicas` (new)      | `ReadyReplicas` (renamed)                                     | `ReadyReplicas`                                    |
| `V1Beta2.AvailableReplicas` (new)  | `AvailableReplicas` (renamed)                                 | `AvailableReplicas`                                |
|                                    | `Deprecated.V1Beta1` (new)                                    | (removed)                                          |
| `ReadyReplicas` (deprecated)       | `Deprecated.V1Beta1.ReadyReplicas` (renamed) (deprecated)     | (removed)                                          |
| `AvailableReplicas` (deprecated)   | `Deprecated.V1Beta1.AvailableReplicas` (renamed) (deprecated) | (removed)                                          |
| `FailureReason` (deprecated)       | `Deprecated.V1Beta1.FailureReason` (renamed) (deprecated)     | (removed)                                          |
| `FailureMessage` (deprecated)      | `Deprecated.V1Beta1.FailureMessage` (renamed) (deprecated)    | (removed)                                          |
| `Conditions` (deprecated)          | `Deprecated.V1Beta1.Conditions` (renamed) (deprecated)        | (removed)                                          |
| other fields...                    | other fields...                                               | other fields...                                    |

Notes:
- The `V1Beta2` struct is going to be added to in v1beta1 types in order to provide a preview of changes coming with the v1beta2 types, but without impacting the semantic of existing fields.
  Fields in the `V1Beta2` will be promoted to status top level fields in the v1beta2 types.
- The `Deprecated` struct is going to exist in v1beta2 types only until v1beta1 removal (9 months or 3 minor releases after v1beta2 is released/v1beta1 is deprecated, whichever is longer).
  Fields in this struct are used for supporting down conversions, thus providing users relying on v1beta1 APIs additional buffer time to pick up the new changes.

##### MachinePool (New)Conditions

| Condition              | Note                                                                                                         |
|------------------------|--------------------------------------------------------------------------------------------------------------|
| `Available`            | True when `InfrastructureReady` and available replicas >= desired replicas (see notes below)                 |
| `BootstrapConfigReady` | Mirrors the corresponding condition from the MachinePool's BootstrapConfig resource                          |
| `InfrastructureReady`  | Mirrors the corresponding condition from the MachinePool's Infrastructure resource                           |
| `MachinesReady`        | This condition surfaces detail of issues on the controlled machines, if any                                  |
| `MachinesUpToDate`     | This condition surfaces details of controlled machines not up to date, if any                                |
| `RollingOut`           | True if there is at least one machine not up to date                                                         |
| `ScalingUp`            | True if actual replicas < desired replicas                                                                   |
| `ScalingDown`          | True if actual replicas > desired replicas                                                                   |
| `Remediating`          | This condition surfaces details about ongoing remediation of the controlled machines, if any                 |
| `Deleting`             | If MachinePool is deleted, this condition surfaces details about ongoing deletion of the controlled machines |
| `Paused`               | True if this MachinePool or the Cluster it belongs to are paused                                             |

> To better evaluate proposed changes, below you can find the list of current MachinePool's conditions:
> Ready, BootstrapReady, InfrastructureReady, ReplicasReady.

Notes:
- Conditions like `ScalingUp`, `ScalingDown`, `Remediating` and `Deleting` are intended to provide visibility on the corresponding lifecycle operation.
  e.g. If the scaling down operation is being blocked by a Machine having issues while deleting, this should surface with a reason/message in
  the `ScalingDown` condition.
- As of today MachinePool does not have a notion similar to MachineDeployment's MaxUnavailability.

#### MachinePool Spec

Following changes are implemented to MachinePool's spec:

- Remove `Spec.MinReadySeconds`, which is now part of Machine's spec (and thus exists in MachinePool as `Spec.Template.Spec.MinReadySeconds`).

Below you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

| v1beta1 (tentative Dec 2024) | v1beta2 (tentative Apr 2025)                   | v1beta2 after v1beta1 removal (tentative Apr 2026) |
|------------------------------|------------------------------------------------|----------------------------------------------------|
| `Spec.MinReadySeconds`       | `Spec.Template.Spec.MinReadySeconds` (renamed) | `Spec.Template.Spec.MinReadySeconds`               |
| other fields...              | other fields...                                | other fields...                                    |

#### MachinePool Print columns

| Current       | To be                  |
|---------------|------------------------|
| `NAME`        | `NAME`                 |
| `CLUSTER`     | `CLUSTER`              |
| `DESIRED` (*) | `PAUSED` (new) (*)     |
| `REPLICAS`    | `DESIRED`              |
| `PHASE`       | `CURRENT` (*)          |
| `AGE`         | `READY`                |
| `VERSION`     | `AVAILABLE` (new)      |
|               | `UP-TO-DATE` (renamed) |
|               | `PHASE`                |
|               | `AGE`                  |
|               | `VERSION`              |

(*) visible only when using `kubectl get -o wide`

Notes:
- Print columns are not subject to any deprecation rule, so it is possible to iteratively improve print columns without waiting for the next API version.
- During the implementation we are going to verify the resulting layout and eventually make final adjustments to the column list.

### Changes to Cluster API contract

The Cluster API contract defines a set of rules a provider is expected to comply with in order to interact with Cluster API.

When the v1beta2 API will be released (tentative Apr 2025), also the Cluster API contract will be bumped to v1beta2.

As written at the beginning of this document, this proposal is not going to change the fundamental way the Cluster API contract
with infrastructure, bootstrap and control providers currently works (by using status fields; however, we are renaming a few fields
as detailed below).

Similarly, this proposal is not going to change the fact that the Cluster API contract does not require providers to implement
conditions, even if this is recommended because conditions greatly improve user's experience.

However, this proposal is introducing a few changes into the v1beta2 version of the Cluster API contract in order to:
- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
- Remove `failureReason` and `failureMessage`.

What is worth to notice is that for the first time in the history of the project, this proposal is introducing
a mechanism that allows providers to adapt to a new contract incrementally, more specifically:

- Providers won't be required to synchronize their changes to adapt to the Cluster API v1beta2 contract with the
  Cluster API's v1beta2 release.

- Each provider can implement changes described in the following paragraphs at its own pace, but the transition
  _must be completed_ before v1beta1 removal (tentative Apr 2026).

- Starting from the CAPI release when v1beta1 removal will happen (tentative Apr 2026), providers which are implementing
  the v1beta1 contract will stop to work (they will work only with older versions of Cluster API).

Additionally:

- Providers implementing conditions won't be required to do the transition from custom Cluster API Condition type
  to Kubernetes `metav1.Conditions` type (but this transition is recommended because it improves the consistency of each provider
  with Kubernetes, Cluster API and the ecosystem).

- However, providers choosing to keep using Cluster API custom conditions should be aware that starting from the
  CAPI release when v1beta1 removal will happen (tentative Apr 2026), the Cluster API project will remove the
  Cluster API condition type, the `util/conditions` package, the code handling conditions in `util/patch.Helper` and
  everything related to the custom Cluster API `v1beta.Condition` type.
  (in other words, Cluster API custom condition must be replaced by provider's own custom conditions).

#### Contract for infrastructure providers

Note: given that the contract only defines expected names for fields in a resources at YAML/JSON level, we are
using these in this paragraph (instead of golang field names).

##### InfrastructureCluster

Following changes are planned for the contract for the InfrastructureCluster resource:

- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
  - Rename `status.ready` into `status.initialization.provisioned`.
- Remove `failureReason` and `failureMessage`.

| v1beta1 (tentative Dec 2024)                                          | v1beta2 (tentative Apr 2025)                                                                                     | v1beta2 after v1beta1 removal (tentative Apr 2026)                                         |
|-----------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------|
| `status.ready`, required                                              | `status.ready` (deprecated), one of `status.ready` or `status.initialization.provisioned` required               | (removed)                                                                                  |
|                                                                       | `status.initialization.provisioned` (new), one of `status.ready` or `status.initialization.provisioned` required | `status.initialization.provisioned`                                                        |
| `status.conditions[Ready]`, optional with fall back on `status.ready` | `status.conditions[Ready]`, optional with fall back on `status.ready` or `status.initialization.provisioned`     | `status.conditions[Ready]`, optional with fall back on `status.initialization.provisioned` |
| `status.failureReason`, optional                                      | `status.failureReason` (deprecated), optional                                                                    | (removed)                                                                                  |
| `status.failureMessage`, optional                                     | `status.failureMessage` (deprecated), optional                                                                   | (removed)                                                                                  |
| other fields/rules...                                                 | other fields/rules...                                                                                            |                                                                                            |

Notes:
- InfrastructureCluster's `status.initialization.provisioned` will surface into Cluster's `status.initialization.infrastructureProvisioned` field.
- InfrastructureCluster's `status.initialization.provisioned` must signal the completion of the initial provisioning of the cluster infrastructure.
  The value of this field should never be updated after provisioning is completed, and Cluster API will ignore any changes to it.
- InfrastructureCluster's `status.conditions[Ready]` will surface into Cluster's `status.conditions[InfrastructureReady]` condition.
- InfrastructureCluster's `status.conditions[Ready]` must surface issues during the entire lifecycle of the InfrastructureCluster
  (both during initial InfrastructureCluster provisioning and after the initial provisioning is completed).

##### InfrastructureMachine

Following changes are planned for the contract for the InfrastructureMachine resource:

- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
  - Rename `status.ready` into `status.initialization.provisioned`.
- Remove `failureReason` and `failureMessage`.

| v1beta1 (tentative Dec 2024)                                          | v1beta2 (tentative Apr 2025)                                                                                     | v1beta2 after v1beta1 removal (tentative Apr 2026)                                         |
|-----------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------|
| `status.ready`, required                                              | `status.ready` (deprecated), one of `status.ready` or `status.initialization.provisioned` required               | (removed)                                                                                  |
|                                                                       | `status.initialization.provisioned` (new), one of `status.ready` or `status.initialization.provisioned` required | `status.initialization.provisioned`                                                        |
| `status.conditions[Ready]`, optional with fall back on `status.ready` | `status.conditions[Ready]`, optional with fall back on `status.ready` or `status.initialization.provisioned`     | `status.conditions[Ready]`, optional with fall back on `status.initialization.provisioned` |
| `status.failureReason`, optional                                      | `status.failureReason` (deprecated), optional                                                                    | (removed)                                                                                  |
| `status.failureMessage`, optional                                     | `status.failureMessage` (deprecated), optional                                                                   | (removed)                                                                                  |
| other fields/rules...                                                 | other fields/rules...                                                                                            |                                                                                            |

Notes:
- InfrastructureMachine's `status.initialization.provisioned` will surface into Machine's `status.initialization.infrastructureProvisioned` field.
- InfrastructureMachine's `status.initialization.provisioned` must signal the completion of the initial provisioning of the machine infrastructure.
  The value of this field should never be updated after provisioning is completed, and Cluster API will ignore any changes to it.
- InfrastructureMachine's `status.conditions[Ready]` will surface into Machine's `status.conditions[InfrastructureReady]` condition.
- InfrastructureMachine's `status.conditions[Ready]` must surface issues during the entire lifecycle of the Machine
  (both during initial InfrastructureMachine provisioning and after the initial provisioning is completed).

#### Contract for bootstrap providers

Following changes are planned for the contract for the BootstrapConfig resource:

- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
  - Rename `status.ready` into `status.initialization.dataSecretCreated`.
- Remove `failureReason` and `failureMessage`.

| v1beta1 (tentative Dec 2024)                                          | v1beta2 (tentative Apr 2025)                                                                                                  | v1beta2 after v1beta1 removal (tentative Apr 2026)                                                   |
|-----------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------|
| `status.ready`, required                                              | `status.ready` (deprecated), one of `status.ready` or `status.initialization.dataSecretCreated`, required                     | (removed)                                                                                            |
|                                                                       | `status.initialization.dataSecretCreated` (new), one of `status.ready` or `status.initialization.dataSecretCreated`, required | `status.initialization.dataSecretCreated`, required                                                  |
| `status.conditions[Ready]`, optional with fall back on `status.ready` | `status.conditions[Ready]`, optional with fall back on `status.ready` or `status.initialization.dataSecretCreated` set        | `status.conditions[Ready]`, optional with fall back on `status.initialization.dataSecretCreated` set |
| `status.failureReason`, optional                                      | `status.failureReason` (deprecated), optional                                                                                 | (removed)                                                                                            |
| `status.failureMessage`, optional                                     | `status.failureMessage` (deprecated), optional                                                                                | (removed)                                                                                            |
| other fields/rules...                                                 | other fields/rules...                                                                                                         |                                                                                                      |

Notes:
- BootstrapConfig's `status.initialization.dataSecretCreated` will surface into Machine's `status.initialization.bootstrapDataSecretCreated` field.
- BootstrapConfig's `status.initialization.dataSecretCreated` must signal the completion of the initial provisioning of the bootstrap data secret.
  The value of this field should never be updated after provisioning is completed, and Cluster API will ignore any changes to it.
- BootstrapConfig's `status.conditions[Ready]` will surface into Machine's `status.conditions[BootstrapConfigReady]` condition.
- BootstrapConfig's `status.conditions[Ready]` must surface issues during the entire lifecycle of the BootstrapConfig
  (both during initial BootstrapConfig provisioning and after the initial provisioning is completed).

#### Contract for control plane providers

Following changes are planned for the contract for the ControlPlane resource:

- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
  - Remove `status.ready` (`status.ready` is a redundant signal of the control plane being initialized).
  - Rename `status.initialized` into `status.initialization.controlPlaneInitialized`.
- Remove `failureReason` and `failureMessage`.
- Align replica counters with CAPI core objects

| v1beta1 (tentative Dec 2024)                                          | v1beta2 (tentative Apr 2025)                                                                                                                                          | v1beta2 after v1beta1 removal (tentative Apr 2026)                                                          |
|-----------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------|
| `status.ready`, required                                              | `status.ready` (deprecated), one of `status.ready` or `status.initialization.controlPlaneInitialized` required                                                        | (removed)                                                                                                   |
| `status.initialized`, required                                        | `status.initialization.controlPlaneInitialized` (renamed), one of `status.ready` or `status.initialization.controlPlaneInitialized` required                          | `status.initialization.controlPlaneInitialized`, required                                                   |
| `status.conditions[Ready]`, optional with fall back on `status.ready` | `status.deprecated.v1beta1.conditions[Ready]` (renamed, deprecated), optional with fall back on `status.ready` or `status.initialization.controlPlaneInitialized` set | (removed)                                                                                                   |
|                                                                       | `status.conditions[Available]` (new), optional with fall back optional with fall back on `status.ready` or `status.initialization.controlPlaneInitialized` set        | `status.conditions[Available]`, optional with fall back on `status.initializiation.controlPlaneInitialized` |
| `status.failureReason`, optional                                      | `status.failureReason` (deprecated), optional                                                                                                                         | (removed)                                                                                                   |
| `status.failureMessage`, optional                                     | `status.failureMessage` (deprecated), optional                                                                                                                        | (removed)                                                                                                   |
|                                                                       | `status.availableReplicas` (new), required (1) with fallback on `status.readyReplicas` (CP did not have a concept of availability before)                             | `status.availableReplicas`, required (1)                                                                    |
| `status.updatedReplicas`, required (1)                                | `status.upToDateReplicas` (renamed), required (1) will fall back on `status.updatedReplicas`                                                                          | `status.upToDateReplicas`, required (1)                                                                     |
| other fields/rules...                                                 | other fields/rules...                                                                                                                                                 |                                                                                                             |

required (1): required only if using replicas.

Additionally:
- Control plane providers will be expected to continuously set Machine's `status.conditions[UpToDate]` condition
  and `spec.minReadySeconds`; please note that a CP provider implementation can decide to enforce `spec.minReadySeconds` to be 0 and 
  introduce a difference between readiness and availability at a later stage (e.g. KCP will do this). 
  Those fields should be treated like other fields propagated /updated in place, without triggering
  machine rollouts (`nodeDrainTimeout`, `nodeVolumeDetachTimeout`, `nodeDeletionTimeout`, labels and annotations).
- Cluster controller is going to aggregate `ScalingUp` and `ScalingDown` conditions from Control plane providers, if existing.

Notes:
- ControlPlane's `status.initialization.controlPlaneInitialized` will surface into Cluster's `staus.initialization.controlPlaneInitialized` field; also,
  the fact that the control plane is available to receive requests will be recorded in Cluster's `status.conditions[ControlPlaneInitialized]` condition.
  The value of this field should never be updated after provisioning is completed, and Cluster API will ignore any changes to it.
- The new ControlPlane's `status.conditions[Available]` condition must surface control plane availability, e.g. the ability to
  accept and process API server call, having etcd quorum etc.
- It is up to each control plane provider to determine what could impact the overall availability in their own
  specific control plane implementation.
- As a general guideline, control plane providers implementing solutions with redundant instances of Kubernetes control plane components,
  should not consider the temporary unavailability of one of those instances as relevant for the overall control plane availability.
  e.g. one kube-apiserver over three down, should not impact the overall control plane availability.

### Example use cases

This paragraph is a collection of use cases for an improved status in Cluster API resources and notes about how this
proposal address those use cases.

As a cluster admin with MachineDeployment ownership I'd like to understand if my MD is performing a rolling upgrade and why by looking at the MD status/conditions

> The main signal for MD is performing a rolling upgrade will be `MD.Status.Conditions[MachinesUpToDate]`.

> At least in the first iteration there won't be a signal at MD level about why rollout is happening, because controlled machines might
> have different reasons why they are not UpToDate (and the admin can check those conditions by looking at single machines).
> In future iterations of this proposal we might find ways to aggregate those reasons into the message for the `MD.Status.Conditions[MachinesUpToDate]` condition.

As a cluster admin with MachineDeployment ownership I'd like to understand why my MD rollout is blocked by looking at the MD status/conditions

> `MD.Status.Conditions[ScalingUp]` and `MD.Status.Conditions[ScalingDown]` will give information about how the rollout is being performed,
> if there are issues creating or deleting the machines, etc.

As a cluster admin with MachineDeployment ownership I'd like to understand why Machines are failing to be available by looking at the MD status/conditions

> `MD.Status.Conditions[MachinesReady]` condition will aggregate errors from all the Machines controlled by a MD.

As a cluster admin with MachineDeployment ownership I'd like to understand why Machines are stuck on deletion looking at the MD status/conditions

> `MD.Status.Conditions[ScalingDown]` will give information if there are issues deleting machines.

### Security Model

This proposal does not impact Cluster API security model.

### Risks and Mitigations

_Like any API change, this proposal will have impact on Cluster API users_

Mitigations:

This proposal abides to Kubernetes deprecation rules, and it also ensures isomorphic conversions to/from v1beta1 APIs
can be supported (until v1beta1 removal, tentative Apr 2026).

On top of that, a few design decisions have been made with the specific intent to further minimize impact on
users and providers e.g.
- The decision to keep `Deprecated` fields in v1beta2 API (until v1beta1 removal, tentative Apr 2026).
- The decision to allow providers to adopt the Cluster API v1beta2 contract at their own pace (transition _must be completed_
  before v1beta1 removal, tentative Apr 2026).

All in all, those decisions are consistent with the fact that in Cluster API we are already treating our APIs
(and the Cluster API contract) as fully graduated APIs no matter if they are still beta.

_This proposal requires a considerable amount of work, and it can be risky to implement this in a single release cycle_

This proposal intentionally highlights changes that can be implemented before the actual work for the v1beta2 API version starts.

Those changes will not only allow users to take benefit from this work ASAP, but also provides a way to split the work
across more than one release cycle (tentatively two release cycles).

## Alternatives

_Keep Cluster API custom condition types, eventually improve them incrementally_

This idea was considered, but ultimately discarded because the end state we are aiming for is to align to Kubernetes.
Therefore, the sooner, the better, and the opportunity materialized when discussing the scope for v1beta2 API version.

_Implement down conversion instead of maintaining `Deprecated` fields_

This idea was considered, but discarded because the constraint of ensuring down conversion for every new field/condition
would have prevented this proposal from designing the ideal target state we are aiming to.

Additionally, the idea of dropping all the existing status fields/conditions in the new v1beta2 API (by supporting down conversion),
was considered negatively because it implies a sudden, big change both for users and providers.

Instead, we would like to minimize impact on users and providers by preserving old fields in `Deprecated` until v1beta1 removal,
which is ultimately the same process suggested for removal of API fields from graduated APIs.

Note: There will still be some impacts because `Deprecated` fields will be in a different location from where the
original fields was, but this should be easier to handle than being forced to immediately adapt the new status fields/conditions.

## Upgrade Strategy

Transition from v1beta1 API/contract to v1beta2 contract is detailed in previous paragraphs. Notably:
- Isomorphic conversions to/from v1beta1 APIs are supported until v1beta1 removal, as required by Kubernetes deprecation rules.
- Providers will be allowed to adopt the Cluster API v1beta2 contract at their own pace (transition _must be completed_
  before v1beta1 removal).

## Implementation History

- [x] 2024-07-17: Open proposal PR, still WIP
- [x] 2024-07-17: Present proposal at a [community meeting](https://www.youtube.com/watch?v=frCg522ZfRQ)
  - [10000 feet overview](https://docs.google.com/presentation/d/1hhgCufOIuqHz6YR_RUPGo0uTjfm5YafjCb6JHY1_clY/edit?usp=sharing)
- [x] 2024-09-16: Proposal approved
