---
title: Cluster Autoscaler Scale from Zero UX
authors:
  - "@michaelgugino"
reviewers:
  - "@detiber"
creation-date: 2020-03-04
creation-date: 2020-03-04
status:
---

# Cluster Autoscaler Scale from Zero UX

## Summary
Cluster Autoscaler supports scale from zero for cloud-native providers. This is done via compiling CPU, GPU, and RAM info into the cluster autoscaler at build time for each supported cloud.

We are attempting to add cluster-api support to the cluster-autoscaler to scale machinesets, machinedeployments, and/or other groupings of machines as appropriate. We are trying to keep an abstraction which is isolated from particular cloud implementations. We need a way to inform the cluster-autoscaler of the value of machine resources so it can make proper scheduling decisions when there are not currently any machines in a particular scale set (machineset/machinedeployment, etc).

We will need input from the autoscaler team, as well as input from api reviewers to ensure we obtain the best solution.

These ideas require a particular (possibly new) cluster-api cloud provider controller
which can inspect machinesets and annotate them as appropriate.  This could be
done dynamically via billing API (not recommended) or have values compiled in
statically.

We could also attempt to build this logic into the machineset/machinedeployment
controller, but this would probably not be very scalable and would require
a well defined set of fields on the provider CRD.

## Motivation

### Goals

### Non-Goals

## Proposal

### User Stories

As a cluster admin, I want to add the ability to automatically scale my cluster
using cluster autoscaler and the cluster-api.  In order to save money, I need
the ability to scale from zero for a given machineset or machinedeployment.

## Proposal Ideas

There are several proposals to consider.  Once we decide on one, we'll eliminate
the others.

### Annotations
Modify the autoscaler to look at particular annotations on a given object to determine the appropriate amounts of each resource.

#### Benefits
Easy, simple.  Users can modify if needed (we can instruct controllers to only
add annotations if they're missing, etc), users can bring their own clouds
and annotate their objects to support scale from zero and we don't need to worry
about supporting every cloud day 1.

#### Cons
We're essentially creating an API around an annotation, and the api folks don't
like that type of thing.

### Spec Field
We can add fields to the spec of a particular object to set these items.

#### Benefits
Well defined API.

Users can set these fields, or some controller can
populate them.

#### Cons
Automatically populating Spec fields is not ideal.  For starters, a user might
copy an existing machineset to create a new one with a different instance type,
and might overlook these fields because they didn't set them.  This would
definitely cause problems.

What do we do if the user specifies some fields and not others.

Automatically populating spec seems to violate 'spec == user intent' principle.

### Status Field
We can add fields to the status of an object.

#### Benefits
Makes the most sense.  If it's information we're determining programatically
about an object that other things need to consume, it should probably go here.

Well defined API.

#### Cons
Users can't easily update this field if they want to set values themselves for
unsupported clouds or clouds where billing data is not common or available
such as OpenStack.

### Status Field + Override Mechanism
Use the status field method as mentioned above, but add an override field to
the Spec or support an annotation on the machineset/machinedeployment object
to copy information into the status field.  The autoscaler always reads from
the status field, we're just making it easy to get data there.

#### Benefits
All the benefits of the Status Field option above. Users can set their own
values easily without having to create special status client to do it (eg,
can use kubectl or specify the values at object creation time).

Since we're calling these new override fields (either in the spec or annoation)
it should be clear what the intent of the values are.  It solves the problem
of user only specifies some fields and not others (eg, they only override CPU).

Clean separation of discovered values and user intent.

#### Cons
We have to build something that copies these values into the staus.  This seems
like it could be an easily shared piece of library code.

Multiple controllers updating the same field on the status object, need to
ensure the logic is sound on both ends so there's not a reconcile war.
