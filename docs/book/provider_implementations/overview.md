# Overview

In order to demonstrate how to develop a new Cluster API provider we will use 
`kubebuilder` to create an example provider. For more information on `kubebuilder`
and CRDs in general we highly recommend reading the [Kubebuilder Book][kubebuilder-book].
Much of the information here was adapted directly from it.

## Prerequisites

- Install [`dep`][install-dep]
- Install [`kubectl`][kubectl-install]
- Install [`kustomize`][install-kustomize]
- Install [`kubebuilder`][install-kubebuilder]

### tl;dr

{% codegroup %}
```bash::MacOS
# Install kubectl
brew install kubernetes-cli

# Install minikube
curl -Lo minikube https://storage.googleapis.com/minikube/releases/v0.30.0/minikube-darwin-amd64 && \
chmod +x minikube && \
sudo cp minikube /usr/local/bin/ && \
rm minikube

# Install kustomize
brew install kustomize
```
```bash::Linux
# Install kubectl
KUBECTL_VERSION=$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)
curl -LO https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl

# Install minikube
curl -Lo minikube https://storage.googleapis.com/minikube/releases/v0.30.0/minikube-linux-amd64 && \
chmod +x minikube && \
sudo cp minikube /usr/local/bin/ && \
rm minikube

# Install kustomize
OS_TYPE=linux
curl -s https://api.github.com/repos/kubernetes-sigs/kustomize/releases/latest |\
  grep browser_download |\
  grep ${OS_TYPE} |\
  cut -d '"' -f 4 |\
  xargs curl -O -L
mv kustomize_*_${OS_TYPE}_amd64 /usr/local/bin/kustomize
chmod u+x /usr/local/bin/kustomize
```
{% endcodegroup %}

[kubebuilder-book]: https://book.kubebuilder.io/
[install-dep]: https://github.com/golang/dep/blob/master/docs/installation.md
[kubectl-install]: http://kubernetes.io/docs/user-guide/prereqs/
[install-kustomize]: https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md
[install-kubebuilder]: https://book.kubebuilder.io/getting_started/installation_and_setup.html
