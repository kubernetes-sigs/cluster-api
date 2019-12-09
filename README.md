<p align="center"><img alt="capi" src="./docs/book/src/images/introduction.png" width="160x" /></p>
<p align="center"><a href="https://prow.k8s.io/?job=ci-cluster-api-build">
<!-- prow build badge, godoc, and go report card-->
<img alt="Build Status" src="https://prow.k8s.io/badge.svg?jobs=ci-cluster-api-build">
</a> <a href="https://godoc.org/sigs.k8s.io/cluster-api"><img src="https://godoc.org/sigs.k8s.io/cluster-api?status.svg"></a> <a href="https://goreportcard.com/report/sigs.k8s.io/cluster-api"><img alt="Go Report Card" src="https://goreportcard.com/badge/sigs.k8s.io/cluster-api" /></a></p>

# Cluster API

## Please see our [Book](https://cluster-api.sigs.k8s.io) for more in-depth documentation.

#### Useful links
- [Scope, objectives, goals and requirements](./docs/scope-and-objectives.md)
- [Feature proposals](./docs/proposals)
- [Reference use cases](./docs/staging-use-cases.md)
- [Quick Start](https://cluster-api.sigs.k8s.io/user/quick-start.html)

## What is the Cluster API?

The Cluster API is a Kubernetes project to bring declarative, Kubernetes-style
APIs to cluster creation, configuration, and management. It provides optional,
additive functionality on top of core Kubernetes.

__NB__: Cluster API is still in a prototype stage while we get
feedback on the API types themselves. All of the code here is to experiment with
the API and demo its abilities, in order to drive more technical feedback to the
API design. Because of this, all of the codebase is rapidly changing.

<!-- ANCHOR: Community -->

## Community, discussion, contribution, and support

- Chat with us on [Slack](http://slack.k8s.io/) in the _#cluster-api_ channel
- Join the [SIG Cluster Lifecycle](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle) Google Group for access to documents and calendars
- Join our Cluster API working group sessions
    - Weekly on Wednesdays @ 10:00 PT on [Zoom][zoomMeeting]
    - Previous meetings: \[ [notes][notes] | [recordings][recordings] \]
- Provider implementers office hours
    - Weekly on Tuesdays @ 12:00 PT ([Zoom](providerZoomMeetingTues)) and Wednesdays @ 15:00 CET ([Zoom](providerZoomMeetingWed))
    - Previous meetings: \[ [notes][implementerNotes] \]

Pull Requests are very welcome!
See the [issue tracker] if you're unsure where to start, or feel free to reach out to discuss.

See also: our own [contributor guide](CONTRIBUTING.md) and the Kubernetes [community page].

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[community page]: https://kubernetes.io/community
[notes]: https://docs.google.com/document/d/1Ys-DOR5UsgbMEeciuG0HOgDQc8kZsaWIWJeKJ1-UfbY/edit
[recordings]: https://www.youtube.com/playlist?list=PL69nYSiGNLP29D0nYgAGWt1ZFqS9Z7lw4
[zoomMeeting]: https://zoom.us/j/861487554
[implementerNotes]: https://docs.google.com/document/d/1IZ2-AZhe4r3CYiJuttyciS7bGZTTx4iMppcA8_Pr3xE/edit
[providerZoomMeetingTues]: https://zoom.us/j/140808484
[providerZoomMeetingWed]: https://zoom.us/j/424743530
[issue tracker]: https://github.com/kubernetes-sigs/cluster-api/issues


<!-- ANCHOR_END: Community -->
