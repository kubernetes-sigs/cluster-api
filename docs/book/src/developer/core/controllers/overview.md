# Controllers

This section of the book provides an overview about "core" controllers in Cluster API.

> [!IMPORTANT]
> **The code is the source of truth!**
>
> While we put a great effort in ensuring a good documentation for Cluster API, we also recognize that some
> part of the documentation are more prone to miss details or become outdated.
>
> Unfortunately, this section is one of those parts, because things in Cluster API change fast and the
> complexity of core controllers keeps growing.
>
> Please feel free to open issues or even better send PRs with improvement that can make this documentation 
> even more valuable for the readers that will follow you.

### Rules

- Write reentrant reconcilers that observe the current state instead of relying on what a previous reconcile did
  ([details](#controller-reentrancy)).
- Create a `patch.NewHelper` at the start of the reconcile and patch the object and its status in a deferred call.
- After a successful write, call `DeferNextReconcileUntilCacheUpToDate`; call `ClearConsistencyStore` when the object
  is not found and before removing its finalizer ([details](#stale-cache-mitigation)).
- Every return must have a clear trigger for the next reconcile: a watch, a requeue, or a returned error
  ([details](#reconcile-results-requeues-and-errors)).
- Cache and client configuration (watched types, `ByObject` selectors, `DynamicCache` object types, the secret
  caching client) lives in `<provider>/setup` and is shared by `main.go` and the envtest suites; change it there,
  not in `main.go` or a `suite_test.go`.

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

### Stale cache mitigation

Controllers read from an informer cache that can lag behind the writes they just made. To avoid acting on stale data
(e.g. creating a second Machine because the first one is not in the cache yet), core controllers are built with
`util/controller` and use its `Controller` interface:

- After a successful create, update or delete, call `DeferNextReconcileUntilCacheUpToDate`, passing the reconciled
  object, the GVK of the written object and its resourceVersion. The next reconcile of the reconciled object is
  deferred until the cache has observed that resourceVersion. Only call it for kinds the controller has an informer for.
- When the reconciled object is not found, and before removing its finalizer, call `ClearConsistencyStore`.

```go
if err := r.Client.Create(ctx, machine); err != nil {
	return pkgerrors.Wrapf(err, "failed to create Machine %s", klog.KObj(machine))
}
r.controller.DeferNextReconcileUntilCacheUpToDate(machineSet, capicontrollerutil.StructuredObject(clusterv1.GroupVersion, "Machine"), machine.ResourceVersion)
```

See `core/reconcilers/machineset` for a complete example, including tests using the fake controller from
`util/controller/controller_fake.go`.

### Reconcile results (requeues and errors)

A reconcile can complete with one of the following results:

- done, e.g. `return ctrl.Result{}, nil`: the next reconcile is triggered by a watch event or the resync period.
- requeue after a delay, e.g. `return ctrl.Result{RequeueAfter: 30 * time.Second}, nil`.
- error, e.g. `return ctrl.Result{}, err`: the request is requeued with exponential backoff. Don't set `RequeueAfter`
  together with an error: controller-runtime ignores the result when the error is not nil.

Before adding a new return or a new error path, make sure you can answer:

- What triggers the next reconcile after this return (a watch on a related object, `RequeueAfter`, or a returned error)?
- How is a failed or timed-out reconcile retried?
- What happens on partial failure, NotFound and stale cache?
- Can the controller get stuck, e.g. waiting for a long-running operation that never completes?
- Can a change in defaulting (e.g. a feature gate) cause a rollout of existing Machines?

Also:

- Return errors instead of swallowing them, also for branches that "should never happen", and check `err` before
  using a result.
- Error messages must be specific and use the real Kind and `klog.KObj`/`klog.KRef`, e.g.
  `failed to delete InfraMachine` is too generic, `failed to delete DockerMachine default/md-0-abcde` is not.
