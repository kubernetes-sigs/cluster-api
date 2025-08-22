# Cluster API Manifesto

## Intro

Taking inspiration from [Tim Hockin’s talk at KubeCon NA 2023](https://www.youtube.com/watch?v=WqeShpaztZY), also for the
Cluster API project is important to define the long term vision, the manifesto of “where we are going” and “why”.

This document would hopefully provide valuable context for all users, contributors and companies investing in this project,
as well as act as compass for all reviewers and maintainers currently working on it.

## Community

_Together we can go far._

The Cluster API community is the foundation for this project’s past, present and future.
The project will continue to encourage and praise active participation and contribution.

_We are an active part of a bigger ecosystem_

The Cluster API community is an active part of Kubernetes SIG Cluster Lifecycle, of the broader Kubernetes community
and of the CNCF.

CNCF provides the core values this project recognizes and contributes to.
The Kubernetes community provides most of the practices and policies this project abides to or is inspired by.

## Core goals and design principles

The project remains true to its original goals and design principles:

_Cluster API is a Kubernetes sub-project focused on providing declarative APIs and tooling to simplify provisioning,
upgrading, and operating multiple Kubernetes clusters._

Nowadays, like at the beginning of the project, some concepts from the above statement deserve further clarification.

### Declarative APIs

The Cluster API project motto is “Kubernetes all the way down”, and this boils down to two elements.

The target state of a cluster can be defined using Kubernetes declarative APIs.

The project also implements controllers – Kubernetes reconcile loops  – ensuring that desired and current state of the
cluster will remain consistent over time.

The combination of those elements, declarative APIs and controllers, defines “how” this project aims to make Kubernetes
and Cluster API a _stable, reliable and consistent platform_ that just works to enable higher order business value
supported by cloud-native applications.

### Simplicity

Kubernetes Cluster lifecycle management is a complex problem space, especially if you consider doing this across so
many different types of infrastructures.

Hiding this complexity behind a simple declarative API is “why” the Cluster API project ultimately exists.

The project is strongly committed to continue its quest in defining a set of common API primitives working consistently
across all infrastructures (one API to rule them all).

Working towards graduating our API to v1 will be the next step in this journey.

While doing so, the project should be inspired by Tim Hockin’s talk, and continue to move forward without increasing
operational and conceptual complexity for Cluster API’s users.

## The right to be Unfinished

Like Kubernetes, also the Cluster API project claims the right to remain unfinished, because there is still a strong,
foundational need to continuously evolve, improve and adapt to the changing needs of Cluster API’s users and to the
growing Cloud Native ecosystem.

What is important to notice, is that being a project that is “continuously evolving” is not in contrast with another
request from Cluster API’s users, which is about the project being stable, as expected by a system that has “crossed the chasm”.

Those two requests from Cluster API’s users are two sides of the same coin, a reminder that Cluster API must 
“evolve responsibly” by ensuring upgrade paths and avoiding (or at least minimizing) disruptions for users.

The Cluster API project will continue to “evolve responsibly” by abiding to the same guarantees that Kubernetes offers
for its own APIs, but also ensuring a continuous and obsessive focus on CI signal, test coverage and test flakes.

Also ensuring a predictable release calendar, clear support windows and compatibility matrix for each release
is a crucial part of this effort to “evolve responsibly”.

## The complexity budget

Tim Hockins explains the idea of complexity budget very well in his talk:

There is a finite amount of complexity that a project can absorb over a certain amount of time;
when the complexity budget runs out, bad things happen, quality decreases, we can’t fix bugs timely etc.

Since the beginning of the Cluster API project, its maintainers intuitively handled the complexity budget by following
this approach:

“We’ve got to say no to things today, so we can afford to do interesting things tomorrow”.

This is something that is never done lightly, and it is always the result of an open discussion considering the status
of the codebase, the status of the project CI signal, the complexity of the new feature etc. .

Being very pragmatic, also the resources committed to implement and to maintain a feature over time must be considered
when doing such an evaluation, because a model where everything falls on the shoulders of a small set of core
maintainers is not sustainable.

On the other side of this coin, Cluster API maintainer’s also claim the right to reconsider new ideas or ideas
previously put on hold whenever there are the conditions and the required community consensus to work on it.

Probably the most well-known case of this is about Cluster API maintainers repeatedly deferring on change requests
about nodes mutability in the initial phases of the project, while starting to embrace some mutable behavior in recent releases.

## Core and providers

_Together we can go far._

The Cluster API project is committed to keep working with the broader CAPI community – all the Cluster API providers –
as a single team in order to continuously improve and expand the capability of this solution.

As we learned the hard way, the extensibility model implemented by CAPI to support so many providers requires a
complementary effort to continuously explore new ways to offer a cohesive solution, not a bag of parts.

It is important to continue and renew efforts to make it easier to bootstrap and operate a system composed of many
components, to ensure consistent APIs and behaviors, to ensure quality across the board.

This effort lays its foundation in all the provider maintainers being committed to this goal, while the Cluster API project
will be the venue where common guidelines are discussed and documented, as well as the place of choice where common
components or utilities are developed and hosted.
