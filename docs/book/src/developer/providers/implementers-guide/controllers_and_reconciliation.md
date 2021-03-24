# Controllers and Reconciliation

From the [kubebuilder book][controller]:

> Controllers are the core of Kubernetes, and of any operator.
>
> It’s a controller’s job to ensure that, for any given object, the actual state of the world (both the cluster state, and potentially external state like running containers for Kubelet or loadbalancers for a cloud provider) matches the desired state in the object.
> Each controller focuses on one root Kind, but may interact with other Kinds.
>
> We call this process reconciling.

[controller]: https://book.kubebuilder.io/cronjob-tutorial/controller-overview.html#whats-in-a-controller

Right now, we can create objects in our API but we won't do anything about it. Let's fix that.

# Let's see the Code

Kubebuilder has created our first controller in `controllers/mailguncluster_controller.go`. Let's take a look at what got generated:

```go
// MailgunClusterReconciler reconciles a MailgunCluster object
type MailgunClusterReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=mailgunclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=mailgunclusters/status,verbs=get;update;patch

func (r *MailgunClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("mailguncluster", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}
```

## RBAC Roles

Those `// +kubebuilder...` lines tell kubebuilder to generate [RBAC] roles so the manager we're writing can access its own managed resources.
We also need to add roles that will let it retrieve (but not modify) Cluster API objects.
So we'll add another annotation for that:

```
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=mailgunclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=mailgunclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
```

Make sure to add the annotation to both `MailgunClusterReconciler` and `MailgunMachineReconciler`.

Regenerate the RBAC roles after you are done:

```
make manifests
```


[RBAC]: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#role-and-clusterrole

## State

Let's focus on that `struct` first.
First, a word of warning: no guarantees are made about parallel access, both on one machine or multiple machines.
That means you should not store any important state in memory: if you need it, write it into a Kubernetes object and store it.

We're going to be sending mail, so let's add a few extra fields:

```go
// MailgunClusterReconciler reconciles a MailgunCluster object
type MailgunClusterReconciler struct {
	client.Client
	Log       logr.Logger
	Mailgun   mailgun.Mailgun
	Recipient string
}
```

## Reconciliation

Now it's time for our Reconcile function.
Reconcile is only passed a name, not an object, so let's retrieve ours.

Here's a naive example:

```
func (r *MailgunClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("mailguncluster", req.NamespacedName)

	var cluster infrastructurev1alpha3.MailgunCluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
```

By returning an error, we request that our controller will get `Reconcile()` called again.
That may not always be what we want - what if the object's been deleted? So let's check that:

```
var cluster infrastructurev1alpha3.MailgunCluster
if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
    // 	import apierrors "k8s.io/apimachinery/pkg/api/errors"
    if apierrors.IsNotFound(err) {
        return ctrl.Result{}, nil
    }
    return ctrl.Result{}, err
}
```

Now, if this were any old `kubebuilder` project we'd be done, but in our case we have one more object to retrieve.
Cluster API splits a cluster into two objects: the [`Cluster` defined by Cluster API itself][cluster].
We'll want to retrieve that as well.
Luckily, cluster API [provides a helper for us][getowner].

```go
cluster, err := util.GetOwnerCluster(ctx, r.Client, &mg)
if err != nil {
    return ctrl.Result{}, err

}
```

### client-go versions
At the time this document was written, `kubebuilder` pulls `client-go` version `1.14.1` into `go.mod` (it looks like `k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible`).

If you encounter an error when compiling like:

```
../pkg/mod/k8s.io/client-go@v11.0.1-0.20190409021438-1a26190bd76a+incompatible/rest/request.go:598:31: not enough arguments in call to watch.NewStreamWatcher
    have (*versioned.Decoder)
    want (watch.Decoder, watch.Reporter)`
```

You may need to bump `client-go`. At time of writing, that means `1.15`, which looks like: `k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible`.

## The fun part

_More Documentation: [The Kubebuilder Book][book] has some excellent documentation on many things, including [how to write good controllers!][implement]_

[book]: https://book.kubebuilder.io/
[implement]: https://book.kubebuilder.io/cronjob-tutorial/controller-implementation.html

Now that we have our objects, it's time to do something with them!
This is where your provider really comes into its own.
In our case, let's try sending some mail:

```go
subject := fmt.Sprintf("[%s] New Cluster %s requested", mgCluster.Spec.Priority, cluster.Name)
body := fmt.Sprint("Hello! One cluster please.\n\n%s\n", mgCluster.Spec.Request)

msg := mailgun.NewMessage(mgCluster.Spec.Requester, subject, body, r.Recipient)
_, _, err = r.Mailgun.Send(msg)
if err != nil {
    return ctrl.Result{}, err
}
```

## Idempotency

But wait, this isn't quite right.
`Reconcile()` gets called periodically for updates, and any time any updates are made.
That would mean we're potentially sending an email every few minutes!
This is an important thing about controllers: they need to be [*idempotent*][idempotent].

So in our case, we'll store the result of sending a message, and then check to see if we've sent one before.

```go
if mgCluster.Status.MessageID != nil {
    // We already sent a message, so skip reconciliation
    return ctrl.Result{}, nil
}

subject := fmt.Sprintf("[%s] New Cluster %s requested", mgCluster.Spec.Priority, cluster.Name)
body := fmt.Sprintf("Hello! One cluster please.\n\n%s\n", mgCluster.Spec.Request)

msg := mailgun.NewMessage(mgCluster.Spec.Requester, subject, body, r.Recipient)
_, msgID, err := r.Mailgun.Send(msg)
if err != nil {
    return ctrl.Result{}, err
}

// patch from sigs.k8s.io/cluster-api/util/patch
helper, err := patch.NewHelper(&mgCluster, r.Client)
if err != nil {
    return ctrl.Result{}, err
}
mgCluster.Status.MessageID = &msgID
if err := helper.Patch(ctx, &mgCluster); err != nil {
    return ctrl.Result{}, errors.Wrapf(err, "couldn't patch cluster %q", mgCluster.Name)
}

return ctrl.Result{}, nil
```

[cluster]: https://godoc.org/sigs.k8s.io/cluster-api/api/v1alpha3#Cluster
[getowner]: https://godoc.org/sigs.k8s.io/cluster-api/util#GetOwnerMachine
[idempotent]: https://stackoverflow.com/questions/1077412/what-is-an-idempotent-operation

#### A note about the status

Usually, the `Status` field should only be values that can be _computed from existing state_.
Things like whether a machine is running can be retrieved from an API, and cluster status can be queried by a healthcheck.
The message ID is ephemeral, so it should properly go in the `Spec` part of the object.
Anything that can't be recreated, either with some sort of deterministic generation method or by querying/observing actual state, needs to be in Spec.
This is to support proper disaster recovery of resources.
If you have a backup of your cluster and you want to restore it, Kubernetes doesn't let you restore both spec & status together.

We use the MessageID as a `Status` here to illustrate how one might issue status updates in a real application.

## Update `main.go` with your new fields

If you added fields to your reconciler, you'll need to update `main.go`.

Right now, it probably looks like this:

```go
if err = (&controllers.MailgunClusterReconciler{
    Client: mgr.GetClient(),
    Log:    ctrl.Log.WithName("controllers").WithName("MailgunCluster"),
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "MailgunCluster")
    os.Exit(1)
}
```

Let's add our configuration.
We're going to use environment variables for this:

```go
domain := os.Getenv("MAILGUN_DOMAIN")
if domain == "" {
    setupLog.Info("missing required env MAILGUN_DOMAIN")
    os.Exit(1)
}

apiKey := os.Getenv("MAILGUN_API_KEY")
if apiKey == "" {
    setupLog.Info("missing required env MAILGUN_API_KEY")
    os.Exit(1)
}

recipient := os.Getenv("MAIL_RECIPIENT")
if recipient == "" {
    setupLog.Info("missing required env MAIL_RECIPIENT")
    os.Exit(1)
}

mg := mailgun.NewMailgun(domain, apiKey)

if err = (&controllers.MailgunClusterReconciler{
    Client:    mgr.GetClient(),
    Log:       ctrl.Log.WithName("controllers").WithName("MailgunCluster"),
    Mailgun:   mg,
    Recipient: recipient,
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "MailgunCluster")
    os.Exit(1)
}
```

If you have some other state, you'll want to initialize it here!
