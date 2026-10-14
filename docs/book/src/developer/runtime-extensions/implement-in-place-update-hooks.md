# Implementing In-Place Update Hooks

> [!IMPORTANT]
> Both the `RuntimeSDK` and the `InPlaceUpdates` feature gates must be set to true in order to use this feature.
> See [Feature Gates](../../reference/feature-gates.md) for more details.

> [!CAUTION]
> Please note Runtime SDK is an advanced feature. If implemented incorrectly, a failing Runtime Extension can severely impact the Cluster API runtime.

> [!CAUTION]
> For Clusters that have been created with CAPI <= v1.11 it will only be possible to unset fields during in-place updates after:
> * a regular rollout that replaces all Machines
> * an in-place update that updates all Machines
>
> For details see the PR description of [CAPI-12890](https://github.com/kubernetes-sigs/cluster-api/pull/12890) section "Migration from managedFields v1.11 => v1.12".

## Introduction

The proposal for [in-place updates in Cluster API](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240807-in-place-updates.md)
introduced extensions allowing users to execute changes on existing machines without deleting the Machine and creating a new one.

Notably, the Cluster API user experience remains the same as of today no matter of the in-place update feature is enabled 
or not e.g. in order to trigger a MachineDeployment rollout, you have to rotate a template, etc.

Users should care ONLY about the desired state (as of today).

Cluster API is responsible to choose the best strategy to achieve desired state, and with the introduction of 
update extensions, Cluster API is expanding the set of tools that can be used to achieve the desired state.

If external update extensions can not cover the totality of the desired changes, CAPI will fall back to Cluster API’s default, 
replace machines (immutable rollouts).

Cluster API will be also responsible to determine which Machine/MachineSet should be updated, as well as to handle rollout
options like MaxSurge/MaxUnavailable. With this regard:

- By default, in-place updates are considered as an operation that might affect machine availability, 
  because in-place updates are always considered as potentially disruptive.
  - For control plane machines, if maxSurge is 1, a new machine must be created first, then as soon as there is 
    “buffer” for in-place, in-place update can proceed.
    - KCP will not use in-place in case it will detect that it can impact health of the control plane.
  - For workers machines, if maxUnavailable is 0, a new machine must be created first, then as soon as there
    is “buffer” for in-place, in-place update can proceed.
    - When in-place is possible, the system should try to in-place update as many machines as possible.
      In practice, this means that maxSurge might not be fully used (it is used only for scale up by one if maxUnavailable=0).
- The `CanUpdateMachineResponse` or the `CanUpdateMachineSetResponse` hook can override the default behavior and specify 
  that the required change does not affect availability. In this case the in-place update will start immediately, without
  creating an additional machine/using maxSurge (if available)
  - **Warning!** the system is going to perform in-place update on the first Machine without the safety net provided by the additional machine.
  - If something goes wrong unexpectedly, and the operation leads to actual unavailability, control plane components or workloads could be impacted.
  - **Use this feature with caution!**
- The `CanUpdateMachineResponse` or the `CanUpdateMachineSetResponse` hook are also to express a binding opinion on which operations 
  can be used to roll out the changes for the Machine or for Machines controlled by the MachineSet.
  - This opinion can be used to block the rollout under temporary circumstances: e.g. preflight checks fail.
  - Please note that some limitations apply, see documentation below.  
- No in-place updates are performed for workers machines when using rollout strategy `OnDelete`.

> [!IMPORTANT]
> Cluster API will call the in-place extensions only if the `InPlaceUpdates` feature flag is enabled.
>
> Also, please note that the current implementation of the [in-place updates proposal](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240807-in-place-updates.md) only allows registering one extension for the `CanUpdateMachine`, `CanUpdateMachineSet` and `UpdateMachine` hooks.

## Guidelines

All guidelines defined in [Implementing Runtime Extensions](implement-extensions.md#guidelines) apply to the
implementation of Runtime Extensions for in-place update hooks as well.

In summary, Runtime Extensions are components that should be designed, written and deployed with great caution given
that they can affect the proper functioning of the Cluster API runtime. A poorly implemented Runtime Extension could
potentially block updates.

Following recommendations are especially relevant:

* [Timeouts](implement-extensions.md#timeouts)
* [Idempotence](implement-extensions.md#idempotence)
* [Deterministic result](implement-extensions.md#deterministic-result)
* [Error messages](implement-extensions.md#error-messages)
* [Error management](implement-extensions.md#error-management)
* [Avoid dependencies](implement-extensions.md#avoid-dependencies)

## Definitions

For additional details about the OpenAPI spec of the upgrade plan hooks, please download the [`runtime-sdk-openapi.yaml`]({{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api" gomodule:"sigs.k8s.io/cluster-api" asset:"runtime-sdk-openapi.yaml" version:"1.12.x"}})
file and then open it from the [Swagger UI](https://editor.swagger.io/).

### CanUpdateMachine

> [!IMPORTANT] The can CanUpdateMachine hook MUST be implemented by control plane providers that
> are using Cluster API Machines to represent replicas and supporting in-place updates for those replicas.
> 
> The documentation in this paragraph focuses on the reference implementation in the KubeadmControlPlane (KCP) provider, 
> but the behavior of the CanUpdateMachine hook MUST be the same also when implemented by other control plane providers.

This hook is called by KCP when performing the "can update in-place" for a control plane machine.

Example request:

```yaml
apiVersion: hooks.runtime.cluster.x-k8s.io/v1alpha1
kind: CanUpdateMachineRequest
settings: <Runtime Extension settings>
current:
  machine:
    apiVersion: cluster.x-k8s.io/v1beta2
    kind: Machine
    metadata:
      name: test-cluster
      namespace: test-ns
    spec:
      ...
  infrastructureMachine:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: VSphereMachine
    metadata:
      name: test-cluster
      namespace: test-ns
    spec:
      ...
  boostrapConfig:
    apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
    kind: KubeadmConfig
    metadata:
      name: test-cluster
      namespace: test-ns
    spec:
      ...
desired:
  machine:
    ...
  infrastructureMachine:
    ...
  boostrapConfig:
    ...
```

Note:
- All the objects will have the latest API version known by Cluster API.
- Only spec is provided, status fields are not included
- In a future release, when registering more than one extension for the `CanUpdateMachine` will be supported, the current state will already include changes that can be handled in-place by other runtime extensions.

Example Response:

```yaml
apiVersion: hooks.runtime.cluster.x-k8s.io/v1alpha1
kind: CanUpdateMachineResponse
status: Success # or Failure
message: "error message if status == Failure"
retryAfterSeconds: 15 # Set retryAfterSeconds > 0 if a decision cannot be made yet
machinePatch:
  patchType: JSONPatch
  patch: <JSON-patch>
infrastructureMachinePatch:
  ...
boostrapConfigPatch:
  ...
affectsAvailability: true
allowedOperations: [UpdateInPlace, Replace] # optional, if missing all operations are allowed
```

Note: 
- Extensions should return per-object patches to be applied on current objects to indicate which changes they can handle in-place.
- Only fields in Machine/InfraMachine/BootstrapConfig spec have to be covered by patches
- Patches must be in JSONPatch or JSONMergePatch format
- If `retryAfterSeconds` is set to a non-zero value, the extension signals that it is not yet able to decide if the change can be handled in-place.
  The other fields of the response are ignored, no fallback to machine replace is performed and the hook will be called again after the given amount of seconds.
- `affectsAvailability` is an optional value, if missing it defaults to true.
- When `CanUpdateMachineResponse` hook specifies that the required change does not affect availability, the in-place update will start immediately, without creating an additional machine/using maxSurge (if available)
  - **Warning!** the system is going to perform in-place update on the first Machine without the safety net provided by the additional machine.
  - If something goes wrong unexpectedly, and the operation leads to actual unavailability, control plane components could be impacted.
  - **Use this feature with caution!**
- If using KCP, KCP might override the `CanUpdateMachineResponse` and decide to scale up or down when this will be considered safer than performing in place to preserve the overall control plane health (e.g. when
  there are unhealthy machines).
- `allowedOperations` is an optional value that allows the extension to express a binding opinion on which operations can be used to roll out the changes to the Machine:
  - `UpdateInPlace`: the Machine can be updated in-place.
  - `Replace`: the Machine can be replaced (i.e. a new Machine is created and the old one is deleted).
  - If `allowedOperations` is not set, the extension has no opinion and all operations are allowed (this is the default).
  - If `allowedOperations` is set to an empty list (`allowedOperations: []`), no operation is allowed and the rollout of the Machine is blocked.
    **Note**: an empty list and a missing field have a different meaning. When implementing an extension in Go using the types in `sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1`
    an empty list must be a non-nil empty slice, e.g. `new([]runtimehooksv1.MachineUpdateOperation{})`.
  - `allowedOperations` is independent of the patches returned by the extension: patches tell which changes can be handled in-place, `allowedOperations` tells which operations are allowed.
    A runtime extension can e.g. block a rollout when a preflight check fails, even if it would be able to handle the changes in-place.
    A runtime extension can e.g. also cover the entire diff but return `allowedOperations: [Replace]`, then KCP would replace the Machine.
  - When the update is blocked the extension is called again every 5 minutes, so it is possible to unblock the rollout by changing the answer.
    The answer is also evaluated again when the desired state of KCP is changed.

### CanUpdateMachineSet

This hook is called by the MachineDeployment controller when performing the "can update in-place" for all the Machines controlled by
a MachineSet.

Example request:

```yaml
apiVersion: hooks.runtime.cluster.x-k8s.io/v1alpha1
kind: CanUpdateMachineSetRequest
settings: <Runtime Extension settings>
current:
  machineSet:
    apiVersion: cluster.x-k8s.io/v1beta2
    kind: MachineSet
    metadata:
      name: test-cluster
      namespace: test-ns
    spec:
      ...
  infrastructureMachineTemplate:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: VSphereMachineTemplate
    metadata:
      name: test-cluster
      namespace: test-ns
    spec:
      ...
  boostrapConfigTemplate:
    apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
    kind: KubeadmConfigTemplate
    metadata:
      name: test-cluster
      namespace: test-ns
    spec:
      ...
desired:
  machineSet:
    ...
  infrastructureMachineTemplate:
    ...
  boostrapConfigTemplate:
    ...
```

Note:
- All the objects will have the latest API version known by Cluster API.
- Only spec is provided, status fields are not included
- In a future release, when registering more than one extension for the `CanUpdateMachineSet` will be supported, the current state will already include changes that can be handled in-place by other runtime extensions.

Example Response:

```yaml
apiVersion: hooks.runtime.cluster.x-k8s.io/v1alpha1
kind: CanUpdateMachineSetResponse
status: Success # or Failure
message: 15 # Set retryAfterSeconds > 0 if a decision cannot be made yet
retryAfterSeconds: 0
machineSetPatch:
  patchType: JSONPatch
  patch: <JSON-patch>
infrastructureMachineTemplatePatch:
  ...
boostrapConfigTemplatePatch:
  ...
affectsAvailability: true
allowedOperations: [UpdateInPlace, Replace] # optional, if missing all operations are allowed
```

Note:
- Extensions should return per-object patches to be applied on current objects to indicate which changes they can handle in-place.
- Only fields in MachineSet/InfraMachineTemplate/BootstrapConfigTemplate spec.template.spec have to be covered by patches
- Patches must be in JSONPatch or JSONMergePatch format
- If `retryAfterSeconds` is set to a non-zero value, the extension signals that it is not yet able to decide if the change can be handled in-place.
  The other fields of the response are ignored, no fallback to machine replace is performed and the hook will be called again after the given amount of seconds.
  - While the decision is still in progress for at least one MachineSet, the rollout is paused for the entire MachineDeployment: 
    - Machines are not scaled down (or moved) from old MachineSet and no additional Machines are created on the new MachineSet because of `maxSurge`. 
    - Scale up/down due to changes to the MachineDeployment `spec.replicas` still happen.
- `affectsAvailability` is an optional value, if missing it defaults to true.
- When `CanUpdateMachineSetResponse` specifies that the required change does not affect availability, the in-place update will start immediately, without creating an additional machine/using maxSurge (if available)
  - **Warning!** the system is going to perform in-place update on the first Machine without the safety net provided by the additional machine.
  - If something goes wrong unexpectedly, and the operation leads to actual unavailability, workloads could be impacted.
  - **Use this feature with caution!**
- `allowedOperations` is an optional value that allows the extension to express a binding opinion on which operations can be used to roll out the changes for the Machines controlled by the MachineSet:
  - `UpdateInPlace`: the controlled Machines can be updated in-place.
  - `Replace`: the controlled Machines can be replaced (i.e. a new Machine is created and the old one is deleted).
  - If `allowedOperations` is not set, the extension has no opinion and all operations are allowed (this is the default).
  - If `allowedOperations` is set to an empty list (`allowedOperations: []`), no operation is allowed and the rollout for the Machines controlled by the MachineSet is blocked.
    **Note**: an empty list and a missing field have a different meaning. When implementing an extension in Go using the types in `sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1`
    an empty list must be a non-nil empty slice, e.g. `new([]runtimehooksv1.MachineUpdateOperation{})`.
  - `allowedOperations` is independent of the patches returned by the extension: patches tell which changes can be handled in-place, `allowedOperations` tells which operations are allowed.
    A runtime extension can e.g. block a rollout for the Machines controlled by the MachineSet when a preflight check fails, even if it would be able to handle the changes in-place.
    A runtime extension can e.g. also cover the entire diff but return `allowedOperations: [Replace]`, then KCP would replace the Machine.
- When the update is blocked the extension is called again every 5 minutes, so it is possible to unblock the rollout by changing the answer.
  The answer is also evaluated again when the desired state of KCP is changed.
- The new MachineSet has no `CanUpdateMachineSet` call, and the usage of `maxSurge` is allowed or blocked according to the following rules:
  - If at least one old MachineSet is updated in-place, the usage of `maxSurge` is considered part of the in-place update operation.
    - If the `UpdateInPlace` is blocked by all the old MachineSets, usage of `maxSurge` is blocked as well.
  - Otherwise, if at least one old MachineSet is rolled out (including fallbacks from in-place update), usage of `maxSurge` is considered part of the replace operation. 
    - If the `Replace` operation is blocked by all the old MachineSets, usage of `maxSurge` is blocked as well.
  - If instead all the old MachineSets are blocked, usage of `maxSurge` is blocked as well.
  
### UpdateMachine

This hook is called by the Machine controller when performing the in-place updates for a Machine.

Example request:

```yaml
apiVersion: hooks.runtime.cluster.x-k8s.io/v1alpha1
kind: UpdateMachineRequest
settings: <Runtime Extension settings>
desired:
  machine:
    apiVersion: cluster.x-k8s.io/v1beta2
    kind: Machine
    metadata:
      name: test-cluster
      namespace: test-ns
    spec:
      ...
  infrastructureMachine:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: VSphereMachine
    metadata:
      name: test-cluster
      namespace: test-ns
    spec:
      ...
  boostrapConfig:
    apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
    kind: KubeadmConfig
    metadata:
      name: test-cluster
      namespace: test-ns
    spec:
      ...
```

Note:
- Only desired is provided (the external updater extension should know current state of the Machine).
- Only spec is provided, status fields are not included

Example Response:

```yaml
apiVersion: hooks.runtime.cluster.x-k8s.io/v1alpha1
kind: UpdateMachineResponse
status: Success # or Failure
message: "error message if status == Failure"
retryAfterSeconds: 10
```

Note:
- The status of the update operation is determined by the CommonRetryResponse fields:
  - Status=Success + RetryAfterSeconds > 0: update is in progress
  - Status=Success + RetryAfterSeconds = 0: update completed successfully
  - Status=Failure: update failed
