# Overview

In order to demonstrate how to develop a new Cluster API provider we will use
`kubebuilder` to create an example provider. For more information on `kubebuilder`
and CRDs in general we highly recommend reading the [Kubebuilder Book][kubebuilder-book].
Much of the information here was adapted directly from it.

This is an _infrastructure_ provider - tasked with managing provider-specific resources for clusters and machines.
There are also [bootstrap providers][bootstrap], which turn machines into Kubernetes nodes.

[bootstrap]: ../../../reference/providers.md#bootstrap

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
[kubectl-install]: http://kubernetes.io/docs/user-guide/prereqs/
[install-kustomize]: https://kubectl.docs.kubernetes.io/installation/kustomize/
[install-kubebuilder]:  https://book.kubebuilder.io/quick-start.html#installation
