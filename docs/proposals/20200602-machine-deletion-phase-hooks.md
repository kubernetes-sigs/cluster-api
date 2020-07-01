---
title: Machine Deletion Phase Hooks
authors:
  - "@michaelgugino"
reviewers:
  - "@enxebre"
  - "@vincepri"
  - "@detiber"
  - "@ncdc"
creation-date: 2020-06-02
last-updated: 2020-08-07
status: implemented
---

# Machine Deletion Phase Hooks

## Table of Contents
<!-- toc -->
- [Machine Deletion Phase Hooks](#machine-deletion-phase-hooks)
  - [Table of Contents](#table-of-contents)
  - [Glossary](#glossary)
    - [lifecycle hook](#lifecycle-hook)
    - [deletion phase](#deletion-phase)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
      - [Story 3](#story-3)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
      - [Lifecycle Points](#lifecycle-points)
        - [pre-drain](#pre-drain)
        - [pre-terminate](#pre-terminate)
      - [Annotation Form](#annotation-form)
        - [lifecycle-point](#lifecycle-point)
        - [hook-name](#hook-name)
        - [owner (Optional)](#owner-optional)
        - [Annotation Examples](#annotation-examples)
      - [Changes to machine-controller](#changes-to-machine-controller)
        - [Reconciliation](#reconciliation)
        - [Hook failure](#hook-failure)
        - [Hook ordering](#hook-ordering)
      - [Owning Controller Design](#owning-controller-design)
        - [Owning Controllers must](#owning-controllers-must)
        - [Owning Controllers may](#owning-controllers-may)
      - [Determining when to take action](#determining-when-to-take-action)
        - [Failure Mode](#failure-mode)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Alternatives](#alternatives)
    - [Custom Machine Controller](#custom-machine-controller)
    - [Finalizers](#finalizers)
    - [Status Field](#status-field)
    - [Spec Field](#spec-field)
    - [CRDs](#crds)
  - [Upgrade Strategy](#upgrade-strategy)
  - [Additional Details](#additional-details)
<!-- /toc -->

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

### lifecycle hook
A specific point in a machine's reconciliation lifecycle where execution of
normal machine-controller behavior is paused or modified.

### deletion phase
Describes when a machine has been marked for deletion but is still present
in the API.  Various actions happen during this phase, such as draining a node,
deleting an instance from a cloud provider, and deleting a node object.

### Hook Implementing Controller (HIC)
The Hook Implementing Controller describes a controller, other than the
machine-controller, that adds, removes, and/or responds to a particular
lifecycle hook.  Each lifecycle hook should have a single HIC, but an HIC
can optionally manage one or more hooks.

## Summary

Defines a set of annotations that can be applied to a machine which affect the
linear progress of a machine’s lifecycle after a machine has been marked for
deletion.  These annotations are optional and may be applied during machine
creation, sometime after machine creation by a user, or sometime after machine
creation by another controller or application.

## Motivation

Allow custom and 3rd party components to easily interact with a machine or
related resources while that machine's reconciliation is temporarily paused.
This pause in reconciliation will allow these custom components to take action
after a machine has been marked for deletion, but prior to the machine being
drained and/or associated instance terminated.

### Goals

- Define an initial set of hook points for the deletion phase.
- Define an initial set and form of related annotations.
- Define basic expectations for a controller or process that responds to a
lifecycle hook.


### Non-Goals/Future Work

- Create an exhaustive list of hooks; we can add more over time.
- Create new machine phases.
- Create a mechanism to signal what lifecycle point a machine is at currently.
- Dictate implementation of controllers that respond to the hooks.
- Implement ordering in the machine-controller.
- Require anyone use these hooks for normal machine operations, these are
strictly optional and for custom integrations only.


## Proposal

- Utilize annotations to implement lifecycle hooks.
- Each lifecycle point can have 0 or more hooks.
- Hooks do not enforce ordering.
- Hooks found during machine reconciliation effectively pause reconciliation
until all hooks for that lifecycle point are removed from a machine's annotations.


### User Stories
#### Story 1
(pre-terminate) As an operator, I would like to have the ability to perform
different actions between the time a machine is marked deleted in the api and
the time the machine is deleted from the cloud.

For example, when replacing a control plane machine, ensure a new control
plane machine has been successfully created and joined to the cluster before
removing the instance of the deleted machine. This might be useful in case
there are disruptions during replacement and we need the disk of the existing
instance to perform some disaster recovery operation.  This will also prevent
prolonged periods of having one fewer control plane host in the event the
replacement instance does not come up in a timely manner.

#### Story 2
(pre-drain) As an operator, I want the ability to utilize my own draining
controller instead of the logic built into the machine-controller.  This will
allow me better flexibility and control over the lifecycle of workloads on each
node.

### Implementation Details/Notes/Constraints

For each defined lifecycle point, one or more hooks may be applied as an annotation to the machine object.  These annotations will pause reconciliation of a machine object until all hooks are resolved for that lifecycle point.  The hooks should be managed by an Hook Implementing Controler or other external application, or
manually created and removed by an administrator.

#### Lifecycle Points
##### pre-drain
`pre-drain.delete.hook.machine.cluster.x-k8s.io`

Hooks defined at this point will prevent the machine-controller from draining a node after the machine-object has been marked for deletion until the hooks are removed.
##### pre-terminate
`pre-terminate.delete.hook.machine.cluster.x-k8s.io`

Hooks defined at this point will prevent the machine-controller from
removing/terminating the instance in the cloud provider until the hooks are
removed.

"pre-terminate" has been chosen over "pre-delete" because "terminate" is more
easily associated with an instance being removed from the cloud or
infrastructure, whereas "delete" is ambiguous as to the actual state of the
machine in its lifecycle.


#### Annotation Form
```
<lifecycle-point>.delete.hook.machine.cluster-api.x-k8s.io/<hook-name>: <owner/creator>
```

##### lifecycle-point
This is the point in the lifecycle of reconciling a machine the annotation will have effect and pause the machine-controller.

##### hook-name
Each hook should have a unique and descriptive name that describes in 1-3 words what the intent/reason for the hook is.  Each hook name should be unique and managed by a single entity.

##### owner (Optional)
Some information about who created or is otherwise in charge of managing the annotation.  This might be a controller or a username to indicate an administrator applied the hook directly.

##### Annotation Examples

These examples are all hypothetical to illustrate what form annotations should
take.  The names of of each hook and the respective controllers are fictional.

pre-drain.hook.machine.cluster-api.x-k8s.io/migrate-important-app: my-app-migration-controller

pre-terminate.hook.machine.cluster-api.x-k8s.io/backup-files: my-backup-controller

pre-terminate.hook.machine.cluster-api.x-k8s.io/wait-for-storage-detach: my-custom-storage-detach-controller

#### Changes to machine-controller
The machine-controller should check for the existence of 1 or more hooks at
specific points (lifecycle-points) during reconciliation.  If a hook matching
the lifecycle-point is discovered, the machine-controller should stop
reconciling the machine.

An example of where the pre-drain lifecycle-point might be implemented:
https://github.com/kubernetes-sigs/cluster-api/blob/30c377c0964efc789ab2f3f7361eb323003a7759/controllers/machine_controller.go#L270

##### Reconciliation
When a Hook Implementing Controller updates the machine, reconciliation will be
triggered, and the machine will continue reconciling as normal, unless another
hook is still present; there is no need to 'fail' the reconciliation to
enforce requeuing.

When all hooks for a given lifecycle-point are removed, reconciliation
will continue as normal.

##### Hook failure
The machine-controller should not timeout or otherwise consider the lifecycle
hook as 'failed.'  Only the Hook Implementing Controller may decide to remove a
particular lifecycle hook to allow the machine-controller to progress past the
corresponding lifecycle-point.

##### Hook ordering
The machine-controller will not attempt to enforce any ordering of hooks.  No
ordering should be expected by the machine-controller.

Hook Implementing Controllers may choose to provide a mechanism to allow
ordering amongst themselves via whatever means HICs determine.  Examples could
be using CRDs external to the machine-api, gRPC communications, or
additional annotations on the machine or other objects.

#### Hook Implementing Controller Design
Hook Implementing Controller is the component that manages a particular
lifecycle hook.

##### Hook Implementing Controllers must
* Watch machine objects and determine when an appropriate action must be taken.
* After completing the desired hook action, remove the hook annotation.

##### Hook Implementing Controllers may
* Watch machine objects and add a hook annotation as desired by the cluster
administrator.
* Coordinate with other Hook Implementing Controllers through any means
possible, such as using common annotations, CRDs, etc. For example, one hook
controller could set an annotation indicating it has finished its work, and
another hook controller could wait for the presence of the annotation before
proceeding.

#### Determining when to take action

An Hook Implementing Controller should watch machines and determine when is the
best time to take action.

For example, if an HIC manages a lifecycle hook at the pre-drain lifecycle-point,
then that controller should take action immediately after a machine has a
DeletionTimestamp or enters the "Deleting" phase.

Fine-tuned coordination is not possible at this time; eg, it's not
possible to execute a pre-terminate hook only after a node has been drained.
This is reserved for future work.

##### Failure Mode
It is entirely up to the Hook Implementing Controller to determine when it is
prudent to remove a particular lifecycle hook. Some controllers may want to
'give up' after a certain time period, and others may want to block indefinitely.
Cluster operators should consider the characteristics of each controller before
utilizing them in their clusters.


### Risks and Mitigations

* Annotation keys must conform to length limits: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/#syntax-and-character-set
* Requires well-behaved controllers and admins to keep things running
smoothly.  Would be easy to disrupt machines with poor configuration.
* Troubleshooting problems may increase in complexity, but this is
mitigated mostly by the fact that these hooks are opt-in.  Operators
will or should know they are consuming these hooks, but a future proliferation
of the cluster-api could result in these components being bundled as a
complete solution that operators just consume.  To this end, we should
update any troubleshooting guides to check these hook points where possible.


## Alternatives

### Custom Machine Controller
Require advanced users to fork and customize.  This can already be done if someone chooses, so not much of a solution.

### Finalizers
We define additional finalizers, but this really only implies the deletion lifecycle point.  A misbehaving controller that
accidentally removes finalizers could have undesireable
effects.

### Status Field
Harder for users to modify or set hooks during machine creation.  How would a user remove a hook if a controller that is supposed to remove it is misbehaving?  We’d probably need an annotation like ‘skip-hook-xyz’ or similar and that seems redundant to just using annotations in the first place

### Spec Field
We probably don’t want other controllers dynamically adding and removing spec fields on an object.  It’s not very declarative to utilize spec fields in that way.

### CRDs
Seems like we’d need to sync information to and from a CR.  There are different approaches to CRDs (1-to-1 mapping machine to CR, match labels, present/absent vs status fields) that each have their own drawbacks and are more complex to define and configure.


## Upgrade Strategy

Nothing defined here should directly impact upgrades other than defining hooks that impact creation/deletion of a machine, generally.

## Additional Details

Fine-tuned timing of hooks is not possible at this time.

In the future, it is possible to implement this timing via additional
machine phases, or possible "sub-phases" or some other mechanism
that might be appropriate.  As stated in the non-goals, that is
not in scope at this time, and could be future work.  This is currently
being discussed in [issue 3365].

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1Ys-DOR5UsgbMEeciuG0HOgDQc8kZsaWIWJeKJ1-UfbY
[issue 3365]: https://github.com/kubernetes-sigs/cluster-api/issues/3365
