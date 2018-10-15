# Cluster Controller

The cluster controller is responsible for taking an incoming Kubernetes
cluster request, figuring out what should be done with it, and dispatching it
to the cluster actuator methods.

The cluster actuator methods will communicate with the API that will eventually
realize resources within the cluster that machines within the cluster require
to operate. This may include resources such as the networks, bastion hosts,
load balancers, etc, in what is likely the common case. However, it is up to
the provider implementation to decide what constitutes a cluster.

The basic methods for performing this reconcilation are the `Reconcile` and
`Delete` methods and the interface for this can be seen in the
[`/pkg/controller/cluster/actuator.go`] file.

Once the basic cluster resources are available (reconciliation has succeeded)
the cluster is ready for the machine controller to step in and begin creating
machine resources.

The creation of these machine resources in Kubernetes is likely to have
happened at the same time as the request to create a cluster, however, the
machine controller has been waiting for the cluster to be ready before it
starts working on creating the machine resources.

## Basic Controller Flow

  - Request comes in
  - Attempt to get the cluster object that the request is for from the API
    server
  - Check if the cluster is being deleted or not.
    - If it is being deleted:
      - No-op if there’s no finalizer.
      - Delete the cluster by calling actuator `Delete` method.
      - If the cluster was deleted, delete the finalizer.
    - If it is not being deleted:
      - If there's no finalizer, add a finalizer.
  - Reconcile the cluster by calling the actuator `Reconcile` method.
    - If reconciliation fails and returns a retryable error:
      - Retry `Reconcile` after N seconds.
  - Desired state is reached.

<!-- Links used in the document, placed here to avoid breaking the document
     flow with URLs in the middle of a sentence. -->
[`/pkg/controller/cluster/actuator.go`]: https://github.com/kubernetes-sigs/cluster-api/blob/master/pkg/controller/cluster/actuator.go
