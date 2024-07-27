
---
title: Proposal Template
authors:
- "@fabriziopandini"
reviewers:
- "add"
creation-date: 2024-07-17
last-updated: 2024-07-17
status: provisional
see-also:
- ...
---

# Improving status in CAPI resources

## Table of Contents

- [Improving status in CAPI resources](#improving-status-in-capi-resources)
  - [Table of Contents](#table-of-contents)
- [Summary](#summary)
- [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [Readiness and Availability](#readiness-and-availability)
    - [Transition to K8s API conventions aligned conditions](#transition-to-k8s-api-conventions-aligned-conditions)
    - [Changes to Machine resource](#changes-to-machine-resource)
      - [Machine Status](#machine-status)
        - [Machine (New)Conditions](#machine-newconditions)
      - [Machine Spec](#machine-spec)
      - [Machine Print columns](#machine-print-columns)
    - [Changes to MachineSet resource](#changes-to-machineset-resource)
      - [MachineSet Status](#machineset-status)
      - [MachineSet (New)Conditions](#machineset-newconditions)
      - [MachineSet Print columns](#machineset-print-columns)
    - [Changes to MachineDeployment resource](#changes-to-machinedeployment-resource)
      - [MachineDeployment Status](#machinedeployment-status)
      - [MachineDeployment (New)Conditions](#machinedeployment-newconditions)
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
    - [Changes to Cluster API contract](#changes-to-cluster-api-contract)

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
    - Standardize replica counters on control plane, MachineDeployment, MachinePool, and bubble them up to the Cluster resource.
    - Bubble up conditions about Machine readiness to control plane, MachineDeployment, MachinePool.
- Introduce missing signals about connectivity to workload clusters, thus enabling to mark all the conditions
  depending on such connectivity with status Unknown after a certain amount of time.
- Introduce a cleaner signal about Cluster API resources lifecycle transitions, e.g. scaling up or updating.
- Ensure everything in status can be used as a signal informing monitoring tools/automation on top of Cluster API 
  about lifecycle transitions/state of the Cluster and the underlying components as well.

### Non-Goals

- Resolving all the idiosyncrasies that exists in Cluster API, core Kubernetes, the rest of the ecosystem.
  (Let’s stay focused on Cluster API and keep improving incrementally).
- To change how the Cluster API contract with infrastructure, bootstrap and control providers currently works
  (by using status fields).

## Proposal

This proposal groups a set of changes to status fields in Cluster API resources.

Some of those changes could be considered straight forward, e.g.

- K8s API conventions suggest to deprecate and remove `phase` fields from status, Cluster API is going to align to this recommendation
  (and improve Conditions to provide similar or even better info as a replacement).
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
- Transition to K8s API conventions fully aligned conditions types/condition management (and thus deprecation of
  the Cluster API "custom" guidelines for conditions).

The last set of changes is a consequence of the above changes, or small improvements to address feedback received
over time; changes in this group will be detailed case by case in the following paragraphs, a few examples:

- Change the semantic of ReadyReplica counters to use Machine's Ready condition instead of Node's Ready condition.
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

Overall, the union of all those changes, is expected to greatly improve status fields, conditions, replica counters 
and print columns.

Those improvements are expected to provide benefit to users interacting with the system, using monitoring tools, and 
building higher level systems or products on top of Cluster API.

### Readiness and Availability

The [condition CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20200506-conditions.md) in Cluster API introduced very strict requirements about `Ready` conditions, mandating it 
to exists on all resources and also mandating that `Ready` must be computed as the summary of all other existing
conditions.

However, over time Cluster API maintainers recognized several limitations of the “one fits all”, strict approach.

E.g., higher level abstractions in Cluster API are designed to remain operational during lifecycle operations,
for instance a MachineDeployment is operational even if is rolling out.

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

- This proposal is ensuring that whenever Machine ready is used, it always means the same thing (e.g. replica counters)
- This proposal is also changing contract fields where ready was used improperly to represent 
  initial provisioning (k8s API conventions suggest to use ready only for long-running process).

All in all, Machine's Ready concept should be much more clear, consistent, intuitive after proposed changes.
But there is more.

This proposal is also dropping the `Ready` condition from higher level abstractions in Cluster API.

Instead, where not already present, this proposal is introducing a new `Available` condition that better represents
the fact that those objects are operational even if there is a certain degree of not readiness / disruption in the system
or if lifecycle operations are happening (prior art `Available` condition in K8s Deployments).

Last but not least:

- With the changes to the semantic of `Ready` and `Available` conditions, it is now possible to add conditions to
  surface ongoing lifecycle operations, e.g. scaling up.
- As suggested by K8s API conventions, this proposal is also making sure all conditions are consistent and have
  uniform meaning across all resource types
- Additionally, we are enforcing the same consistency for replica counters and other status fields.

### Transition to K8s API conventions aligned conditions

K8s is undergoing an effort of standardizing usage of conditions across all resource types, and the transition to
the v1beta2 API version is a great opportunity for Cluster API to align to this effort.

The value of this transition is substantial, because the differences that exists today's are really confusing for users;
those differences are also making it harder for ecosystem tools to build on top of Cluster API, and in some cases
even confusing new (and old) contributors.

With this proposal Cluster API will close the gap with K8s API conventions in regard to:
- Polarity: Condition type names should make sense for humans; neither positive nor negative polarity can be recommended
  as a general rule (already implemented by [#10550](https://github.com/kubernetes-sigs/cluster-api/pull/10550))
- Use of the `Reason` field is required (currently in Cluster API reasons is added only when condition are false)
- Controllers should apply their conditions to a resource the first time they visit the resource, even if the status is `Unknown`.
  (currently Cluster API controllers add conditions at different stages of the reconcile loops)
- Cluster API is also dropping its own `Condition` type and will start using `metav1.Conditions` from the Kubernetes API.

The last point also has another implication, which is the removal of the `Severity` field which is currently used
to determine priority when merging conditions into the ready summary.

However, considering all the work to clean up and improve readiness and availability, now dropping the `Severity` field
is not an issue anymore. Let's clarify this with an example:

When Cluster API will compute Machine `Ready` there will be a very limited set of conditions
to merge (see [next paragraph](#machine-newconditions)). Considering this, it will be probably simpler and more informative
for users if we surface all relevant messages instead of arbitrarily dropping some of them as we are doing
today by inferring merge priority from the `Severity` field.

In case someone wants a more sophisticated control over the process of merging conditions, the new version of the
condition utils in Cluster API will allow developers to plug in custom functions to compute merge priority
for a condition, e.g. by looking at status, reason, time since the condition transitioned, etc.

### Changes to Machine resource

#### Machine Status

Following changes are implemented to Machine's status:

- Disambiguate usage of ready term by renaming fields used for the provisioning workflow
- Align to K8s API conventions by deprecating `Phase` and corresponding `LastUpdated`
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions

Below you can find the relevant fields in Machine Status v1beta2, after v1beta1 removal (end state);
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
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    
    // Other fields...
    // NOTE: `Phase`, `LastUpdated`, `FailureReason`, `FailureMessage`, `BootstrapReady`, `InfrastructureReady` fields won't be there anymore
}

// MachineInitializationStatus provides observations of the Machine initialization process.
type MachineInitializationStatus struct {

    // BootstrapSecretCreated is true when the bootstrap provider reports that the Machine's boostrap secret is created.
    // NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Machine provisioning.
    // The value of this field is never updated after provisioning is completed.
    // Use conditions to monitor the operational state of the Machine's BootstrapSecret.
    // +optional
    BootstrapSecretCreated bool `json:"bootstrapSecretCreated"`
    
    // InfrastructureProvisioned is true when the infrastructure provider reports that the Machine's infrastructure is fully provisioned.
    // NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Machine provisioning.
    // The value of this field is never updated after provisioning is completed.
    // Use conditions to monitor the operational state of the Machine's infrastructure.
    // +optional
    InfrastructureProvisioned bool `json:"infrastructureProvisioned"`
}
```

| v1beta1 (current)              | v1beta2 (tentative Q1 2025)                              | v1beta1 removal (tentative Q1 2026)        |
|--------------------------------|----------------------------------------------------------|--------------------------------------------|
|                                | `Initialization` (new)                                   | `Initialization`                           |
| `BootstrapReady`               | `Initialization.BootstrapSecretCreated` (renamed)        | `Initialization.BootstrapSecretCreated`    |
| `InfrastructureReady`          | `Initialization.InfrastructureProvisioned` (renamed)     | `Initialization.InfrastructureProvisioned` |
|                                | `BackCompatibilty` (new)                                 | (removed)                                  |
| `Phase` (deprecated)           | `BackCompatibilty.Phase` (renamed) (deprecated)          | (removed)                                  |
| `LastUpdated` (deprecated)     | `BackCompatibilty.LastUpdated` (renamed) (deprecated)    | (removed)                                  |
| `FailureReason` (deprecated)   | `BackCompatibilty.FailureReason` (renamed) (deprecated)  | (removed)                                  |
| `FailureMessage` (deprecated)  | `BackCompatibilty.FailureMessage` (renamed) (deprecated) | (removed)                                  |
| `Conditions`                   | `BackCompatibilty.Conditions` (renamed) (deprecated)     | (removed)                                  |
| `ExperimentalConditions` (new) | `Conditions` (renamed)                                   | `Conditions`                               |
| other fields...                | other fields...                                          | other fields...                            |

Notes:
- The `BackCompatibilty` struct is going to exist in v1beta2 types only until v1beta1 removal (9 months or 3 minor releases after v1beta2 is released/v1beta1 is deprecated, whichever is longer).
  Fields in this struct are used for supporting down conversions, thus providing users relying on v1beta1 APIs additional buffer time to pick up the new changes. 

##### Machine (New)Conditions

| Condition              | Note                                                                                                                                                                                                                                                            |
|------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Available`            | True if at the machine is Ready for at least MinReady seconds, as defined by the Machine's minReadySeconds field                                                                                                                                                |
| `Ready`                | True if Machine's `BootstrapSecretReady`, `InfrastructureReady`, `NodeHealthy` and `HealthCheckSucceeded` (if present) are true; if other conditions are defined in `spec.readinessGates`, those conditions should be true as well for the Machine to be ready. |
| `UpToDate`             | True if the Machine spec matches the spec of the Machine's owner resource, e.g KubeadmControlPlane or MachineDeployment                                                                                                                                         |
| `BootstrapConfigReady` | Mirrors the corresponding condition from the Machine's BootstrapConfig resource                                                                                                                                                                                 |
| `InfrastructureReady`  | Mirrors the corresponding condition from the Machine's Infrastructure resource                                                                                                                                                                                  |
| `NodeReady`            | True if the Machine's Node is ready                                                                                                                                                                                                                             |
| `NodeHealthy`          | True if the Machine's Node is ready and it does not report MemoryPressure, DiskPressure and PIDPressure                                                                                                                                                         |
| `HealthCheckSucceeded` | True if MHC instances targeting this machine report the Machine is healthy according to the definition of healthy present in the spec of the Machine Health Check object                                                                                        |
| `OwnerRemediated`      |                                                                                                                                                                                                                                                                 |
| `Deleted`              | True if Machine is deleted; Reason can be used to observe the cleanup progress when the resource is deleted                                                                                                                                                     |
| `Paused`               | True if the Machine or the Cluster it belongs to are paused                                                                                                                                                                                                     |

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
  more specifically those condition should be set to `Unknown` after the cluster probe fails
  (or after whatever period is defined in the `--remote-conditions-grace-period` flag)
- `HealthCheckSucceeded` and `OwnerRemediated` (or `ExternalRemediationRequestAvailable`) conditions are set by the 
  MachineHealthCheck controller in case a MachineHealthCheck targets the machine.
- KubeadmControlPlane also adds additional conditions to Machines, but those conditions are not included in the table above
  for sake of simplicity (however they are documented in the KubeadmControlPlane paragraph).

TODO: think carefully at remote conditions becoming unknown, this could block a few operations ... 

#### Machine Spec

Machine's spec is going to be improved to allow 3rd party components to extend the semantic of the new Machine's `Ready` condition
as well as to standardize the concept of Machine's `Availability`.

Below you can find the relevant fields in Machine Status v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```go
type MachineSpec struct {

    // MinReadySeconds is the minimum number of seconds for which a Machine should be ready before considering the replica available.
    // Defaults to 0 (machine will be considered available as soon as the Node is ready)
    // +optional
    MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

    // If specified, all readiness gates will be evaluated for Machine readiness.
    // A Machine is ready when `InfrastructureReady`, `NodeHealthy` and `HealthCheckSucceeded` (if present) are "True"; 
    // if other conditions are defined in this field, those conditions should be "True" as well for the Machine to be ready.
    // +optional
    // +listType=map
    // +listMapKey=conditionType
    ReadinessGates []MachineReadinessGate `json:"readinessGates,omitempty"`

    // Other fields...
}

// MachineReadinessGate contains the reference to a Machine condition to be used as readiness gates.
type MachineReadinessGate struct {
    // ConditionType refers to a condition in the Machine's condition list with matching type.
    // Note: Both  Cluster API conditions or conditions added by 3rd party controller can be used as readiness gates.
    ConditionType string `json:"conditionType"`
}
```

| v1beta1 (current)      | v1Beta2 (tentative Q1 2025) | v1beta1 removal (tentative Q1 2026) |
|------------------------|-----------------------------|-------------------------------------|
| `ReadinessGates` (new) | `ReadinessGates`            | `ReadinessGates`                    |
| other fields...        | other fields...             | other fields...                     |

Notes:
- Both `MinReadySeconds` and `ReadinessGates` should be treated as other in-place propagated fields (changing this should not trigger rollouts).
- Similarly to Pod's `ReadinessGates`, also Machine's `ReadinessGates` accept only conditions with positive polarity; 
  The Cluster API project might revisit this in future to stay aligned with Kubernetes or if there are use cases justifying this change.

#### Machine Print columns

| Current           | To be                         |
|-------------------|-------------------------------|
| `NAME`            | `NAME`                        |
| `CLUSTER`         | `CLUSTER`                     |
| `NODE NAME`       | `PAUSED` (new) (*)            |
| `PROVIDER ID`     | `NODE NAME`                   |
| `PHASE` (deleted) | `PROVIDER ID`                 |
| `AGE`             | `READY` (new)                 |
| `VERSION`         | `AVAILABLE` (new)             |
|                   | `UP TO DATE` (new)            |
|                   | `AGE`                         |
|                   | `OS-IMAGE` (new) (*)          |
|                   | `KERNEL-VERSION` (new) (*)    |
|                   | `CONTAINER-RUNTIME` (new) (*) |

TODO: figure out if can `INTERNAL-IP` (new) (*),   `EXTERNAL-IP` after `VERSION` / before `OS-IMAGE`?  (similar to Nodes...).
might be something like `$.status.addresses[?(@.type == 'InternalIP')].address` works, but not sure what happens if there are 0 or more addresses...
  Stefan +1 if possible

(*) visible only when using `kubectl get -o wide`

### Changes to MachineSet resource

#### MachineSet Status

Following changes are implemented to MachineSet's status:

- Update `ReadyReplicas` counter to use the same semantic Machine's `Ready` (today it is computed a Machines with Node Ready) condition and add missing `UpToDateReplicas`.
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions

Below you can find the relevant fields in Machine Status v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type MachineSetStatus struct {

    // The number of ready replicas for this MachineSet. A machine is considered ready when Machine's Ready condition is true.
    // +optional
    ReadyReplicas int32 `json:"readyReplicas"`
	
	// The number of available replicas for this MachineSet. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas"`

    // The number of up-to-date replicas for this MachineSet. A machine is considered up-to-date when Machine's UpToDate condition is true.
    // +optional
    UpToDateReplicas int32 `json:"upToDateReplicas"`

    // Represents the observations of a MachineSet's current state.
    // +optional
    // +listType=map
    // +listMapKey=type
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // Other fields...
    // NOTE: `FailureReason`, `FailureMessage` fields won't be there anymore
}
```

| v1beta1 (current)                    | v1beta2 (tentative Q1 2025)                                 | v1beta1 removal (tentative Q1 2026) |
|--------------------------------------|-------------------------------------------------------------|-------------------------------------|
| `ExprimentalReadyReplicas` (new)     | `ReadyReplicas` (renamed)                                   | `ReadyReplicas`                     |
| `ExprimentalAvailableReplicas` (new) | `AvailableReplicas` (renamed)                               | `AvailableReplicas`                 |
|                                      | `BackCompatibilty` (new)                                    | (removed)                           |
| `ReadyReplicas` (deprecated)         | `BackCompatibilty.ReadyReplicas` (renamed) (deprecated)     | (removed)                           |
| `AvailableReplicas` (deprecated)     | `BackCompatibilty.AvailableReplicas` (renamed) (deprecated) | (removed)                           |
| `FailureReason` (deprecated)         | `BackCompatibilty.FailureReason` (renamed) (deprecated)     | (removed)                           |
| `FailureMessage` (deprecated)        | `BackCompatibilty.FailureMessage` (renamed) (deprecated)    | (removed)                           |
| `Conditions`                         | `BackCompatibilty.Conditions` (renamed) (deprecated)        | (removed)                           |
| `ExperimentalConditions` (new)       | `Conditions` (renamed)                                      | `Conditions`                        |
| `UpToDateReplicas` (new)             | `UpToDateReplicas`                                          | `UpToDateReplicas`                  |
| other fields...                      | other fields...                                             | other fields...                     |

Notes:
- The `BackCompatibilty` struct is going to exist in v1beta2 types only until v1beta1 removal (9 months or 3 minor releases after v1beta2 is released/v1beta1 is deprecated, whichever is longer).
  Fields in this struct are used for supporting down conversions, thus providing users relying on v1beta1 APIs additional buffer time to pick up the new changes.
- This proposal is using `UpToDateReplicas` instead of `UpdatedReplicas`; This is a deliberated choice to avoid 
  confusion between update (any change) and upgrade (change of the Kubernetes versions).
- Also `AvailableReplicas` will determine Machine's availability by reading Machine.Available condition instead of
  computing availability as of today, however in this case the semantic of the field is not changed

TODO: check `FullyLabeledReplicas`, do we still need it?

#### MachineSet (New)Conditions

| Condition        | Note                                                                                                             |
|------------------|------------------------------------------------------------------------------------------------------------------|
| `ReplicaFailure` | This condition surfaces issues on creating a Machine replica in Kubernetes, if any. e.g. due to resource quotas. |
| `MachinesReady`  | This condition surfaces detail of issues on the controlled machines, if any.                                     |
| `ScalingUp`      | True if available replicas < desired replicas                                                                    |
| `ScalingDown`    | True if replicas > desired replicas                                                                              |
| `UpToDate`       | True if all the Machines controlled by this MachineSet are up to date (replicas = upToDate replicas)             |
| `Remediating`    | True if there is at least one Machine controlled by this MachineSet that is not passing health checks            |
| `Deleted`        | True if MachineSet is deleted; Reason can be used to observe the cleanup progress when the resource is deleted   |
| `Paused`         | True if this MachineSet or the Cluster it belongs to are paused                                                  |

> To better evaluate proposed changes, below you can find the list of current MachineSet's conditions:
> Ready, MachinesCreated, Resized, MachinesReady.

Notes:
- MachineSet conditions are intentionally mostly consistent with MachineDeployment conditions to help users troubleshooting .
- MachineSet is considered as a sort of implementation detail of MachineDeployments, so it doesn't have its own concept of availability.
  Similarly, this proposal is dropping the notion of MachineSet readiness because it is preferred to let users focusing on Machines readiness.
- `Remediating` for older MachineSet sets will report that remediation will happen as part of the regular rollout.
- `UpToDate` condition initially will be `false` for older MachineSet, `true` for the current MachineSet; however in
  the future the latter might evolve in case Cluster API will start supporting in-place upgrades.

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
- In k8s Deployment and ReplicaSet have different print columns for replica counters; this proposal enforces replicas
  counter columns consistent across all resources.

### Changes to MachineDeployment resource

#### MachineDeployment Status

Following changes are implemented to MachineDeployment's status:

- Align `UpdatedReplicas` to use Machine's `UpToDate` condition (and rename it accordingly)
- Align to K8s API conventions by deprecating `Phase`
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions

Below you can find the relevant fields in Machine Status v1beta2, after v1beta1 removal (end state);
After golang types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type MachineDeploymentStatus struct {

    // The number of ready replicas for this MachineDeployment. A machine is considered ready when Machine's Ready condition is true.
    // +optional
    ReadyReplicas int32 `json:"readyReplicas"`

    // The number of available replicas for this MachineDeployment. A machine is considered available when Machine's Available condition is true.
    // +optional
    AvailableReplicas int32 `json:"availableReplicas"`

    // The number of up-to-date replicas targeted by this deployment.
    // +optional
    UpToDateReplicas int32 `json:"upToDateReplicas"`

    // Represents the observations of a MachineDeployment's current state.
    // +optional
    // +listType=map
    // +listMapKey=type
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // Other fields...
    // NOTE: `Phase`, `FailureReason`, `FailureMessage` fields won't be there anymore
}
```

| v1beta1 (current)                    | v1beta2 (tentative Q1 2025)                                 | v1beta1 removal (tentative Q1 2026) |
|--------------------------------------|-------------------------------------------------------------|-------------------------------------|
| `UpdatedReplicas`                    | `UpToDateReplicas` (renamed)                                | `UpToDateReplicas`                  |
| `ExprimentalReadyReplicas` (new)     | `ReadyReplicas` (renamed)                                   | `ReadyReplicas`                     |
| `ExprimentalAvailableReplicas` (new) | `AvailableReplicas` (renamed)                               | `AvailableReplicas`                 |
|                                      | `BackCompatibilty` (new)                                    | (removed)                           |
| `ReadyReplicas` (deprecated)         | `BackCompatibilty.ReadyReplicas` (renamed) (deprecated)     | (removed)                           |
| `AvailableReplicas` (deprecated)     | `BackCompatibilty.AvailableReplicas` (renamed) (deprecated) | (removed)                           |
| `Phase` (deprecated)                 | `BackCompatibilty.Phase` (renamed) (deprecated)             | (removed)                           |
| `FailureReason` (deprecated)         | `BackCompatibilty.FailureReason` (renamed) (deprecated)     | (removed)                           |
| `FailureMessage` (deprecated)        | `BackCompatibilty.FailureMessage` (renamed) (deprecated)    | (removed)                           |
| `Conditions`                         | `BackCompatibilty.Conditions` (renamed) (deprecated)        | (removed)                           |
| `ExperimentalConditions` (new)       | `Conditions` (renamed)                                      | `Conditions`                        |
| other fields...                      | other fields...                                             | other fields...                     |

Notes:
- The `BackCompatibilty` struct is going to exist in v1beta2 types only until v1beta1 removal (9 months or 3 minor releases after v1beta2 is released/v1beta1 is deprecated, whichever is longer).
  Fields in this struct are used for supporting down conversions, thus providing users relying on v1beta1 APIs additional buffer time to pick up the new changes.

#### MachineDeployment (New)Conditions

| Condition        | Note                                                                                                                                                                                                                                                   |
|------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Available`      | True if the MachineDeployment has minimum availability according to parameters specified in the deployment strategy, e.g. If using RollingUpgrade strategy, availableReplicas must be greater or equal than desired replicas - MaxUnavailable replicas |  
| `ReplicaFailure` | This condition surfaces issues on creating a MachineSet replica in Kubernetes, if any. e.g. due to resource quotas.                                                                                                                                    |
| `MachinesReady`  | This condition surfaces detail of issues on the controlled machines, if any.                                                                                                                                                                           |
| `ScalingUp`      | True if available replicas < desired replicas                                                                                                                                                                                                          |
| `ScalingDown`    | True if replicas > desired replicas                                                                                                                                                                                                                    |
| `UpToDate`       | True if all the Machines controlled by this MachineDeployment are up to date (replicas = upToDate replicas)                                                                                                                                            |
| `Remediating`    | True if there is at least one machine controlled by this MachineDeployment is not passing health checks                                                                                                                                                |
| `Deleted`        | True if MachineDeployment is deleted; Reason can be used to observe the cleanup progress when the resource is deleted                                                                                                                                  |
| `Paused`         | True if this MachineDeployment or the Cluster it belongs to are paused                                                                                                                                                                                 |

> To better evaluate proposed changes, below you can find the list of current MachineDeployment's conditions:
> Ready, Available.

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
| `PHASE` (deleted)       | `UP-TO-DATE` (renamed) |
| `AGE`                   | `AGE`                  |
| `VERSION`               | `VERSION`              |

TODO: consider if to add MachineDeployment `AVAILABLE`, but we should find a way to differentiate from `AVAILABLE` replicas
  Stefan +1 to have AVAILABLE, not sure if we can have two columns with the same header

(*) visible only when using `kubectl get -o wide`

### Changes to Cluster resource

#### Cluster Status

Following changes are implemented to Cluster's status:

- Disambiguate usage of ready term by renaming fields used for the provisioning workflow
- Align to K8s API conventions by deprecating `Phase` and corresponding `LastUpdated`
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions
- Add replica counters to surface status of Machines belonging to this Cluster
- Surface information about ControlPlane connection heartbeat (see new conditions)

Below you can find the relevant fields in Machine Status v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type ClusterStatus struct {

    // Initialization provides observations of the Cluster initialization process.
    // NOTE: fields in this struct are part of the Cluster API contract and are used to orchestrate initial Cluster provisioning.
    // The value of those fields is never updated after provisioning is completed.
    // Use conditions to monitor the operational state of the Cluster's BootstrapSecret.
    // +optional
    Initialization *MachineInitializationStatus `json:"initialization,omitempty"`
    
    // Represents the observations of a Cluster's current state.
    // +optional
    // +listType=map
    // +listMapKey=type
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    
    // ControlPlane groups all the observations about Cluster's ControlPlane current state.
    // +optional
    ControlPlane *ClusterControlPlaneStatus `json:"controlPlane,omitempty"`
    
    // Workers groups all the observations about Cluster's Workers current state.
    // +optional
    Workers *ClusterControlPlaneStatus `json:"workers,omitempty"`
    
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
    DesiredReplicas int32 `json:"desiredReplicas"`

    // Total number of non-terminated control plane machines in this cluster.
    // +optional
    Replicas int32 `json:"replicas"`
    
    // The number of up-to-date control plane machines in this cluster.
    // +optional
    UpToDateReplicas int32 `json:"upToDateReplicas"`
    
    // Total number of ready control plane machines in this cluster.
    // +optional
    ReadyReplicas int32 `json:"readyReplicas"`
    
    // Total number of available control plane machines in this cluster.
    // +optional
    AvailableReplicas int32 `json:"availableReplicas"`
}

// WorkersPlaneStatus groups all the observations about workers current state.
type WorkersPlaneStatus struct {
    // Total number of desired worker machines in this cluster.
    // +optional
    DesiredReplicas int32 `json:"desiredReplicas"`

    // Total number of non-terminated worker machines in this cluster.
    // +optional
    Replicas int32 `json:"replicas"`
    
    // The number of up-to-date worker machines in this cluster.
    // +optional
    UpToDateReplicas int32 `json:"upToDateReplicas"`
    
    // Total number of ready worker machines in this cluster.
    // +optional
    ReadyReplicas int32 `json:"readyReplicas"`
    
    // Total number of available worker machines in this cluster.
    // +optional
    AvailableReplicas int32 `json:"availableReplicas"`
}
```

// TODO: check about "non-terminated" for replicas fields.

| v1beta1 (current)                        | v1beta2 (tentative Q1 2025)                              | v1beta1 removal (tentative Q1 2026)        |
|------------------------------------------|----------------------------------------------------------|--------------------------------------------|
|                                          | `Initialization` (new)                                   | `Initialization`                           |
| `InfrastructureReady`                    | `Initialization.InfrastructureProvisioned` (renamed)     | `Initialization.InfrastructureProvisioned` |
| `ControlPlaneReady`                      | `Initialization.ControlPlaneInitialized` (renamed)       | `Initialization.ControlPlaneInitialized`   |
|                                          | `BackCompatibilty` (new)                                 | (removed)                                  |
| `Phase` (deprecated)                     | `BackCompatibilty.Phase` (renamed) (deprecated)          | (removed)                                  |
| `FailureReason` (deprecated)             | `BackCompatibilty.FailureReason` (renamed) (deprecated)  | (removed)                                  |
| `FailureMessage` (deprecated)            | `BackCompatibilty.FailureMessage` (renamed) (deprecated) | (removed)                                  |
| `Conditions`                             | `BackCompatibilty.Conditions` (renamed) (deprecated)     | (removed)                                  |
| `ExperimentalConditions` (new)           | `Conditions` (renamed)                                   | `Conditions`                               |
| `ControlPlane` (new)                     | `ControlPlane`                                           | `ControlPlane`                             |
| `ControlPlane.DesiredReplicas` (new)     | `ControlPlane.DesiredReplicas`                           | `ControlPlane.DesiredReplicas`             |
| `ControlPlane.Replicas` (new)            | `ControlPlane.Replicas`                                  | `ControlPlane.Replicas`                    |
| `ControlPlane.ReadyReplicas` (new)       | `ControlPlane.ReadyReplicas`                             | `ControlPlane.ReadyReplicas`               |
| `ControlPlane.UpToDateReplicas` (new)    | `ControlPlane.UpToDateReplicas`                          | `ControlPlane.UpToDateReplicas`            |
| `ControlPlane.AvailableReplicas` (new)   | `ControlPlane.AvailableReplicas`                         | `ControlPlane.AvailableReplicas`           |
| `Workers` (new)                          | `Workers`                                                | `Workers`                                  |
| `Workers.DesiredReplicas` (new)          | `Workers.DesiredReplicas`                                | `Workers.DesiredReplicas`                  |
| `Workers.Replicas` (new)                 | `Workers.Replicas`                                       | `Workers.Replicas`                         |
| `Workers.ReadyReplicas` (new)            | `Workers.ReadyReplicas`                                  | `Workers.ReadyReplicas`                    |
| `Workers.UpToDateReplicas` (new)         | `Workers.UpToDateReplicas`                               | `Workers.UpToDateReplicas`                 |
| `Workers.AvailableReplicas` (new)        | `Workers.AvailableReplicas`                              | `Workers.AvailableReplicas`                |
| other fields...                          | other fields...                                          | other fields...                            |

notes:
- The `BackCompatibilty` struct is going to exist in v1beta2 types only until v1beta1 removal (9 months or 3 minor releases after v1beta2 is released/v1beta1 is deprecated, whichever is longer).
  Fields in this struct are used for supporting down conversions, thus providing users relying on v1beta1 APIs additional buffer time to pick up the new changes.

##### Cluster (New)Conditions

| Condition                 | Note                                                                                                                                                                                                                                                                                                                  |
|---------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Available`               | True if Cluster `RemoteConnectionProbe` is true, if Cluster's control plane `Available` condition is true, if all MachineDeployment and MachinePool's `Available` condition are true; if conditions are defined in `spec.availabilityGates`, those conditions should be true as well for the Cluster to be available. |
| `ControlPlaneInitialized` | True when the Cluster's control plane is functional enough to accept requests. This information is usually used as a signal for starting all the provisioning operations that depends on a functional API server, but do not require a full HA control plane to exists.                                               |
| `RemoteConnectionProbe`   | True when control plane can be reached; in case of connection problems, the condition turns to false only if the the cluster cannot be reached for 40s after the first connection problem is detected (or whatever period is defined in the `--remote-connection-grace-period` flag) the cluster cannot be reached    |
| `InfrastructureReady`     | Mirror of Cluster's infrastructure `Ready` condition                                                                                                                                                                                                                                                                  |
| `ControlPlaneAvailable`   | Mirror of Cluster's control plane `Available` condition                                                                                                                                                                                                                                                               |
| `WorkersAvaiable`         | Summary of MachineDeployment and MachinePool's `Available` condition                                                                                                                                                                                                                                                  |
| `TopologyReconciled`      |                                                                                                                                                                                                                                                                                                                       |
| `ScalingUp`               | True if available replicas < desired replicas                                                                                                                                                                                                                                                                         |
| `ScalingDown`             | True if replicas > desired replicas                                                                                                                                                                                                                                                                                   |
| `UpToDate`                | True if all the Machines controlled by this Cluster are up to date (replicas = upToDate replicas)                                                                                                                                                                                                                     |
| `Remediating`             | True if there is at least one machine controlled by this Cluster is not passing health checks                                                                                                                                                                                                                         |
| `Deleted`                 | True if Cluster is deleted; Reason can be used to observe the cleanup progress when the resource is deleted                                                                                                                                                                                                           |
| `Paused`                  | True if Cluster and all the resources being part of it are paused                                                                                                                                                                                                                                                     |

> To better evaluate proposed changes, below you can find the list of current Cluster's conditions:
> Ready, InfrastructureReady, ControlPlaneReady, ControlPlaneInitialized, TopologyReconciled

Notes:
- `TopologyReconciled` exists only for classy clusters; this condition is managed by the topology reconciler.
- Cluster API is going to maintain a `lastRemoteConnectionProbeTime` and use it in combination with the
  `--remote-connection-grace-period` flag to avoid flakes on `RemoteConnectionProbe`.
- Similarly to `lastHeartbeatTime` in Kubernetes conditions, also `lastRemoteConnectionProbeTime` will not surface on the 
  API in order to avoid costly, continuous reconcile events.

#### Cluster Spec

Cluster's spec is going to be improved to allow 3rd party to extend the semantic of the new Cluster's `Available` condition.

Below you can find the relevant fields in Machine Status v1beta2, after v1beta1 removal (end state);
After golang types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

| v1beta1 (current)         | v1Beta2 (tentative Q1 2025) | v1beta1 removal (tentative Q1 2026) |
|---------------------------|-----------------------------|-------------------------------------|
| `AvailabilityGates` (new) | `AvailabilityGates`         | `AvailabilityGates`                 |
| other fields...           | other fields...             | other fields...                     |

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

// ClusterAvailabilityGate contains the reference to a Cluster condition to be used as availability gates.
type ClusterAvailabilityGate struct {
    // ConditionType refers to a condition in the Cluster's condition list with matching type.
    // Note: Both Cluster API conditions or conditions added by 3rd party controller can be used as availability gates. 
    ConditionType string `json:"conditionType"`
}
```

Notes:
- Similarly to Pod's `ReadinessGates`, also Machine's `AvailabilityGates` accept only conditions with positive polarity;
  The Cluster API project might revisit this in the future to stay aligned with Kubernetes or if there are use cases justifying this change.
- In future the Cluster API project might consider ways to make `AvailabilityGates` configurable at ClusterClass level, but
  this can be implemented as a follow-up.

#### Cluster Print columns

| Current           | To be                 |
|-------------------|-----------------------|
| `NAME`            | `NAME`                |
| `CLUSTER CLASS`   | `CLUSTER CLASS`       |
| `PHASE` (deleted) | `PAUSED` (new) (*)    |
| `AGE`             | `AVAILABLE` (new)     |
| `VERSION`         | `CP_DESIRED` (new)    |
|                   | `CP_CURRENT`(new) (*) |
|                   | `CP_READY` (new) (*)  |
|                   | `CP_AVAILABLE` (new)  |
|                   | `CP_UP_TO_DATE` (new) |
|                   | `W_DESIRED` (new)     |
|                   | `W_CURRENT`(new) (*)  |
|                   | `W_READY` (new) (*)   |
|                   | `W_AVAILABLE` (new)   |
|                   | `W_UP_TO_DATE` (new)  |
|                   | `AGE`                 |
|                   | `VERSION`             |

(*) visible only when using `kubectl get -o wide`

### Changes to KubeadmControlPlane (KCP) resource

#### KubeadmControlPlane Status

Following changes are implemented to KubeadmControlPlane's status:

- TODO: figure out what to do with contract fields + conditions
- Update `ReadyReplicas` counter to use the same semantic Machine's `Ready` condition and add missing `UpToDateReplicas`.
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions

Below you can find the relevant fields in Machine Status v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type KubeadmControlPlaneStatus struct {

    // The number of ready replicas for this ControlPlane. A machine is considered ready when Machine's Ready condition is true.
    // Note: In the v1beta1 API version a Machine was counted as ready when the node hosted on the Machine was ready, thus 
    // generating confusion for users looking at the Machine.Ready condition.
    // +optional
    ReadyReplicas int32 `json:"readyReplicas"`

    // The number of available replicas targeted by this ControlPlane.
    // +optional
    AvailableReplicas int32 `json:"availableReplicas"`
	
    // The number of up-to-date replicas targeted by this ControlPlane.
    // +optional
    UpToDateReplicas int32 `json:"upToDateReplicas"`

    // Represents the observations of a ControlPlane's current state.
    // +optional
    // +listType=map
    // +listMapKey=type
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // Other fields...
	// NOTE: `Ready`, `FailureReason`, `FailureMessage` fields won't be there anymore
}
```

| v1beta1 (current)                 | v1beta2 (tentative Q1 2025)                              | v1beta1 removal (tentative Q1 2026) |
|-----------------------------------|----------------------------------------------------------|-------------------------------------|
| `Ready` (deprecated)              | `Ready` (deprecated)                                     | (removed)                           |
|                                   | `BackCompatibilty` (new)                                 | (removed)                           |
| `ReadyReplicas` (deprecated)      | `BackCompatibilty.ReadyReplicas` (renamed) (deprecated)  | (removed)                           |
| `ExperimentalReadyReplicas` (new) | `ReadyReplicas` (renamed)                                | `ReadyReplicas`                     |
| `UpdatedReplicas`                 | `UpToDateReplicas` (renamed)                             | `UpToDateReplicas`                  |
| `AvailableReplicas` (new)         | `AvailableReplicas`                                      | `AvailableReplicas`                 |
| `FailureReason` (deprecated)      | `BackCompatibilty.FailureReason` (renamed) (deprecated)  | (removed)                           |
| `FailureMessage` (deprecated)     | `BackCompatibilty.FailureMessage` (renamed) (deprecated) | (removed)                           |
| `Conditions`                      | `BackCompatibilty.Conditions` (renamed) (deprecated)     | (removed)                           |
| `ExperimentalConditions` (new)    | `Conditions` (renamed)                                   | `Conditions`                        |
| other fields...                   | other fields...                                          | other fields...                     |

TODO: double check usages of status.ready.

#### KubeadmControlPlane (New)Conditions

| Condition                       | Note                                                                                                                    |
|---------------------------------|-------------------------------------------------------------------------------------------------------------------------|
| `Available`                     | True if the control plane can be reached and there is etcd quorum, and `CertificatesAvailable` is true                  |
| `CertificatesAvailable`         | True if all the cluster certificates exist.                                                                             |
| `ReplicaFailure`                | This condition surfaces issues on creating Machines controlled by this KubeadmControlPlane, if any.                     |
| `Initialized`                   | True if ControlPlaneComponentsHealthy.                                                                                  |
| `ControlPlaneComponentsHealthy` | This condition surfaces detail of issues on the controlled machines, if any.                                            |
| `EtcdClusterHealthy`            | This condition surfaces detail of issues on the controlled machines, if any.                                            |
| `MachinesReady`                 | This condition surfaces detail of issues on the controlled machines, if any.                                            |
| `ScalingUp`                     | True if available replicas < desired replicas                                                                           |
| `ScalingDown`                   | True if replicas > desired replicas                                                                                     |
| `UpToDate`                      | True if all the Machines controlled by this ControlPlane are up to date                                                 |
| `Remediating`                   | True if there is at least one machine controlled by this KubeadmControlPlane is not passing health checks               |
| `Deleted`                       | True if KubeadmControlPlane is deleted; Reason can be used to observe the cleanup progress when the resource is deleted |
| `Paused`                        | True if this resource or the Cluster it belongs to are paused                                                           |

> To better evaluate proposed changes, below you can find the list of current KubeadmControlPlane's conditions:
> Ready, CertificatesAvailable, MachinesCreated, Available, MachinesSpecUpToDate, Resized, MachinesReady,
> ControlPlaneComponentsHealthy, EtcdClusterHealthy.

Notes:
- `ControlPlaneComponentsHealthy` and `EtcdClusterHealthy` have a very strict semantic: everything should be ok for the condition to be true;
  This means it is expected those condition to flick while performing lifecycle operations; over time we might consider changes to make
  those conditions to distinguish more accurately health issues vs "expected" temporary unavailability.

#### KubeadmControlPlane Print columns

| Current                  | To be                  |
|--------------------------|------------------------|
| `NAME`                   | `NAME`                 |
| `CLUSTER`                | `CLUSTER`              |
| `DESIRED` (*)            | `PAUSED` (new) (*)     |
| `REPLICAS`               | `INITIALIZED` (new)    |
| `READY`                  | `DESIRED`              |
| `UPDATED` (renamed)      | `CURRENT` (*)          |
| ``UNAVAILABLE` (deleted) | `READY`                |
| `PHASE` (deleted)        | `AVAILABLE` (new)      |
| `AGE`                    | `UP-TO-DATE` (renamed) |
| `VERSION`                | `AGE`                  |
|                          | `VERSION`              |

(*) visible only when using `kubectl get -o wide`

### Changes to MachinePool resource

#### MachinePool Status

Following changes are implemented to MachinePool's status:

- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
- Update `ReadyReplicas` counter to use the same semantic Machine's `Ready` condition and add missing `UpToDateReplicas`.
- Align Machine pools replica counters to other CAPI resources
- Align to K8s API conventions by deprecating `Phase`
- Remove `FailureReason` and `FailureMessage` to get rid of the confusing concept of terminal failures
- Transition to new, improved, K8s API conventions aligned conditions

Below you can find the relevant fields in MachinePool Status v1beta2, after v1beta1 removal (end state);
Below the Go types, you can find a summary table that also shows how changes will be rolled out according to K8s deprecation rules.

```golang
type MachinePoolStatus struct {

    // The number of ready replicas for this MachinePool. A machine is considered ready when Machine's Ready condition is true.
    // +optional
    ReadyReplicas int32 `json:"readyReplicas"`

    // The number of available replicas for this MachinePool. A machine is considered available when Machine's Available condition is true.
    // +optional
    AvailableReplicas int32 `json:"availableReplicas"`

    // The number of up-to-date replicas targeted by this MachinePool.
    // +optional
    UpToDateReplicas int32 `json:"upToDateReplicas"`

    // Initialization provides observations of the MachinePool initialization process.
    // NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial MachinePool provisioning.
    // The value of those fields is never updated after provisioning is completed.
    // Use conditions to monitor the operational state of the MachinePool.
    // +optional
    Initialization *MachineInitializationStatus `json:"initialization,omitempty"`
    
    // Conditions represent the observations of a MachinePool's current state.
    // +optional
    // +listType=map
    // +listMapKey=type
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    
    // Other fields...
    // NOTE: `Phase`, `FailureReason`, `FailureMessage`, `BootstrapReady`, `InfrastructureReady` fields won't be there anymore
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

| v1beta1 (current)                    | v1beta2 (tentative Q1 2025)                                 | v1beta2 after v1beta1 removal (tentative Q1 2026) |
|--------------------------------------|-------------------------------------------------------------|---------------------------------------------------|
|                                      | `Initialization` (new)                                      | `Initialization`                                  |
| `BootstrapReady`                     | `Initialization.BootstrapDataSecretCreated` (renamed)       | `Initialization.BootstrapDataSecretCreated`       |
| `InfrastructureReady`                | `Initialization.InfrastructureProvisioned` (renamed)        | `Initialization.InfrastructureProvisioned`        |
| `UpdatedReplicas` (new)              | `UpToDateReplicas`                                          | `UpToDateReplicas`                                |
| `ExprimentalReadyReplicas` (new)     | `ReadyReplicas` (renamed)                                   | `ReadyReplicas`                                   |
| `ExprimentalAvailableReplicas` (new) | `AvailableReplicas` (renamed)                               | `AvailableReplicas`                               |
|                                      | `BackCompatibilty` (new)                                    | (removed)                                         |
| `ReadyReplicas` (deprecated)         | `BackCompatibilty.ReadyReplicas` (renamed) (deprecated)     | (removed)                                         |
| `AvailableReplicas` (deprecated)     | `BackCompatibilty.AvailableReplicas` (renamed) (deprecated) | (removed)                                         |
| `Phase` (deprecated)                 | `BackCompatibilty.Phase` (renamed) (deprecated)             | (removed)                                         |
| `FailureReason` (deprecated)         | `BackCompatibilty.FailureReason` (renamed) (deprecated)     | (removed)                                         |
| `FailureMessage` (deprecated)        | `BackCompatibilty.FailureMessage` (renamed) (deprecated)    | (removed)                                         |
| `Conditions`                         | `BackCompatibilty.Conditions` (renamed) (deprecated)        | (removed)                                         |
| `ExperimentalConditions` (new)       | `Conditions` (renamed)                                      | `Conditions`                                      |
| other fields...                      | other fields...                                             | other fields...                                   |

Notes:
- The `BackCompatibilty` struct is going to exist in v1beta2 types only until v1beta1 removal (9 months or 3 minor releases after v1beta2 is released/v1beta1 is deprecated, whichever is longer).
  Fields in this struct are used for supporting down conversions, thus providing users relying on v1beta1 APIs additional buffer time to pick up the new changes.

##### MachinePool (New)Conditions

| Condition              | Note                                                                                                              |
|------------------------|-------------------------------------------------------------------------------------------------------------------|
| `Available`            | True when `InfrastructureReady` and available replicas >= desired replicas (see notes below)                      |
| `BootstrapConfigReady` | Mirrors the corresponding condition from the MachinePool's BootstrapConfig resource                               |
| `InfrastructureReady`  | Mirrors the corresponding condition from the MachinePool's Infrastructure resource                                |
| `ReplicaFailure`       | This condition surfaces issues on creating a Machines replica in Kubernetes, if any. e.g. due to resource quotas. |
| `MachinesReady`        | This condition surfaces detail of issues on the controlled machines, if any.                                      |
| `ScalingUp`            | True if available replicas < desired replicas                                                                     |
| `ScalingDown`          | True if replicas > desired replicas                                                                               |
| `UpToDate`             | True if all the Machines controlled by this MachinePool are up to date (replicas = upToDate replicas)             |
| `Remediating`          | True if there is at least one machine controlled by this MachinePool is not passing health checks                 |
| `Deleted`              | True if MachinePool is deleted; Reason can be used to observe the cleanup progress when the resource is deleted   |
| `Paused`               | True if this MachinePool or the Cluster it belongs to are paused                                                  |

> To better evaluate proposed changes, below you can find the list of current MachinePool's conditions:
> Ready, BootstrapReady, InfrastructureReady, ReplicasReady.

Notes:
- Conditions like `ScalingUp`, `ScalingDown`, `Remediating` are intended to provide visibility on the corresponding lifecycle operation.
  e.g. If the scaling up operation is being blocked by a machine having issues while deleting, this should surface with a reason/message in
  the `ScalingDown` condition.
- As of today MachinePool does not have a notion similar to MachineDeployment's MaxUnavailability.

#### MachinePool Print columns

| Current           | To be                  |
|-------------------|------------------------|
| `NAME`            | `NAME`                 |
| `CLUSTER`         | `CLUSTER`              |
| `DESIRED` (*)     | `PAUSED` (new) (*)     |
| `REPLICAS`        | `DESIRED`              |
| `PHASE` (deleted) | `CURRENT` (*)          |
| `AGE`             | `READY`                |
| `VERSION`         | `AVAILABLE` (new)      |
|                   | `UP-TO-DATE` (renamed) |
|                   | `AGE`                  |
|                   | `VERSION`              |

(*) visible only when using `kubectl get -o wide`

Notes:
- Print columns are not subject to any deprecation rule, so it will be possible to iteratively improve them without waiting for the next API version.
- During the implementation we are going to verify if the resulting layout and eventually make final adjustments to the column list.

### Changes to Cluster API contract

The Cluster API contract defines a set of rules a provider is expected to comply with in order to interact with Cluster API.

When the v1beta2 API will be released (tentative Q1 2025), also the Cluster API contract will be bumped to v1beta2.

As defined at the beginning of this document, this proposal is not going to change how the Cluster API contract
with infrastructure, bootstrap and control providers currently works (by using status fields).

Similarly, this proposal is not going to change the fact that the Cluster API contract do not require providers to implement
conditions, even if this is recommended because conditions greatly improve user's experience.

However, this proposal is introducing a few changes into the v1beta2 version of the Cluster API contract in order to:
- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
- Remove `failureReason` and `failureMessage`.

What is worth to notice is that for the first time in the history of the project, this proposal is introducing
a mechanism that allows providers to adapt to new contract incrementally, more specifically:

- Providers won't be required to synchronize their changes to adapt to the Cluster API v1beta2 contract with the
  Cluster API's v1beta2 release.

- Each provider can implement changes described in the following paragraphs at its own pace, but the transition
  _must be completed_ before v1beta1 removal (tentative Q1 2026).

- Starting from the CAPI release when v1beta1 removal will happen (tentative Q1 2026), providers which are implementing
  the v1beta1 contract will stop to work (they will work only with older versions of Cluster API).

Additionally:

- Providers implementing conditions won't be required to do the transition from custom Cluster API custom Condition type
  to Kubernetes metav1.Conditions type (but this transition is recommended because it improves the consistency of each provider
  with Kubernetes, Cluster API, the ecosystem).

- However, providers choosing to keep using Cluster API custom conditions should be aware that starting from the
  CAPI release when v1beta1 removal will happen (tentative Q1 2026), the Cluster API project will remove the
  cluster API condition type, the `util\conditions` package, the code handling conditions in `util\patch.Helper`,
  everything related to custom cluster API condition type.
  (in other words, Cluster API custom condition must be replaced by provider's own custom conditions).

#### Contract for infrastructure providers

Note: given that the contract only defines expected names for fields in a resources at yaml/json level, we are
using those in this paragraph (instead of golang field names).

##### InfrastructureCluster

Following changes are planned for the contract for the InfrastructureCluster resource:

- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
  - Rename `status.ready` into `status.initialization.provisioned`.
- Remove `failureReason` and `failureMessage`.

| v1beta1 (current)                                                     | v1beta2 (tentative Q1 2025)                                                                                      | v1beta2 after v1beta1 removal (tentative Q1 2026)                                          |
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

| v1beta1 (current)                                                     | v1beta2 (tentative Q1 2025)                                                                                      | v1beta2 after v1beta1 removal (tentative Q1 2026)                                          |
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
  (both during initial InfrastructureCluster provisioning and after the initial provisioning is completed).

#### Contract for bootstrap providers

Following changes are planned for the contract for the BootstrapConfig resource:

- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
  - Rename `status.ready` into `status.initialization.dataSecretCreated`.
- Remove `failureReason` and `failureMessage`.

| v1beta1 (current)                                                     | v1beta2 (tentative Q1 2025)                                                                                                   | v1beta2 after v1beta1 removal (tentative Q1 2026)                                                    |
|-----------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------|
| `status.ready`, required                                              | `status.ready` (deprecated), one of `status.ready` or `status.initialization.dataSecretCreated`, required                     | (removed)                                                                                            |
|                                                                       | `status.initialization.dataSecretCreated` (new), one of `status.ready` or `status.initialization.dataSecretCreated`, required | `status.initialization.dataSecretCreated`, required                                                  |
| `status.conditions[Ready]`, optional with fall back on `status.ready` | `status.conditions[Ready]`, optional with fall back on `status.ready` or `status.initialization.dataSecretCreated` set        | `status.conditions[Ready]`, optional with fall back on `status.initialization.DataSecretCreated` set |
| `status.failureReason`, optional                                      | `status.failureReason` (deprecated), optional                                                                                 | (removed)                                                                                            |
| `status.failureMessage`, optional                                     | `status.failureMessage` (deprecated), optional                                                                                | (removed)                                                                                            |
| other fields/rules...                                                 | other fields/rules...                                                                                                         |                                                                                                      |

Notes:
- BootstrapConfig's `status.initialization.dataSecretCreated` will surface into Machine's `status.initialization.BootstrapDataSecretCreated` field.
- BootstrapConfig's `status.initialization.dataSecretCreated` must signal the completion of the initial provisioning of the bootstrap data secret.
  The value of this field should never be updated after provisioning is completed, and Cluster API will ignore any changes to it.
- BootstrapConfig's `status.conditions[Ready]` will surface into Machine's `status.conditions[BootstrapConfigReady]` condition.
- BootstrapConfig's `status.conditions[Ready]` must surface issues during the entire lifecycle of the BootstrapConfig
  (both during initial InfrastructureCluster provisioning and after the initial provisioning is completed).

#### Contract for control plane Providers

Following changes are planned for the contract for the ControlPlane resource:

- Disambiguate the usage of the ready term by renaming fields used for the initial provisioning workflow
  - Remove `status.ready` (`status.ready` is a redundant signal of the control plane being initialized).
  - Rename `status.initialized` into `status.initialization.controlPlaneInitialized`.
- Remove `failureReason` and `failureMessage`.

| v1beta1 (current)                                                     | v1beta2 (tentative Q1 2025)                                                                                                                                          | v1beta2 after v1beta1 removal (tentative Q1 2026)                                                           |
|-----------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------|
| `status.ready`, required                                              | `status.ready` (deprecated), one of `status.ready` or `status.initialization.controlPlaneInitialized` required                                                       | (removed)                                                                                                   |
| `status.initialized`, required                                        | `status.initialization.controlPlaneInitialized` (renamed), one of `status.ready` or `status.initialization.controlPlaneInitialized` required                         | `status.initialization.controlPlaneInitialized`, required                                                   |
| `status.conditions[Ready]`, optional with fall back on `status.ready` | `status.backCompatibilty.conditions[Ready]` (renamed, deprecated), optional with fall back on `status.ready` or `status.Initializiation.ControlPlaneInitialized` set | (removed)                                                                                                   |
|                                                                       | `status.conditions[Available]` (new), optional with fall back optional with fall back on `status.ready` or `status.Initializiation.ControlPlaneInitialized` set      | `status.conditions[Available]`, optional with fall back on `status.initializiation.controlPlaneInitialized` |
| `status.failureReason`, optional                                      | `status.failureReason` (deprecated), optional                                                                                                                        | (removed)                                                                                                   |
| `status.failureMessage`, optional                                     | `status.failureMessage` (deprecated), optional                                                                                                                       | (removed)                                                                                                   |
| other fields/rules...                                                 | other fields/rules...                                                                                                                                                |                                                                                                             |

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

## [WIP] Example use cases
NOTE: Let me know if you want to add more use cases. I will try to collect more too and add a brief explanation about how
each use case can be addressed with the improved status in CAPI resources 

As a cluster admin with MachineDeployment ownership I'd like to understand if my MD is performing a rolling upgrade and why by looking at the MD status/conditions
As a cluster admin with MachineDeployment ownership I'd like to understand why my MD rollout is blocked and why by looking at the MD status/conditions
As a cluster admin with MachineDeployment ownership I'd like to understand why Machines are failing to be available by looking at the MD status/conditions
As a cluster admin with MachineDeployment ownership I'd like to understand why Machines are stuck on deletion looking at the MD status/conditions
