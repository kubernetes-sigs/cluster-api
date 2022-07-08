# Webhooks

Cluster API provides support for three kinds of webhooks: validating webhooks, defaulting webhook and conversion webhooks. 

## Validating webhooks
Validating webhooks are an implementation of a [Kubernetes validating webhook](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#validatingadmissionwebhook). A validating webhook allows developers to test whether values supplied by users are valid. e.g. [the Cluster webhook] ensures the Infrastructure reference supplied at the Cluster's `.spec.infrastructureRef` is in the same namespace as the Cluster itself and rejects the object creation or update if not.

<aside class="note">

<h1> ClusterClass and managed topology support in validating webhooks </h1>

Validating webhooks implemented for a `InfrastructureMachineTemplate` or `BootstrapConfigTemplate` resource
are required to not block due to immutability checks when the controller for managed
topology and ClusterClass does [Server Side Apply] dry-run requests.
[Server Side Apply] implementation in ClusterClass and managed topologies requires to dry-run changes on templates.
If infrastructure or bootstrap providers have implemented immutability checks in their InfrastructureMachineTemplate
or BootstrapConfigTemplate webhooks, it is required to implement the following changes in order to prevent dry-run
to return errors. The implementation requires sigs.k8s.io/controller-runtime in version >= v0.12.3.
In order to do so it is required to use a controller runtime CustomValidator.
This will allow to skip the immutability check only when the topology controller is dry running while preserving the
validation behavior for all other cases.

See [the DockerMachineTemplate webhook] as a reference for a compatible implementation.

[Server Side Apply]: https://kubernetes.io/docs/reference/using-api/server-side-apply/
[the DockerMachineTemplate webhook]: https://github.com/kubernetes-sigs/cluster-api/blob/main/test/infrastructure/docker/api/v1beta1/dockermachinetemplate_webhook.go

</aside>

## Defaulting webhooks
Defaulting webhooks are an implementation of a [Kubernetes mutating webhook](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#mutatingadmissionwebhook). A defaulting webhook allows developers to set default values for a type before they are placed in the Kubernetes data store. e.g. [the Cluster webhook] will set the Infrastructure reference namespace to equal the Cluster namespace if `.spec.infrastructureRef.namespace` is empty.

## Conversion webhooks
Conversion webhooks are what allow Cluster API to work with multiple API types without requiring different versions. It does this by converting the incoming version to a `Hub` version which is used internally by the controllers. To read more about conversion see the [Kubebuilder documentation](https://book.kubebuilder.io/multiversion-tutorial/conversion.html)

For a walkthrough on implementing conversion webhooks see the video in the [Developer Guide](../guide.md#videos-explaining-capi-architecture-and-code-walkthroughs)

## Implementing webhooks with Controller Runtime and Kubebuilder
The webhooks in Cluster API are offered through tools in Controller Runtime and Kubebuilder. The webhooks implement interfaces defined in Controller Runtime, while generation of manifests can be done using Kubebuilder.

For information on how to create webhooks [refer to the Kubebuilder book](https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation.html).


Webhook manifests are generated using Kubebuilder in Cluster API. This is done by adding tags to the webhook implementation in the codebase. Below, for example, are the tags on the [the Cluster webhook]:

```go

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-cluster-x-k8s-io-v1beta1-cluster,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusters,versions=v1beta1,name=validation.cluster.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-cluster,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusters,versions=v1beta1,name=default.cluster.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// Cluster implements a validating and defaulting webhook for Cluster.
type Cluster struct {
    Client client.Reader
}
```

A detailed guide on the purpose of each of these tags is [here](https://book.kubebuilder.io/reference/markers/webhook.html).

<!-- links -->
[the Cluster webhook]: https://github.com/kubernetes-sigs/cluster-api/blob/main/internal/webhooks/cluster.go
