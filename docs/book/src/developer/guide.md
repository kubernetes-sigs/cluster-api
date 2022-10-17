# Developer Guide

## Pieces of Cluster API

Cluster API is made up of many components, all of which need to be running for correct operation.
For example, if you wanted to use Cluster API [with AWS][capa], you'd need to install both the [cluster-api manager][capi-manager] and the [aws manager][capa-manager].

Cluster API includes a built-in provisioner, [Docker], that's suitable for using for testing and development.
This guide will walk you through getting that daemon, known as [CAPD], up and running.

Other providers may have additional steps you need to follow to get up and running.

[capa]: https://github.com/kubernetes-sigs/cluster-api-provider-aws
[capi-manager]: https://github.com/kubernetes-sigs/cluster-api/blob/main/main.go
[capa-manager]: https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/main/main.go
[Docker]: https://github.com/kubernetes-sigs/cluster-api/tree/main/test/infrastructure/docker
[CAPD]: https://github.com/kubernetes-sigs/cluster-api/blob/main/test/infrastructure/docker/README.md

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

[mcluster]: ../reference/glossary.md#management-cluster
[kind]: https://github.com/kubernetes-sigs/kind

### A container registry

If you're using [kind], you'll need a way to push your images to a registry so they can be pulled.
You can instead [side-load] all images, but the registry workflow is lower-friction.

Most users test with [GCR], but you could also use something like [Docker Hub][hub].
If you choose not to use GCR, you'll need to set the `REGISTRY` environment variable.

[side-load]: https://kind.sigs.k8s.io/docs/user/quick-start/#loading-an-image-into-your-cluster
[GCR]: https://cloud.google.com/container-registry/
[hub]: https://hub.docker.com/

### Kustomize

You'll need to [install `kustomize`][kustomize].
There is a version of `kustomize` built into kubectl, but it does not have all the features of `kustomize` v3 and will not work.

[kustomize]: https://kubectl.docs.kubernetes.io/installation/kustomize/

### Kubebuilder

You'll need to [install `kubebuilder`][kubebuilder].

[kubebuilder]: https://book.kubebuilder.io/quick-start.html#installation

### Envsubst

You'll need [`envsubst`][envsubst] or similar to handle clusterctl var replacement. Note: drone/envsubst releases v1.0.2 and earlier do not have the binary packaged under cmd/envsubst. It is available in Go pseudo-version `v1.0.3-0.20200709231038-aa43e1c1a629`

We provide a make target to generate the `envsubst` binary if desired. See the [provider contract][provider-contract] for more details about how clusterctl uses variables.

```bash
make envsubst
```

The generated binary can be found at ./hack/tools/bin/envsubst

[envsubst]: https://github.com/drone/envsubst
[provider-contract]: ./../clusterctl/provider-contract.md

### Cert-Manager

You'll need to deploy [cert-manager] components on your [management cluster][mcluster], using `kubectl`

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.10.0/cert-manager.yaml
```

Ensure the cert-manager webhook service is ready before creating the Cluster API components.

This can be done by running:

```bash
kubectl wait --for=condition=Available --timeout=300s apiservice v1beta1.webhook.cert-manager.io
```

[cert-manager]: https://github.com/cert-manager/cert-manager

## Development

## Option 1: Tilt

[Tilt][tilt] is a tool for quickly building, pushing, and reloading Docker containers as part of a Kubernetes deployment.
Many of the Cluster API engineers use it for quick iteration. Please see our [Tilt instructions] to get started.

[tilt]: https://tilt.dev
[capi-dev]: https://github.com/chuckha/capi-dev
[Tilt instructions]: ../developer/tilt.md

## Option 2: The Old-fashioned way

```bash
# Build all the images
make docker-build

# Push images
make docker-push

# Apply the manifests
kustomize build config/default | ./hack/tools/bin/envsubst | kubectl apply -f -
kustomize build bootstrap/kubeadm/config | ./hack/tools/bin/envsubst | kubectl apply -f -
kustomize build controlplane/kubeadm/config | ./hack/tools/bin/envsubst | kubectl apply -f -
kustomize build test/infrastructure/docker/config/default | ./hack/tools/bin/envsubst | kubectl apply -f -
```

## Testing

Cluster API has a number of test suites available for you to run. Please visit the [testing][testing] page for more
information on each suite.

[testing]: ./testing.md

## That's it!

Now you can [create CAPI objects][qs]!
To test another iteration, you'll need to follow the steps to build, push, update the manifests, and apply.

[qs]: ../user/quick-start.md#usage

## Videos explaining CAPI architecture and code walkthroughs

**CAPI components and architecture**

* [Cluster API Intro and Deep Dive - May 2022 v1beta1](https://www.youtube.com/watch?v=9H8flXm_lKk)
* [Cluster API Deep Dive - Dec 2020 v1alpha3](https://youtu.be/npFO5Fixqcc)
* [Cluster API Deep Dive - Sept 2020 v1alpha3](https://youtu.be/9SfuQQeeK6Q)
* [Declarative Kubernetes Clusters with Cluster API - Oct 2020 v1alpha3](https://youtu.be/i6OWn2zRsZg)
* [TGI Kubernetes 178: ClusterAPI - ClusterClass & Managed Topologies - Dec 2020 v1beta1](https://www.youtube.com/watch?v=U9CDND0nzRI&list=PL7bmigfV0EqQzxcNpmcdTJ9eFRPBe-iZa&index=5)

**Additional ClusterAPI KubeCon talks**

* [How to Migrate 700 Kubernetes Clusters to Cluster API with Zero Downtime - May 2022](https://www.youtube.com/watch?v=KzYV-fJ_wH0)
* [Build Your Own Cluster API Provider the Easy Way - May 2022](https://www.youtube.com/watch?v=HSdgmcAAXa8)

**Code walkthroughs**

* [CAPD Deep Dive - March 2021 v1alpha4](https://youtu.be/67kEp471MPk)
* [API conversion code walkthrough - January 2022](https://www.youtube.com/watch?v=Mk14N4SelNk)

**Let's chat about ...**

We are currently hosting "Let's chat about ..." sessions where we are talking about topics relevant to 
contributors and users of the Cluster API project. For more details and an up-to-date list of recordings of past sessions please 
see [Let's chat about ...](https://github.com/kubernetes-sigs/cluster-api/discussions/6106).

* [Local CAPI development and debugging with Tilt (EMEA/Americas) - February 2022](https://www.youtube.com/watch?v=tEIRGmJahWs)
* [Local CAPI development and debugging with Tilt (APAC/EMEA) - February 2022](https://www.youtube.com/watch?v=CM-dotO2nSU)
* [Code structure & Makefile targets (EMEA/Americas) - February 2022](https://www.youtube.com/watch?v=_prbOnziCJw)
* [Code structure & Makefile targets (APAC/EMEA) - February 2022](https://www.youtube.com/watch?v=Y6Gws65H1tE)
