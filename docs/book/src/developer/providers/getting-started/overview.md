# Getting Started

This is a getting started guide to demonstrate how to develop a new Cluster API provider.

The guide focus on setting up a new project for implementing the provider and creating:
- API types and corresponding CustomResourceDefinition (CRD).
- Webhooks, responsible to default and validate above resources.
- Controllers, responsible of reconciling above resources.

We will use `kubebuilder` to create an example _infrastructure_ provider; for more information on `kubebuilder` 
and CRDs in general we highly recommend reading the [Kubebuilder Book][kubebuilder-book].
Much of the information here was adapted directly from it.

Also worth to notice that suggestion in this guide are only intended to help first time provider implementers to get started,
but this is not an exhaustive guide of all the intricacies of developing Kubernetes controllers. 
Please refer to  the [Kubebuilder Book][kubebuilder-book] and to [Cluster API videos and tutorials](../../getting-started.md#videos-explaining-capi-architecture-and-code-walkthroughs)
for more information.

If you already know how `kubebuilder` works, if you know how to write Kubernetes controllers, or if you are planning 
to use something different than `kubebuilder` to develop your own Cluster API provider, you can skip this guide entirely.

<aside class="note warning">

<h1>We need your help!</h1>

While we put a great effort in ensuring a good documentation for Cluster API, we also recognize that some
part of the documentation are more prone to become outdated.

Unfortunately, this guide is one of those parts, simply because Cluster API maintainers do not create new providers very often,
while things in Cluster API and in the Kubernetes ecosystem change fast.

This is why we need your help to identify outdated part of this guide as well as any improvement that can make it
even more valuable for the newcomers that will follow you.

</aside>

## Prerequisites

- Install [`kubectl`][kubectl-install]
- Install [`kustomize`][install-kustomize]
- Install [`kubebuilder`][install-kubebuilder]

### tl;dr

{{#tabs name:"kubectl and kustomize" tabs:"MacOS,Linux"}}
{{#tab MacOS}}

```bash
# Install kubectl
brew install kubernetes-cli

# Install kustomize
brew install kustomize

# Install Kubebuilder
brew install kubebuilder
```
{{#/tab }}
{{#tab Linux}}

```bash
# Install kubectl
KUBECTL_VERSION=$(curl -sfL https://dl.k8s.io/release/stable.txt)
curl -fLO https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl

# Install kustomize
curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"  | bash
chmod +x ./kustomize && sudo mv ./kustomize /usr/local/bin/kustomize

# Install Kubebuilder
curl -sLo kubebuilder https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)
chmod +x ./kubebuilder && sudo mv ./kubebuilder /usr/local/bin/kubebuilder
```

{{#/tab }}
{{#/tabs }}

[kubebuilder-book]: https://book.kubebuilder.io/
[kubectl-install]: https://kubernetes.io/docs/tasks/tools/#kubectl
[install-kustomize]: https://kubectl.docs.kubernetes.io/installation/kustomize/
[install-kubebuilder]:  https://book.kubebuilder.io/quick-start.html#installation
