# Introduction

*To share this book use the icons in the top-right of the menu.*

**Note:** Impatient readers may head straight to [Existing Providers](
getting_started/existing_providers.md) section.

## What is the Cluster API?
  
The Cluster API is a Kubernetes project to bring declarative, [Kubernetes-style 
APIs][k8s-apis] to cluster creation, configuration, and management. It
provides optional, additive functionality on top of core Kubernetes. By making
use of structured nature of kubernetes APIs it is possible to build higher-level
cloud agnostic tools which improve user experience by allowing for greater
ease of use and more sophisticated automation.

[k8s-apis]: https://kubernetes.io/docs/concepts/overview/kubernetes-api/

## Who is this book for now?

#### Kubernetes Developers

Kubernetes Developers will learn how to develop Cluster API providers which 
allow Kubernetes to be deployed to ever more environments, thereby increasing 
how often and where users can deploy their applications.

Including the ability to:

- Develop new Cluster and Machine controllers.
- Develop automation on top of MachineSets and MachineDeployments as well
as other Cluster API resources. For example,
  - Cloud agnostic upgrade and repair tools.
  - Cluster autoscalers.
  - Etc.

## Who else will this book be for in the future?

#### Kubernetes Users

A Kubernetes User can be an Application User, Developer, or anyone else who
needs access to a Kubernetes cluster. One of the aims of the Cluster API
project is to leverage the relative uniformity of Kubernetes APIs and 
associated tooling to make it easier for ordinary users to access computational
resources in a portable way.

#### Infrastructure Engineers

Infrastructure Engineers will learn the fundamental concepts of how Kubernetes
clusters are built according to best practices, and how they can be managed
through Kubernetes native abstractions.

Including the ability to:

- Create reproducible Kubernetes clusters.
- Create hybrid cloud environments which optimize for cost, performance, and
reliability. 

## Resources

- GitBook: [kubernetes-sigs.github.io/cluster-api](https://kubernetes-sigs.github.io/cluster-api)
- GitHub Repo: [kubernetes-sigs/cluster-api](https://github.com/kubernetes-sigs/cluster-api)
- Slack channel: [#cluster-api](http://slack.k8s.io/#cluster-api)
- Google Group: [kubernetes-sig-cluster-lifecycle@googlegroups.com](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle)

## Navigating this book

This section describes how to use the navigation elements of this book.

##### Code Navigation

Code samples may be either displayed to the side of the corresponding 
documentation, or inlined immediately afterward.  This setting may be toggled 
using the split-screen icon at the left side of the top nav.

##### Table of Contents

The table of contents may be hidden using the hamburger icon at the left side 
of the top nav.

##### OS / Language Navigation

Some chapters have code snippets for multiple OS or Languages.  These chapters 
will display OS or Language selections at the right side of the top nav, which 
may be used to change the OS or Language of the examples shown.

<!-- References -->

