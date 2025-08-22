# Initialize a repository and the provider's API types

## Create a repository

```bash
mkdir cluster-api-provider-mailgun
cd src/sigs.k8s.io/cluster-api-provider-mailgun
git init
```

You'll then need to set up [go modules][gomod]

```bash
go mod init github.com/liztio/cluster-api-provider-mailgun
```
```bash
go: creating new go.mod: module github.com/liztio/cluster-api-provider-mailgun
```
[gomod]: https://github.com/golang/go/wiki/Modules#how-to-define-a-module

## Generate controller scaffolding

```bash
kubebuilder init --domain cluster.x-k8s.io
```

`kubebuilder init` will create the basic repository layout, including a simple containerized manager.
It will also initialize the external go libraries that will be required to build your project.

A few considerations about `--domain cluster.x-k8s.io`: 

Every Kubernetes resource has a *Group*, *Version* and *Kind* that uniquely identifies it.

The resource *Group* is similar to package in a language; it disambiguates different APIs that may happen to have identically named *Kind*s.
Groups often contain a domain name, such as k8s.io. 
The domain for Cluster API resources is `cluster.x-k8s.io`, and infrastructure providers generally use `infrastructure.cluster.x-k8s.io`.

Commit your changes so far:

```bash
git commit -m "Generate scaffolding."
```

## Generate API types for Clusters and Machines

A Cluster API infrastructure provider usually has two main API types, one modeling the infrastructure to get the
Cluster working (e.g. LoadBalancer), and one modeling the infrastructure for one machine/VM.

When creating an API, the resource *Kind* should be the name of the objects we'll be creating and modifying.
In this case it's `MailgunMachine` and `MailgunCluster`.

The resource *Version* defines the stability of the API and its backward compatibility guarantees.
Examples include v1alpha1, v1beta1, v1, etc. and are governed by the Kubernetes API Deprecation Policy [^1].
Your provider should expect to abide by the same policies.

Also, please note that the API version of Cluster API and the version of your provider do not need to be in sync.
Instead, prefer choosing a version that matches the stability of the provider API and its backward compatibility guarantees.

[^1]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

Once *Kind* and *Version*, are defined, you can run.

```bash
kubebuilder create api --group infrastructure --version v1alpha1 --kind MailgunCluster
kubebuilder create api --group infrastructure --version v1alpha1 --kind MailgunMachine
```

Here you will be asked if you want to generate resources and corresponding reconciler in the controller.
You'll want both of them (you are going to need them later in the guide):

```bash
Create Resource under pkg/apis [y/n]?
y
Create Controller under pkg/controller [y/n]?
y
```

### Add Status subresource

The [status subresource][status] lets Spec and Status requests for custom resources be addressed separately so requests don't conflict with each other.
It also lets you split RBAC rules between Spec and Status. You will have to [manually enable it in Kubebuilder][kbstatus].

Add the `subresource:status` annotation to your `<provider>cluster_types.go` `<provider>machine_types.go`

```go
// +kubebuilder:subresource:status
// +kubebuilder:object:root=true

// MailgunCluster is the Schema for the mailgunclusters API
type MailgunCluster struct {
```

```go
// +kubebuilder:subresource:status
// +kubebuilder:object:root=true

// MailgunMachine is the Schema for the mailgunmachines API
type MailgunMachine struct {
```

And regenerate the CRDs:
```bash
make manifests
```

Commit your changes

```bash
git add .
git commit -m "Generate Cluster and Machine resources."
```

[status]:  https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definitions/#status-subresource
[kbstatus]: https://book.kubebuilder.io/reference/generating-crd.html?highlight=status#status

### Apply further customizations

The cluster API CRDs should be further customized, please refer to [provider contracts](../contracts/overview.md).
