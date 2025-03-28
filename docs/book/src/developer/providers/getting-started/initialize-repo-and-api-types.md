# Initialize a repository and the provider's API types

## Create a repository

```bash
mkdir -p src/sigs.k8s.io/cluster-api-provider-mailgun
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
git add .
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

And regenerate the CRDs:
```bash
make manifests
```

Commit your changes

```bash
git add .
git commit -m "Generate Cluster and Machine resources."
```

### Apply further customizations

The cluster API CRDs should be further customized, please refer to [provider contracts](../contracts/overview.md).
