# Cluster API
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [What is the Cluster API?](#what-is-the-cluster-api)
- [Getting Started](#getting-started)
  - [Resources](#resources)
  - [Prerequisites](#prerequisites)
  - [Using `clusterctl` to create a cluster](#using-clusterctl-to-create-a-cluster)
  - [How does Cluster API compare to Kubernetes Cloud Providers?](#how-does-cluster-api-compare-to-kubernetes-cloud-providers)
- [Get involved!](#get-involved)
- [Provider Implementations](#provider-implementations)
- [API Adoption](#api-adoption)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->
## What is the Cluster API?

The Cluster API is a Kubernetes project to bring declarative, Kubernetes-style
APIs to cluster creation, configuration, and management. It provides optional,
additive functionality on top of core Kubernetes.

Note that Cluster API effort is still in the prototype stage while we get
feedback on the API types themselves. All of the code here is to experiment with
the API and demo its abilities, in order to drive more technical feedback to the
API design. Because of this, all of the prototype code is rapidly changing.

## Getting Started

### Resources

* GitBook: [cluster-api.sigs.k8s.io](https://cluster-api.sigs.k8s.io)

### Prerequisites
* `kubectl` is required, see [here](http://kubernetes.io/docs/user-guide/prereqs/).
* `clusterctl` is a SIG-cluster-lifecycle sponsored tool to manage Cluster API clusters. See [here](cmd/clusterctl)

### Using `clusterctl` to create a cluster
* Doc [here](./docs/how-to-use-clusterctl.md)

![Cluster API Architecture](./docs/book/common_code/architecture.svg "Cluster API Architecture")

Learn more about the project's [scope, objectives, goals and requirements](./docs/scope-and-objectives.md), [feature proposals](./docs/proposals/) and [reference use cases](./docs/staging-use-cases.md).

### How does Cluster API compare to [Kubernetes Cloud Providers](https://kubernetes.io/docs/concepts/cluster-administration/cloud-providers/)?

Cloud Providers and the Cluster API work in concert to provide a rich Kubernetes experience in cloud environments.
The Cluster API initializes new nodes and clusters using available [providers](#Provider-Implementations).
Running clusters can then use Cloud Providers to provision support infrastructure like
[load balancers](https://kubernetes.io/docs/tasks/access-application-cluster/create-external-load-balancer/)
and [persistent volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/).

<!-- ANCHOR: Get Involved -->

# Get involved!

- Join the [Cluster API discuss forum](https://discuss.kubernetes.io/c/contributors/cluster-api).
- Join the [SIG Cluster Lifecycle](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle) Google Group for access to documents and calendars.
- Join our Cluster API working group sessions.
    - Weekly on Wednesdays @ 10:00 PT on [Zoom][zoomMeeting]
    - Previous meetings: \[ [notes][notes] | [recordings][recordings] \]
- Provider implementers office hours
    - Weekly on Tuesdays @ 12:00 PT ([Zoom](providerZoomMeetingTues)) and Wednesdays @ 15:00 CET ([Zoom](providerZoomMeetingWed))
    - Previous meetings: \[ [notes][implementerNotes] \]
- Chat with us on [Slack](http://slack.k8s.io/) in the _#cluster-api_ channel.

## Versioning, Maintenance, and Compatibility

- We follow [Semantic Versioning (semver)](https://semver.org/).
- Cluster API release cadence is Kubernetes Release + 6 weeks.
- The cadence is subject to change if necessary, refer to the [Milestones](https://github.com/kubernetes-sigs/cluster-api/milestones) page for up-to-date information.
- The _master_ branch is where development happens, this might include breaking changes.
- The _release-X_ branches contain stable, backward compatible code. A new _release-X_ branch is created at every major (X) release.

[notes]: https://docs.google.com/document/d/1Ys-DOR5UsgbMEeciuG0HOgDQc8kZsaWIWJeKJ1-UfbY/edit
[recordings]: https://www.youtube.com/playlist?list=PL69nYSiGNLP29D0nYgAGWt1ZFqS9Z7lw4
[zoomMeeting]: https://zoom.us/j/861487554
[implementerNotes]: https://docs.google.com/document/d/1IZ2-AZhe4r3CYiJuttyciS7bGZTTx4iMppcA8_Pr3xE/edit
[providerZoomMeetingTues]: https://zoom.us/j/140808484
[providerZoomMeetingWed]: https://zoom.us/j/424743530

<!-- ANCHOR_END: Get Involved -->
