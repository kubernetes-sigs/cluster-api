# Controllers

This section of the book provides an overview about "core" controllers in Cluster API.

<aside class="note warning">

<h1>The code is the source of truth!</h1>

While we put a great effort in ensuring a good documentation for Cluster API, we also recognize that some
part of the documentation are more prone to miss details or become outdated.

Unfortunately, this section is one of those parts, because things in Cluster API change fast and the
complexity of core controllers keeps growing.

Please feel free to open issues or even better send PRs with improvement that can make this documentation 
even more valuable for the readers that will follow you.

</aside>

### Controller reentrancy

In CAPI most of the coding activities happen in controllers, and in order to make robust controllers,
we should strive for implementing reentrant code.

A reentrant code can be interrupted in the middle of its execution and then safely be called again
("re-entered"); this concept, applied to Kubernetes controllers, means that a controller should be capable
of recovering from interruptions, observe the current state of things, and act accordingly. e.g.

- We should not rely on flags/conditions from previous reconciliations since we are the controller
  setting the conditions. Instead, we should detect the status of things through introspection at
  every reconciliation and act accordingly.
- It is acceptable to rely on status flags/conditions that we've previously set as part
  of the current reconciliation.
- It is acceptable to rely on status flags/conditions set by other controllers.

NOTE: An important use case for reentrancy is the move operation, where Cluster API objects gets moved
to a different management cluster and the controller running on the target cluster has to
rebuild the object status from scratch by observing the current state of the underlying infrastructure.