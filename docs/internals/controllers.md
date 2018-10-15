# Controllers

The main controller implementations ([cluster] and [machine]) are located
within  the `cluster-api` library, and each perform almost identical basic
steps within their `Reconcile` methods. The controllers have the responsibility
of watching for changes to specific resource types and reacting to those
changes in order to reach a desired state by calling out to the necessary
actuator methods.

  - Controller watches for changes on a resource type
  - A change on a watched resource type is observed and the controller
    receives a reconcile request
  - An attempt is made to fetch the appropriate object from Kubernetes; and if
    that is successful
  - The object is inspected to see what the appropriate actions to take are
  - Actuator methods are called to realize the state as necessary

At this level, we do not really know what a cluster or machine will really be
defined as, as individual providers will decide that. What we know is that:

  - If machines exist, they will exist as part of a cluster
  - A cluster may contain machines, but not necessarily
  - A cluster may contain other clusters
  - A cluster may contain configuration shared between its machine resources

Further details on the [Cluster Controller] and the [Machine Controller] may be
found in their respective documents.

<!-- Links used in the document, placed here to avoid breaking the document
     flow with URLs in the middle of a sentence. --> 
[cluster]: https://github.com/kubernetes-sigs/cluster-api/blob/master/pkg/controller/cluster/controller.go
[machine]: https://github.com/kubernetes-sigs/cluster-api/blob/master/pkg/controller/machine/controller.go
[Cluster Controller]: cluster_controller.md
[Machine Controller]: machine_controller.md
