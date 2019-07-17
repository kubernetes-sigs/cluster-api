# Cluster API bootstrap provider kubeadm
## What is the Cluster API bootstrap provider kubeadm?

Cluster API bootstrap provider is a component of
[Cluster API](https://github.com/kubernetes-sigs/cluster-api/blob/master/README.md) 
that is responsible of generating a cloud-init script to
turn a Machine into a Kubernetes node; this implementation uses [kubeadm](https://github.com/kubernetes/kubeadm) 
for K8s bootstrap.

### Resources

* [design doc](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20190610-machine-states-preboot-bootstrapping.md)
* [cluster-api.sigs.k8s.io](https://cluster-api.sigs.k8s.io)
* [The Kubebuilder Book](https://book.kubebuilder.io)

## Versioning, Maintenance, and Compatibility

- We follow [Semantic Versioning (semver)](https://semver.org/).
- Cluster API bootstrap provider kubeadm versioning is syncronized with
  [Cluster API](https://github.com/kubernetes-sigs/cluster-api/blob/master/README.md).
- The _master_ branch is where development happens, this might include breaking changes.
- The _release-X_ branches contain stable, backward compatible code. A new _release-X_ branch
  is created at every major (X) release.

## Get involved!

* Join the [Cluster API discuss forum](https://discuss.kubernetes.io/c/contributors/cluster-api).

* Join the [sig-cluster-lifecycle](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle)
Google Group for access to documents and calendars.

* Join our Cluster API working group sessions
  * Weekly on Wednesdays @ 10:00 PT on [Zoom][zoomMeeting]
  * Previous meetings: \[ [notes][notes] | [recordings][recordings] \]

* Provider implementer office hours
  * Weekly on Tuesdays @ 12:00 PT ([Zoom][providerZoomMeetingTues]) and Wednesdays @ 15:00 CET ([Zoom][providerZoomMeetingWed])
  * Previous meetings: \[ [notes][implementerNotes] \]

* Chat with us on [Slack](http://slack.k8s.io/): #cluster-api

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE

### Useful links

* notes: https://docs.google.com/document/d/1Ys-DOR5UsgbMEeciuG0HOgDQc8kZsaWIWJeKJ1-UfbY/edit
* recordings: https://www.youtube.com/playlist?list=PL69nYSiGNLP29D0nYgAGWt1ZFqS9Z7lw4
* zoomMeeting: https://zoom.us/j/861487554
* implementerNotes: https://docs.google.com/document/d/1IZ2-AZhe4r3CYiJuttyciS7bGZTTx4iMppcA8_Pr3xE/edit
* providerZoomMeetingTues: https://zoom.us/j/140808484
* providerZoomMeetingWed: https://zoom.us/j/424743530
