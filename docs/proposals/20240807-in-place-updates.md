---
title: In-place updates in Cluster API
authors:
  - "@dharmjit"
  - "@g-gaston"
  - "@furkatgofurov7"
  - "@alexander-demicev"
  - "@@mogliang"
  - "@sbueringer"
  - "@fabriziopandini"
reviewers:
  - TBD
creation-date: "2024-08-07"
last-updated: "2024-08-07"
status: experimental
---

# In-place updates in Cluster API

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Glossary](#glossary)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Divide and conquer](#divide-and-conquer)
  - [Tenets](#tenets)
    - [Same UX](#same-ux)
    - [Fallback to Immutable rollouts](#fallback-to-immutable-rollouts)
    - [Clean separation of concern](#clean-separation-of-concern)
  - [Goals](#goals)
  - [Non-Goals/Future work](#non-goalsfuture-work)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
    - [Story 1](#story-1)
    - [Story 2](#story-2)
    - [Story 3](#story-3)
    - [Story 4](#story-4)
    - [Story 5](#story-5)
    - [Story 6](#story-6)
  - [High level flow](#high-level-flow)
  - [Deciding the update strategy](#deciding-the-update-strategy)
  - [MachineDeployment updates](#machinedeployment-updates)
  - [KCP updates](#kcp-updates)
  - [Machine updates](#machine-updates)
  - [Infra Machine Template changes](#infra-machine-template-changes)
  - [Remediation](#remediation)
  - [Examples](#examples)
    - [KCP kubernetes version update](#kcp-kubernetes-version-update)
    - [Update worker node memory](#update-worker-node-memory)
    - [Update worker nodes OS from Linux to Windows](#update-worker-nodes-os-from-linux-to-windows)
  - [Security Model](#security-model)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Additional Details](#additional-details)
  - [Test Plan](#test-plan)
  - [Graduation Criteria](#graduation-criteria)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

__In-place Update__: any change to a Machine spec, including the Kubernetes Version, that is performed without deleting the machines and creating a new one.

__Update Lifecycle Hook__: CAPI Lifecycle Runtime Hook to invoke external update extensions.

__Update Extension__: Runtime Extension (Implementation) is a component responsible to perform in place updates when  the `External Update Lifecycle Hook` is invoked.

## Summary

The proposal introduces update extensions allowing users to execute custom strategies when performing Cluster API rollouts.

An External Update Extension implementing custom update strategies will report the subset of changes they know how to perform. Cluster API will orchestrate the different extensions, polling the update progress from them.

If the totality of the required changes cannot be covered by the defined extensions, Cluster API will fall back to the current behavior (rolling update).

## Motivation

Cluster API by default performs rollouts by creating a new machine and deleting the old one.

This approach, inspired by the principle of immutable infrastructure (the very same used by Kubernetes to manage Pods), has a set of considerable advantages:
* It is simple to explain, it is predictable, consistent and easy to reason about with users and between engineers.
* It drastically reduces the number of variables to be considered when managing the lifecycle of machines hosting nodes (it prevents each machines to become a snow flake) 
* It is simple to implement, because it relies on two core primitives only, create and delete; additionally implementation does not depend on machine specific choice, like OS, bootstrap mechanism etc.
* It allows to implement and maintain a sustainable test matrix, which is key to each Cluster API release and for the long term sustainability for the Cluster API project.

Over time several improvement were made to Cluster API immutable rollouts:
* Support for delete first strategy, thus making it easier to do immutable rollouts on bare metal / environments with constrained resources.
* Support for [In place propagation of changes affecting Kubernetes objects only](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20221003-In-place-propagation-of-Kubernetes-objects-only-changes.md), thus avoiding unnecessary rollouts
* Support for [Taint nodes with PreferNoSchedule during rollouts](https://github.com/kubernetes-sigs/cluster-api/pull/10223), thus reducing Pod churn by optimizing how Pods are rescheduled during rollouts.

Even if the project continues to improve immutable rollouts, most probably there are and there will always be some remaining use cases where it is complex for users to perform immutable rollouts, or where users perceive immutable rollouts to be too disruptive to how they are used to manage machines in their organization:
* More efficient updates (multiple instances) that don't require re-bootstrap. Re-bootstrapping a bare metal machine takes ~10-15 mins on average. Speed matters when you have 100s - 1000s of nodes to upgrade. For a common telco RAN use case, users can have 30000-ish nodes. Depending on the parallelism, that could take days / weeks to upgrade because of the re-bootstrap time.
* Credentials rotation, e.g. rotating authorized keys for SSH.


With this proposal, Cluster API provides a new extensibility point for users willing to implement their own specific solution for these problems by implementing an Update extension.

With the implementation of an Update extension, users can take ownership of the rollout process and embrace in-place rollout strategies, intentionally trading off some of the benefits that you get from immutable infrastructure.

### Divide and conquer

Considering the complexity of this topic, a phased approach is required to design and implement the solution for in-place upgrades.

The main goal of the first iteration of this proposal is to make it possible for Cluster API users to start experimenting usage of in-place upgrades, so we can gather feedback and evolve to the next stage.

This iteration will focus on implementing the machinery required to interact with update extensions, while users facing changes in the API types are deferred to follow up iterations.

### Tenets

#### Same UX

Cluster API user experience MUST be the same when using default, immutable updates or when using external update extensions: e.g. in order to trigger a MachineDeployment rollout, you have to rotate a template, etc.

#### Fallback to Immutable rollouts

If external update extensions can not cover the totality of the desired changes, CAPI WILL defer to Cluster API’s default, immutable rollouts. This is important for a couple of reasons:

* It allows to implement custom rollout strategies incrementally, without the need to cover all use cases up-front.
* There are cases when replacing the machine will always be necessary:
    * When it is not possible to recover the machine, e.g. hardware failure.
    * When the user determines that recovering the machine is too complex/costly vs replacing it. 
    * Automatic machine remediation (unless you use external remediation strategies)

#### Clean separation of concern

It is the responsibility of the extension to decide if it can perform changes in-place and to perform these changes on a single machine. If the extension decides that it cannot perform changes in-place, CAPI will fall back to rollout.

The responsibility to determine which machine should be rolled out as well as the responsibility to handle rollout options like MaxSurge/MaxUnavailable will remain on the controllers owning the machine (e.g. KCP, MD controller). 

### Goals

- Enable the implementation of pluggable update extensions.
- Allow users to update Kubernetes clusters using pluggable External Update Extension.
- Support External Update Extensions for both Control Plane (KCP or others) and MachineDeployment controlled machines.

### Non-Goals/Future work

- To provide rollbacks in case of an in-place update failure. Failed updates need to be fixed manually by the user on the machine or by replacing the machine.
- Introduce any changes to KCP (or any other control plane provider), MachineDeployment, MachineSet, Machine APIs.
- Maintain a coherent user experience for both rolling and in-place updates.
- Allow in-place updates for single-node clusters without the requirement to reprovision hosts (future goal).

## Proposal

We propose to extend upgrade workflows to call External Update Extensions, if defined.

Initially, this feature will be implemented without making API changes in the current core Cluster API objects. It will follow Kubernetes' feature gate mechanism. All functionality related to In-Place Updates will be available only if the `InPlaceUpdates` feature flag is set to true. It is disabled unless explicitly configured.

This proposal introduces a Lifecycle Hook named `ExternalUpdate` for communication between CAPI and external update implementers. Multiple external updaters can be registered, each of them only covering a subset of machine changes. The CAPI controllers will ask the external updaters what kind of changes they can handle and, based on the reponse, compose and orchestrate them to achieve the desired state.

With the introduction of this experimental feature, users may want to apply the in-place updates workflow to a subset of CAPI clusters only. By leveraging CAPI's `RuntimeExtension`, we can provide a namespace selector via [`ExtensionConfig`](https://cluster-api.sigs.k8s.io/tasks/experimental-features/runtime-sdk/implement-extensions#extensionconfig). This allows us to support cluster selection at the namespace level (only clusters/machines namespaces that match the selector) without applying API changes.

### User Stories

#### Story 1

As a cluster operator, I want to perform in-place updates on my Kubernetes clusters without replacing the underlying machines. I expect the update process to be flexible, allowing me to customize the strategy based on my specific requirements, such as air-gapped environments or special node configurations.

#### Story 2

As a cluster operator, I want to seamlessly transition between rolling and in-place updates while maintaining a consistent user interface. I appreciate the option to choose or implement my own update strategy,  ensuring that the update process aligns with my organization's unique needs.

#### Story 3
As a cluster operator for resource constrained environments, I want to utilize CAPI pluggable external update mechanism to implement in-place updates without requiring additional compute capacity in a single node cluster.

#### Story 4
As a cluster operator for highly specialized/customized environments, I want to utilize CAPI pluggable external update mechanism to implement in-place updates without losing the existing VM/OS customizations.

#### Story 5
As a cluster operator, I want to update machine attributes supported by my infrastructure provider without the need to recreate the machine.

#### Story 6
As a cluster service provider, I want guidance/documentation on how to write external update extension for my own use case.

### High level flow

```mermaid
sequenceDiagram
    participant Operator

    box Management Cluster
        participant apiserver as kube-api server
        participant capi as CP/MD controller
        participant mach as Machine Controller
        participant hook as External updater
    end

    Operator->>+apiserver: Make changes to KCP
    apiserver->>+capi: Notify changes
    apiserver->>-Operator: OK
    loop For all External Updaters
        capi->>+hook: Can update?
        hook->>capi: Supported changes
    end
    capi->>capi: Decide Update Strategy
    loop For all Machines
        capi->>apiserver: Mark Machine as pending
        apiserver->>mach: Notify changes
        mach->>apiserver: Set UpToDate condition to False
        loop For all External Updaters
            mach->>hook: Run updater
        end
        mach->>apiserver: Mark Hooks in Machine as Done
        mach->>apiserver: Set UpToDate condition to True
    end
```

When configured, external updates will, roughly, follow these steps:
1. CP/MD Controller: detect an update is required.
2. CP/MD Controller: query defined update extensions, and based on the response decides if an update should happen in-place. If not, the update will be performed as of today (rollout).
3. CP/MD Controller: mark machines as pending to track that updaters should be called.
4. Machine Controller: set `UpToDate` condition on machines to `False`.
5. Machine Controller: invoke all registered updaters, sequentially, one by one.
6. Machine Controller: once updaters finish use `sigs.k8s.io/cluster-api/internal/hooks.MarkAsDone()` to mark machine as done updating.
7. Machine Controller: set `UpToDate` condition on machines to `True`.

The following sections dive deep into these steps, zooming in into the different component interactions and defining how the main error cases are handled.

### Deciding the update strategy

```mermaid
sequenceDiagram
    participant Operator

    box Management Cluster
        participant apiserver as kube-api server
        participant capi as CP/MD Controller
        participant hook as External updater 1
        participant hook2 as External updater 2
    end

    Operator->>+apiserver: make changes to CP/MD
    apiserver->>+capi: Notify changes
    apiserver->>-Operator: OK
    capi->>+hook: Can update?
    hook->>capi: Set of changes
    capi->>+hook2: Can update?
    hook2->>capi: Set of changes
    alt all changes covered?
        capi->>apiserver: Decide Update Strategy
    else
        alt fallback strategy?
            capi->>apiserver: Re-create machines
        else
            capi->>apiserver: Marked update as failed
        end
    end
```

Both `KCP` and `MachineDeployment` controllers follow a similar pattern around updates, they first detect if an update is required and then based on the configured strategy follow the appropiate update logic (note that today there is only one valid strategy, `RollingUpdate`).

With `InPlaceUpdates` feature gate enabled, CAPI controllers will compute the set of desired changes and iterate over the registered external updaters, requesting through the Runtime Hook the set of changes each updater can handle. The changes supported by an updater can be the complete set of desired changes, a subset of them or an empty set, signaling it cannot handle any of the desired changes.

If any combination of the updaters can handle the desired changes then CAPI will determine that the update can be performed using the external strategy. 

If any of the desired changes cannot be covered by the updaters capabilities, CAPI will determine the desired state cannot be reached through external updaters. In this case, it will fallback to the rolling update strategy, replacing machines as needed. 

### MachineDeployment updates

```mermaid
sequenceDiagram
participant Operator

box Management Cluster
    participant apiserver as kube-api server
    participant capi as MD controller
    participant msc as MachineSet Controller
    participant mach as Machine Controller
    participant hook as External updater
end

Operator->>apiserver: make changes to MD
apiserver->>capi: Notify changes
apiserver->>Operator: OK
capi->>capi: Decide Update Strategy
capi->>apiserver: Create new MachineSet
loop For all machines
    capi->>apiserver: Mark as pending, update spec, and move to new Machine Set
    apiserver->>mach: Notify changes
    mach->>apiserver: Set UpToDate condition to False
    loop For all updaters in plan
        mach->>hook: Run updater
    end
    mach->>apiserver: Mark Hooks in Machine as Done
    mach->>apiserver: Set UpToDate condition to True
end
```

### KCP updates

```mermaid
sequenceDiagram
box Management Cluster
    participant Operator
    participant apiserver as kube-api server
    participant capi as KCP controller
    participant mach as Machine Controller
    participant hook as External updater
end

Operator->>apiserver: make changes to KCP
apiserver->>capi: Notify changes
apiserver->>Operator: OK
capi->>capi: Decide Update Strategy
loop For all machines
    capi->>apiserver: Mark Machine as pending, update spec
    apiserver->>mach: Notify changes
    mach->>apiserver: Set UpToDate condition to False
    loop For each External Updater
        mach->>hook: Run until completion
    end
    mach->>apiserver: Mark Hooks in Machine as Done
    mach->>apiserver: Set UpToDate condition to True
end
```

The KCP external updates will work in a very similar way to MachineDeployments but removing the MachineSet level of indirection. In this case, it's the KCP controller responsible for marking the machine as pending and updating 
the Machine spec, while the Machine controller manages setting the `UpToDate` condition. This follows this same pattern as for rolling updates, where the KCP controller directly creates and deletes Machines. Machines will be updated one by one, sequentially.

### Machine updates

```mermaid
sequenceDiagram
box Management Cluster
    participant apiserver as kube-api server
    participant capi as CAPI
    participant mach as Machine Controller
    participant hook as External updater
    participant hook1 as Other external updaters
end

box Workload Cluster
    participant infra as Infrastructure
end

capi->>apiserver: Decide Update Strategy
capi->>apiserver: Mark Machine as pending, update spec
apiserver->>mach: Notify changes
mach->>hook: Start update
loop For all External Updaters
  mach->>hook: call UpdateMachine
  hook->>infra: Update components
  alt is pending
    hook->>mach: try in X secs
    Note over hook,mach: Retry loop
  else is done
    hook->>mach: Done
  end
  mach->>hook1: call UpdateMachine
  hook1->>infra: Update components
  alt is pending
    hook1->>mach: try in X secs
    Note over hook1,mach: Retry loop
  else is done
    hook1->>mach: Done
  end
end
mach->>apiserver: Mark Hooks in Machine as Done
mach->>apiserver: Set UpToDate condition to True
```

Once a Machine is marked as pending and `UpToDate` condition is set and the Machine's spec has been updated with the desired changes, the Machine controller takes over. This controller is responsible for calling the updaters and tracking the progress of those updaters and exposing this progress in the Machine conditions.

The Machine controller currently calls registered external updaters sequentially but without a defined order. We are explicitly not trying to design a solution for ordering of execution at this stage. However, determining a specific ordering mechanism or dependency management between update extensions will need to be addressed in future iterations of this proposal.

The controller will trigger updaters by hitting a RuntimeHook endpoint (eg. `/UpdateMachine`). The updater could respond saying "update completed", "update failed" or "update in progress" with an optional "retry after X seconds". The CAPI controller will continuously poll the status of the update by hitting the same endpoint until it reaches a terminal state.

CAPI expects the `/UpdateMachine` endpoint of an updater to be idempotent: for the same Machine with the same spec, the endpoint can be called any number of times (before and after it completes), and the end result should be the same. CAPI guarantees that once an `/UpdateMachine` endpoint has been called once, it won't change the Machine spec until the update either completes or fails.

Once all of the updaters are complete, the Machine controller will mark machine as done. If the update fails, this will be reflected in the Machine status.

From this point on, the `KCP` or `MachineDeployment` controller will take over and set the `UpToDate` condition to `True`.

*Note: We might revisit which controller should set `UpToDate` during implementation, because we have to make sure there are no race conditions that can lead to reconcile failures, but apart from the ownership of this operation, the workflows described in this doc should not be impacted.*

### Infra Machine Template changes

As mentioned before, the user experience to update in-place should be the exact same one as for rolling updates. This includes the need to rotate the Infra machine template. For providers that bundle the kubernetes components in some kind of image, this means that when upgrading kubernetes versions, a new image will be required.

This might seem counter-intuitive, given the update will be made in-place, so there is no need for a new image. However, not only this ensures the experience is the same as in rolling updates, but it also allows new Machines to be created for that MachineDeployment/CP in case a scale up is required or fallback to rolling update.

We leave up to the external updater implementers to decide how to deal with these changes. Some infra providers might have the ability to swap the image in an existing running machine, in which case they can offer a true in-place update for this field. For the ones that can't do this but want to allow changes that require a new image (like kubernetes updates), they should "ignore" the image field when processing the update, leaving the machine in a dirty state.

We might explore the ability to represent this "dirty" state at the API level. We leave this for a future iteration of this feature.

### Remediation

Remediation can be used as the solution to recover machine when in-place update fails on a machine. The remediation process stays the same as today: the MachineHealthCheck controller monitors machine health status and marks it to be remediated based on pre-configured rules, then ControlPlane/MachineDeployment replaces the machine or call external remediation.

However, in-place updates might cause Nodes to become unhealthy while the update is in progress. In addition, an in-place update might take more (or less) time than a fresh machine creation. Hence, in order to successfully use MHC to remediate in-place updated Machines, in a future iteration of this proposal we will consider:
* A mechanism to identify if a Machine is being updated. We will surface this in the Machine status. API details will be added later.
* A way to define different rules for Machines on-going an update. This might involve new fields in the MHC object. We will decouple these API changes from this proposal. For the first implementation of in-place updates, we might decide to just disable remediation for Machines that are on-going an update.

### Examples

This section aims to showcase our vision for the In-Places Updates end state. It shows a high level picture of a few common usecases, specially around how the different components interact through the API.

Note that these examples don't show all the low level details. Some of those details might not yet be defined in this doc and will be added later, the examples here are just to help communicate the vision.

Let's imagine a vSphere cluster with a KCP control plane that has two fictional In-Place update extensions already deployed and registered through their respective `ExtensionConfig`.
1. `vsphere-vm-memory-update`: The extension uses vSphere APIs to hot-add memory to VMs if "Memory Hot Add" is enabled or through a power cycle.
2. `kcp-version-upgrade`: Updates the kubernetes version of KCP machines by using an agent that first updates the kubernetes related packages (`kubeadm`, `kubectl`, etc.) and then runs the `kubeadm upgrade` command. The In-place Update extension communicates with this agent, sending instructions with the kubernetes version a machine needs to be updated to.

> Please note that exact Spec of messages will be defined during implementation; current examples are only meant to explain the flow.

#### KCP kubernetes version update

```mermaid
sequenceDiagram
    participant Operator

    box Management Cluster
        participant apiserver as kube-api server
        participant capi as KCP Controller
        participant mach as Machine Controller
        participant hook as KCP version <br>update extension
        participant hook2 as vSphere memory <br>update extension
    end
    
    box Workload Cluster
        participant machines as Agent in machine
    end    

    Operator->>+apiserver: Update version field in KCP
    apiserver->>+capi: Notify changes
    apiserver->>-Operator: OK
    loop For 3 KCP Machines
        capi->>+hook2: Can update [spec.version,<br>clusterConfiguration.kubernetesVersion]?
        hook2->>capi: I can update []
        capi->>+hook: Can update [spec.version,<br>clusterConfiguration.kubernetesVersion]?
        hook->>capi: I can update [spec.version,<br>clusterConfiguration.kubernetesVersion]
        capi->>capi: Decide Update Strategy
        capi->>apiserver: Mark Machine as pending, update spec
        apiserver->>mach: Notify changes
        mach->>apiserver: Set UpToDate condition to False
        apiserver->>mach: Notify changes
        mach->>hook2: Run update in<br> in Machine
        hook2->>mach: Done
        mach->>hook: Run update in<br> in Machine
        hook->>mach: In progress, requeue in 5 min
        hook->>machines: Update packages and<br>run kubeadm upgrade 1.31
        machines->>hook: Done
        mach->>hook2: Run update in<br> in Machine
        hook2->>mach: Done
        mach->>hook: Run update in<br> in Machine
        hook->>mach: Done
        mach->>apiserver: Mark Hooks in Machine as Done
        mach->>apiserver: Set UpToDate condition
    end
```

The user starts the process by updating the version field in the KCP object:

```diff
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: KubeadmControlPlane
metadata:
  name: kcp-1
spec:
  replicas: 3
  rolloutStrategy:
    type: InPlace
- version: v1.30.0
+ version: v1.31.0
```

The KCP computes the difference between the current CP machines (plus bootstrap config and infra machine) and their desired state and detects a difference for the machine `spec.version` and for the KubeadmConfig `spec.clusterConfiguration.kubernetesVersion`. It then starts calling the external update extensions to see if they can handle these changes.

First, it makes a request to the `vsphere-vm-memory-update/CanUpdateMachine` endpoint of the one of update extension registered, the `vsphere-vm-memory-update` extension in this case:

```json
{
    "changes": ["machine.spec.version", "bootstrap.spec.clusterConfiguration.kubernetesVersion"],
}
```

The `vsphere-vm-memory-update` extension does not support any or the required changes, so it responds with the following message declaring that if does not accept any of the requrested changes:

```json
{
    "error": null,
    "acceptedChanges": [],
}
```

Given that there are still changes not covered, KCP continue with the next update extension, making the same request to the `kcp-version-upgrade/CanUpdateMachine` endpoint of the `kcp-version-upgrade` extension:


```json
{
    "changes": ["machine.spec.version", "bootstrap.spec.clusterConfiguration.kubernetesVersion"],
}
```

The `kcp-version-upgrade` extension detects that this is a KCP machine, verifies that the changes only require a kubernetes version upgrade, and responds:

```json
{
    "error": null,
    "acceptedChanges": ["machine.spec.version", "bootstrap.spec.clusterConfiguration.kubernetesVersion"],
}
```

Now that the KCP knows how to cover all desired changes, it proceeds to mark the first selected KCP machine for update. It sets the pending hook annotation and condition on the machine, which is the signal for the Machine controller to treat this changes differently (as an external update).

```diff
apiVersion: cluster.x-k8s.io/v1beta1
kind: Machine
metadata:
+ annotations:
+   runtime.cluster.x-k8s.io/pending-hooks: ExternalUpdate
  name: kcp-1-hfg374h
spec:
- version: v1.30.0
+ version: v1.31.0
  bootstrap:
    configRef:
      apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
      kind: KubeadmConfig
      name: kcp-1-hfg374h-9wc29
      uid: fc69d363-272a-4b91-aa35-72ccdaa7a427
status:
  conditions:
+ - lastTransitionTime: "2024-12-31T23:50:00Z"
+   status: "False"
+   type: UpToDate
```

These changes are observed by the Machine controller. Then it call all updaters. To trigger the updater, it calls the update extensions one by one. The `vsphere-vm-memory-update/UpdateMachine` receives the first request:

```json
{
    "machineRef": {...},
}
```

Since this extension has not been able to cover any of the changes, it responds with the `Done` (machine controller doesn't need to know if the update was accepted or rejected):

```json
{
    "error": null,
    "status": "Done"
}
```

The Machine controller then sends a simillar request to `kcp-version-upgrade/UpdateMachine` endpoint:

```json
{
    "machineRef": {...},
}
```

When the `kcp-version-upgrade` extension receives the request, it verifies it can read the Machine object, verifies it's a CP machine and triggers the upgrade process by sending the order to the agent. It then responds to the Machine controller:

```json
{
    "error": null,
    "status": "InProgress",
    "retryAfterSeconds": "5m0s"
}
```

The Machine controller then requeues the reconcile request for this Machine for 5 minutes later. On the next reconciliation it repeats the request to the `vsphere-vm-memory-update/UpdateMachine` endpoint:

```json
{
    "machineRef": {...},
}
```

The `vsphere-vm-memory-update` which is idempotent, returns `Done` response, once again.

```json
{
    "error": null,
    "status": "Done"
}
```

The Machine controller then repeats the request to the previously pending `kcp-version-upgrade/UpdateMachine` endpoint:

```json
{
    "machineRef": {...},
}
```

The `kcp-version-upgrade` which has tracked the upgrade process reported by the agent and received the completion event, responds:

```json
{
    "error": null,
    "status": "Done"
}
```

All in-place `ExternalUpdate` hooks are completed execution, so the Machine controller removes the annotation:

```diff
apiVersion: cluster.x-k8s.io/v1beta1
kind: Machine
metadata:
- annotations:
-   runtime.cluster.x-k8s.io/pending-hooks: ExternalUpdate
  name: kcp-1-hfg374h
spec:
  version: v1.31.0
  bootstrap:
    configRef:
      apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
      kind: KubeadmConfig
      name: kcp-1-hfg374h-9wc29
      uid: fc69d363-272a-4b91-aa35-72ccdaa7a427
status:
  conditions:
  - lastTransitionTime: "2024-12-31T23:50:00Z"
    status: "False"
    type: UpToDate
```

On the next KCP reconciliation, it detects that this machine doesn't have any pending hooks and sets the `UpToDate` condition to `True`.

```diff
apiVersion: cluster.x-k8s.io/v1beta1
kind: Machine
metadata:
  name: kcp-1-hfg374h
spec:
  version: v1.31.0
  bootstrap:
    configRef:
      apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
      kind: KubeadmConfig
      name: kcp-1-hfg374h-9wc29
      uid: fc69d363-272a-4b91-aa35-72ccdaa7a427
status:
  conditions:
- - lastTransitionTime: "2024-12-31T23:50:00Z"
-   status: "False"
+ - lastTransitionTime: "2024-12-31T23:59:59Z"
+   status: "True"
    type: UpToDate
```

This process is repeated for the second and third KCP machine, finally marking the KCP object as up to date.

#### Update worker node memory

```mermaid
sequenceDiagram
    participant Operator

    box Management Cluster
        participant apiserver as kube-api server
        participant capi as  MD controller
        participant msc as MachineSet Controller
        participant mach as Machine Controller
        participant hook2 as KCP version <br>update extension
        participant hook as vSphere memory <br>update extension
    end
    

    participant api as vSphere API

    Operator->>+apiserver: Update memory field in<br> VsphereMachineTemplate
    apiserver->>+capi: Notify changes
    apiserver->>-Operator: OK
    capi->>+hook: Can update [spec.memoryMiB]?
        hook->>capi: I can update [spec.memoryMiB]
    capi->>+hook2: Can update []?
        hook2->>capi: I can update []
    capi->>capi: Decide Update Strategy
    capi->>apiserver: Create new MachineSet
    loop For all Machines
        capi->>apiserver: Mark as pending and move to new Machine Set
        apiserver->>msc: Notify changes
        msc->>apiserver: Update Machine's<br> spec.memoryMiB
        apiserver->>mach: Notify changes
        mach->>hook: Run update in<br> in Machine
        hook->>mach: In progress
        hook->>api: Update VM's memory
        api->>hook: Done
        mach->>hook: Run update in<br> in Machine
        hook->>mach: Done
        mach->>apiserver: Mark Machine as updated
        mach->>apiserver: Mark Hooks in Machine as Done
        msc->>apiserver: Set UpToDate condition
    end
```

The user starts the process by creating a new VSphereMachineTemplate with the updated `memoryMiB` value and updating the infrastructure template ref in the MachineDeployment:

```diff
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: VSphereMachineTemplate
metadata:
-  name: md-1-1
+  name: md-1-2
spec:
  template:
    spec:
-     memoryMiB: 4096
+     memoryMiB: 8192
```

```diff
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineDeployment
metadata:
  name: m-cluster-vsphere-gaslor-md-0
spec:
  strategy:
    type: InPlace
  template:
    spec:
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: VSphereMachineTemplate
-       name: md-1-1
+       name: md-1-2
```

The `vsphere-vm-memory-update` extension informs that can cover required requested changes:

```json
{
    "changes": ["infraMachine.spec.memoryMiB"],
}
```

```json
{
    "error": null,
    "acceptedChanges": ["infraMachine.spec.memoryMiB"],
}
```

The request is also made to `kcp-version-upgrade` but it responds with an empty array, indicating it cannot handle any of the changes:

```json
{
    "error": null,
    "acceptedChanges": [],
}
```

The Machine controller then creates a new MachineSet with the new spec and moves the first Machine to it by updating its `OwnerRefs`, it also marks the Machine as pending:

```diff
apiVersion: cluster.x-k8s.io/v1beta1
kind: Machine
metadata:
+ annotations:
+   runtime.cluster.x-k8s.io/pending-hooks: ExternalUpdate
  name: md-1-6bp6g
  ownerReferences:
  - apiVersion: cluster.x-k8s.io/v1beta1
    kind: MachineSet
-   name: md-1-gfsnp
+   name: md-1-hndio
```

The MachineSet controller detects that this machine is out of date and is pending external update execution and proceeds to update the Machine's spec and sets the `UpToDate` condition to `False`:

```diff
apiVersion: cluster.x-k8s.io/v1beta1
kind: Machine
metadata:
  name: md-1-6bp6g
  annotations:
    runtime.cluster.x-k8s.io/pending-hooks: ExternalUpdate
spec:
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: VSphereMachine
-   name: md-1-1-whtwq
+   name: md-1-2-nfdol
status:
+ conditions:
+ - lastTransitionTime: "2024-12-31T23:50:00Z"
+   status: "False"
+   type: UpToDate
```

From that point, the Machine controller follows the same process as in the first example.

The process is repeated for all replicas in the MachineDeployment.

#### Update worker nodes OS from Linux to Windows

```mermaid
flowchart TD
    Update[MachineDeployment<br> spec change] --> UpdatePlane{Can all changes be<br> covered with the registered<br> update extensions?}
    UpdatePlane -->|No| Roll[Fallback to machine<br> replacement rollout]
    UpdatePlane -->|Yes| InPlace[Run extensions to<br> update in place]
```

```mermaid
sequenceDiagram
    participant Operator

    box Management Cluster
        participant apiserver as kube-api server
        participant capi as  MD controller
        participant msc as MachineSet Controller
        participant mach as Machine Controller
        participant hook as vSphere memory <br>update extension
        participant hook2 as KCP version <br>update extension
    end
    

    Operator->>+apiserver: Update template field in<br> VsphereMachineTemplate
    apiserver->>+capi: Notify changes
    apiserver->>-Operator: OK
    capi->>+hook: Can update [spec.template]?
        hook->>capi: I can update []
    capi->>+hook2: Can update [spec.template]?
        hook2->>capi: I can update []
    capi->>apiserver: Create new MachineSet
    loop For all Machines
        capi->>apiserver: Replace machines
    end
```

The user starts the process by creating a new VSphereMachineTemplate with the updated `template` value and updating the infrastructure template ref in the MachineDeployment:

```diff
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: VSphereMachineTemplate
metadata:
-  name: md-1-2
+  name: md-1-3
spec:
  template:
    spec:
-     template: /Datacenter/vm/Templates/kubernetes-1-32-ubuntu
+     template: /Datacenter/vm/Templates/kubernetes-1-32-windows
```

```diff
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineDeployment
metadata:
  name: m-cluster-vsphere-gaslor-md-0
spec:
  strategy:
    type: InPlace
    fallbackRollingUpdate:
      maxUnavailable: 1
  template:
    spec:
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: VSphereMachineTemplate
-       name: md-1-2
+       name: md-1-3
```

Both the `kcp-version-upgrade` and the `vsphere-vm-memory-update` extensions inform that they cannot handle any of the changes:

```json
{
    "changes": ["infraMachine.spec.template"],
}
```

```json
{
    "error": null,
    "acceptedChanges": [],
}
```

Since the fallback to machine replacement is a default strategy and always enabled, the MachineDeployment controller proceeds with the rollout process as it does today, replacing the old machines with new ones.

### Security Model

On the core CAPI side, the security model for this feature is very straightforward: CAPI controllers only require to read/create/update CAPI resources and those controllers are the only ones that need to modify the CAPI resources. Moreover, the controllers that need to perform these actions already have the necessary permissions over the resources they need to modify.

However, each external updater should define their own security model. Depending on the mechanism used to update machines in-place, different privileges might be needed, from scheduling privileged pods to SSH access to the hosts. Moreover, external updaters  might need RBAC to read CAPI resources.

### Risks and Mitigations

The main risk of this change is its complexity. This risk is mitigated by:

1. Implementing the feature in incremental steps. 

2. Avoiding user-facing changes in the first iteration, allowing us to gather feedback and validate the core functionality before making changes that are difficult to revert.

3. Using a feature flag to control the availability of this functionality, ensuring it remains opt-in and can be disabled if issues arise.


## Additional Details

### Test Plan

To test the external update strategy, we will implement a "CAPD Kubeadm Updater". This will serve as a reference implementation and will be integrated into CAPI CI. In-place updates will be performed by executing a set of commands in the container, similar to how it is currently implemented for cloud config when machine is bootstrapped.

### Graduation Criteria

The initial plan is to provide support for external update strategy in the KCP and MD controllers under a feature flag (which would be unset by default) and to have the webhook API in an `alpha` stage ) which will allow us to iterate faster).

The main criteria for graduating this feature will be community adoption and API stability. Once the feature is stable, we have fixes to any known bugs and the webhook API has remained stable without backward incompatible changes for some time, we will propose to the community moving the API out of the `alpha` stage. It will then be promoted out of experimental and the feature flag for enabling/disabling the functionality will be deprecated. When this happens
we will provide a way to toggle the in-place possibly though the API.

## Implementation History

- [ ] MM/DD/YYYY: Proposed idea in an issue or [community meeting]
- [ ] MM/DD/YYYY: Compile a Google Doc following the CAEP template (link here)
- [ ] MM/DD/YYYY: First round of feedback from community
- [ ] MM/DD/YYYY: Present proposal at a [community meeting]
- [ ] MM/DD/YYYY: Open proposal PR

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1ushaVqAKYnZ2VN_aa3GyKlS4kEd6bSug13xaXOakAQI/edit#heading=h.pxsq37pzkbdq
