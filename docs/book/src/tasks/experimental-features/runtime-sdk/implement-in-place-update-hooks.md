# Implementing in-place update hooks

<aside class="note warning">

<h1>Caution</h1>

Please note Runtime SDK is an advanced feature. If implemented incorrectly, a failing Runtime Extension can severely impact the Cluster API runtime.

</aside>

## Introduction

The proposal for [n-place updates in Cluster API](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/20240807-in-place-updates.md)
introduced extensions allowing users to execute changes on existing machines without deleting the machines and creating a new one.

Notably, the Cluster API user experience remain the same as of today no matter of the in-place update feature is enabled 
or not e.g. in order to trigger a MachineDeployment rollout, you have to rotate a template, etc.

Users should care ONLY about the desired state (as of today).

Cluster API is responsible to choose the best strategy to achieve desired state, and with the introduction of 
update extensions, Cluster API is expanding the set of tools Cluster API can use to achieve the desired state.

If external update extensions can not cover the totality of the desired changes, CAPI will fall back to Cluster API’s default, 
immutable rollouts.

Cluster API will be also responsible to determine which Machine/MachineSet should be updated, as well as to handle rollout
options like MaxSurge/MaxUnavailable. With this regard:

- Machines updating in-place are considered not available, because in-place updates are always considered as potentially disruptive.
  - For control plane machines, if maxSurge is one, a new machine must be created first, then as soon as there is 
    “buffer” for in-place, in-place update can proceed.
    - KCP will not use in-place in case it will detect that it can impact health of the control plane.
  - For workers machines, if maxUnavailable is zero, a new machine must be created first, then as soon as there
    is “buffer” for in-place, in-place update can proceed.
    - When in-place is possible, the system should try to in-place update as many machines as possible.
      In practice, this means that maxSurge might be not fully used (it is used only for scale up by one if maxUnavailable=0).
  - No in-place updates are performed for workers machines when using rollout strategy on delete.

<!-- TOC -->
* [Implementing in-place update hooks](#implementing-in-place-update-hooks)
  * [Introduction](#introduction)
  * [Guidelines](#guidelines)
  * [Definitions](#definitions)
    * [CanUpdateMachine](#canupdatemachine)
    * [CanUpdateMachineSet](#canupdatemachineset)
    * [UpdateMachine](#updatemachine)
<!-- TOC -->

## Guidelines

All guidelines defined in [Implementing Runtime Extensions](implement-extensions.md#guidelines) apply to the
implementation of Runtime Extensions for upgrade plan hooks as well.

In summary, Runtime Extensions are components that should be designed, written and deployed with great caution given
that they can affect the proper functioning of the Cluster API runtime. A poorly implemented Runtime Extension could
potentially block upgrade transitions from happening.

Following recommendations are especially relevant:

* [Timeouts](implement-extensions.md#timeouts)
* [Idempotence](implement-extensions.md#idempotence)
* [Deterministic result](implement-extensions.md#deterministic-result)
* [Error messages](implement-extensions.md#error-messages)
* [Error management](implement-extensions.md#error-management)
* [Avoid dependencies](implement-extensions.md#avoid-dependencies)

## Definitions

For additional details about the OpenAPI spec of the upgrade plan hooks, please download the [`runtime-sdk-openapi.yaml`]({{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api" gomodule:"sigs.k8s.io/cluster-api" asset:"runtime-sdk-openapi.yaml" version:"1.11.x"}})
file and then open it from the [Swagger UI](https://editor.swagger.io/).

### CanUpdateMachine

This hook is called by KCP when performing the "can update in-place" for a control plane machine.

Example request

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
- When more than one extension will be supported, the current state will already include changes that can handle in-place by other runtime extensions.

Example Response

```yaml
apiVersion: hooks.runtime.cluster.x-k8s.io/v1alpha1
kind: CanUpdateMachineResponse
status: Success # or Failure
message: "error message if status == Failure"
machinePatch:
  patchType: JSONPatch
  patch: <JSON-patch>
infrastructureMachinePatch:
  ...
boostrapConfigPatch:
  ...
```

Note: 
- Extensions should return per-object patches to be applied on current objects to indicate which changes they can handle in-place.
- Only fields in Machine/InfraMachine/BootstrapConfig spec have to be covered by patches
- Patches must be in JSONPatch or JSONMergePatch format

### CanUpdateMachineSet

This hook is called by the MachineDeployment controller when performing the "can update in-place" for all the Machines controlled by
a MachineSet.

Example request

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
- When more than one extension will be supported, the current state will already include changes that can handle in-place by other runtime extensions.

Example Response

```yaml
apiVersion: hooks.runtime.cluster.x-k8s.io/v1alpha1
kind: CanUpdateMachineSetResponse
status: Success # or Failure
message: "error message if status == Failure"
machineSetPatch:
  patchType: JSONPatch
  patch: <JSON-patch>
infrastructureMachineTemplatePatch:
  ...
boostrapConfigTemplatePatch:
  ...
```

Note:
- Extensions should return per-object patches to be applied on current objects to indicate which changes they can handle in-place.
- Only fields in Machine/InfraMachine/BootstrapConfig spec have to be covered by patches
- Patches must be in JSONPatch or JSONMergePatch format

### UpdateMachine

This hook is called by the Machine controller when performing the in-place updates for a Machine.

Example request

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
```

Note:
- Only desired is provided (the external updater extension should know current state of the Machine).
- Only spec is provided, status fields are not included

Example Response

```yaml
apiVersion: hooks.runtime.cluster.x-k8s.io/v1alpha1
kind: UpdateMachineSetResponse
status: Success # or Failure
message: "error message if status == Failure"
retryAfterSeconds: 10
```

Note:
- The status of the update operation is determined by the CommonRetryResponse fields:
  - Status=Success + RetryAfterSeconds > 0: update is in progress
  - Status=Success + RetryAfterSeconds = 0: update completed successfully
  - Status=Failure: update failed
