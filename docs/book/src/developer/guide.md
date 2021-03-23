# Developer Guide

## Pieces of Cluster API

Cluster API is made up of many components, all of which need to be running for correct operation.
For example, if you wanted to use Cluster API [with AWS][capa], you'd need to install both the [cluster-api manager][capi-manager] and the [aws manager][capa-manager].

Cluster API includes a built-in provisioner, [Docker], that's suitable for using for testing and development.
This guide will walk you through getting that daemon, known as [CAPD], up and running.

Other providers may have additional steps you need to follow to get up and running.

[capa]: https://github.com/kubernetes-sigs/cluster-api-provider-aws
[capi-manager]: https://github.com/kubernetes-sigs/cluster-api/blob/master/main.go
[capa-manager]: https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/master/main.go
[Docker]: https://github.com/kubernetes-sigs/cluster-api/tree/master/test/infrastructure/docker

## Prerequisites

### Docker

Iterating on the cluster API involves repeatedly building Docker containers.
You'll need the [docker daemon][docker] v19.03 or newer available.

[docker]: https://docs.docker.com/install/

### A Cluster

You'll likely want an existing cluster as your [management cluster][mcluster].
The easiest way to do this is with [kind] v0.9 or newer, as explained in the quick start.

Make sure your cluster is set as the default for `kubectl`.
If it's not, you will need to modify subsequent `kubectl` commands below.

[clusterctl]: https://github.com/kubernetes-sigs/cluster-api/tree/master/cmd/clusterctl
[pivot]: https://cluster-api.sigs.k8s.io/reference/glossary.html#pivot
[mcluster]: https://cluster-api.sigs.k8s.io/reference/glossary.html#management-cluster
[kind]: https://github.com/kubernetes-sigs/kind

### A container registry

If you're using [kind], you'll need a way to push your images to a registry to they can be pulled.
You can instead [side-load] all images, but the registry workflow is lower-friction.

Most users test with [GCR], but you could also use something like [Docker Hub][hub].
If you choose not to use GCR, you'll need to set the `REGISTRY` environment variable.

[side-load]: https://kind.sigs.k8s.io/docs/user/quick-start/#loading-an-image-into-your-cluster
[GCR]: https://cloud.google.com/container-registry/
[hub]: https://hub.docker.com/

### Kustomize

You'll need to [install `kustomize`][kustomize].
There is a version of `kustomize` built into kubectl, but it does not have all the features of `kustomize` v3 and will not work.

[kustomize]: https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md

### Kubebuilder

You'll need to [install `kubebuilder`][kubebuilder].

[kubebuilder]: https://book.kubebuilder.io/quick-start.html#installation

### Envsubst

You'll need [`drone/envsubst`][envsubst] or similar to handle clusterctl var replacement. `envsubst` in GNU gettext package is insufficient and we've noticed some parsing differences, e.g. when parsing a YAML configuration file containing variables with default values. Note: drone/envsubst releases v1.0.2 and earlier do not have the binary packaged under cmd/envsubst. It is available in Go psuedo-version `v1.0.3-0.20200709231038-aa43e1c1a629`

We provide a make target to generate the `envsubst` binary if desired. See the [provider contract][provider-contract] for more details about how clusterctl uses variables.

```
make envsubst
```

The generated binary can be found at ./hack/tools/bin/envsubst

[envsubst]: https://github.com/drone/envsubst
[provider-contract]: ./../clusterctl/provider-contract.md

### Cert-Manager

You'll need to deploy [cert-manager] components on your [management cluster][mcluster], using `kubectl`

```bash
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.1.0/cert-manager.yaml
```

Ensure the cert-manager webhook service is ready before creating the Cluster API components. 

This can be done by running: 

```bash
kubectl wait --for=condition=Available --timeout=300s apiservice v1beta1.webhook.cert-manager.io
```

[cert-manager]: https://github.com/jetstack/cert-manager

## Development

## Option 1: Tilt

[Tilt][tilt] is a tool for quickly building, pushing, and reloading Docker containers as part of a Kubernetes deployment.
Many of the Cluster API engineers use it for quick iteration. Please see our [Tilt instructions] to get started.

[tilt]: https://tilt.dev
[capi-dev]: https://github.com/chuckha/capi-dev
[Tilt instructions]: ../developer/tilt.md

## Option 2: The Old-fashioned way

### Building everything

You'll need to build two docker images, one for Cluster API itself and one for the Docker provider (CAPD).

```
make docker-build
make -C test/infrastructure/docker docker-build
```

### Push both images

```shell
$ make docker-push
docker push gcr.io/cluster-api-242700/cluster-api-controller-amd64:dev
The push refers to repository [gcr.io/cluster-api-242700/cluster-api-controller-amd64]
90a39583ad5f: Layer already exists
932da5156413: Layer already exists
dev: digest: sha256:263262cfbabd3d1add68172a5a1d141f6481a2bc443672ce80778dc122ee6234 size: 739
$ make -C test/infrastructure/docker docker-push
make: Entering directory '/home/liz/src/sigs.k8s.io/cluster-api/test/infrastructure/docker'
docker push gcr.io/cluster-api-242700/manager:dev
The push refers to repository [gcr.io/cluster-api-242700/manager]
5b1e744b2bae: Pushed
932da5156413: Layer already exists
dev: digest: sha256:35670a049372ae063dad910c267a4450758a139c4deb248c04c3198865589ab2 size: 739
make: Leaving directory '/home/liz/src/sigs.k8s.io/cluster-api/test/infrastructure/docker'
```

Make a note of the URLs and the digests. You'll need them for the next step. In this case, they're...

`gcr.io/cluster-api-242700/manager@sha256:35670a049372ae063dad910c267a4450758a139c4deb248c04c3198865589ab2`

and

`gcr.io/cluster-api-242700/cluster-api-controller-amd64@sha256:263262cfbabd3d1add68172a5a1d141f6481a2bc443672ce80778dc122ee6234`

### Edit the manifests

```
$EDITOR config/manager/manager_image_patch.yaml
$EDITOR test/infrastructure/docker/config/manager/manager_image_patch.yaml
```

In both cases, change the `- image:` url to the digest URL mentioned above:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: system
spec:
  template:
    spec:
      containers:
      - image: gcr.io/cluster-api-242700/manager@sha256:35670a049372ae063dad910c267a4450758a139c4deb248c04c3198865589ab2`
        name: manager
```

### Apply the manifests
```shell
$ kustomize build config/ | ./hack/tools/bin/envsubst | kubectl apply -f -
namespace/capi-system configured
customresourcedefinition.apiextensions.k8s.io/clusters.cluster.x-k8s.io configured
customresourcedefinition.apiextensions.k8s.io/kubeadmconfigs.bootstrap.cluster.x-k8s.io configured
customresourcedefinition.apiextensions.k8s.io/kubeadmconfigtemplates.bootstrap.cluster.x-k8s.io configured
customresourcedefinition.apiextensions.k8s.io/machinedeployments.cluster.x-k8s.io configured
customresourcedefinition.apiextensions.k8s.io/machines.cluster.x-k8s.io configured
customresourcedefinition.apiextensions.k8s.io/machinesets.cluster.x-k8s.io configured
role.rbac.authorization.k8s.io/capi-leader-election-role configured
clusterrole.rbac.authorization.k8s.io/capi-manager-role configured
rolebinding.rbac.authorization.k8s.io/capi-leader-election-rolebinding configured
clusterrolebinding.rbac.authorization.k8s.io/capi-manager-rolebinding configured
deployment.apps/capi-controller-manager created

$ kustomize build test/infrastructure/docker/config | ./hack/tools/bin/envsubst | kubectl apply -f -
namespace/capd-system configured
customresourcedefinition.apiextensions.k8s.io/dockerclusters.infrastructure.cluster.x-k8s.io configured
customresourcedefinition.apiextensions.k8s.io/dockermachines.infrastructure.cluster.x-k8s.io configured
customresourcedefinition.apiextensions.k8s.io/dockermachinetemplates.infrastructure.cluster.x-k8s.io configured
role.rbac.authorization.k8s.io/capd-leader-election-role configured
clusterrole.rbac.authorization.k8s.io/capd-manager-role configured
clusterrole.rbac.authorization.k8s.io/capd-proxy-role configured
rolebinding.rbac.authorization.k8s.io/capd-leader-election-rolebinding configured
clusterrolebinding.rbac.authorization.k8s.io/capd-manager-rolebinding configured
clusterrolebinding.rbac.authorization.k8s.io/capd-proxy-rolebinding configured
service/capd-controller-manager-metrics-service created
deployment.apps/capd-controller-manager created
```

### Check the status of the clusters

```shell
$ kubectl get po -n capd-system
NAME                                       READY   STATUS    RESTARTS   AGE
capd-controller-manager-7568c55d65-ndpts   2/2     Running   0          71s
$ kubectl get po -n capi-system
NAME                                      READY   STATUS    RESTARTS   AGE
capi-controller-manager-bf9c6468c-d6msj   1/1     Running   0          2m9s
```

## Testing

Cluster API has a number of test suites available for you to run. Please visit the [testing][testing] page for more
information on each suite.

[testing]: ./testing.md

## That's it!

Now you can [create CAPI objects][qs]!
To test another iteration, you'll need to follow the steps to build, push, update the manifests, and apply.

[qs]: https://cluster-api.sigs.k8s.io/user/quick-start.html#usage
