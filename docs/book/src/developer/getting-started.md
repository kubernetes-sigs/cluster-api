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

On MacOS systems using [Lima](https://github.com/lima-vm/lima) is a viable alternative to Docker Desktop.

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
[provider-contract]: providers/contracts/clusterctl.md

### Cert-Manager

You'll need to deploy [cert-manager] components on your [management cluster][mcluster], using `kubectl`

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml
```

Ensure the cert-manager webhook service is ready before creating the Cluster API components.

This can be done by following instructions for [manual verification](https://cert-manager.io/docs/installation/verify/#manual-verification)
from the [cert-manager] web site.
Note: make sure to follow instructions for the release of cert-manager you are installing.

[cert-manager]: https://github.com/cert-manager/cert-manager

## Development

## Option 1: Tilt

[Tilt][tilt] is a tool for quickly building, pushing, and reloading Docker containers as part of a Kubernetes deployment.
Many of the Cluster API engineers use it for quick iteration. Please see our [Tilt instructions] to get started.

[tilt]: https://tilt.dev
[capi-dev]: https://github.com/chuckha/capi-dev
[Tilt instructions]: core/tilt.md

## Option 2: The Old-fashioned way

```bash
# Build all the images
make docker-build

# Push images
make docker-push

# Apply the manifests
kustomize build config/default | ./hack/tools/bin/envsubst | kubectl apply -f -
kustomize build bootstrap/kubeadm/config/default | ./hack/tools/bin/envsubst | kubectl apply -f -
kustomize build controlplane/kubeadm/config/default | ./hack/tools/bin/envsubst | kubectl apply -f -
kustomize build test/infrastructure/docker/config/default | ./hack/tools/bin/envsubst | kubectl apply -f -
```

## Testing

Cluster API has a number of test suites available for you to run. Please visit the [testing][testing] page for more
information on each suite.

[testing]: core/testing.md

## That's it!

Now you can [create CAPI objects][qs]!
To test another iteration, you'll need to follow the steps to build, push, update the manifests, and apply.

[qs]: ../user/quick-start.md

## Videos explaining CAPI architecture and code walkthroughs

**CAPI components and architecture**

* [Simplified Experience Of Building Cluster API Provider In Multitenant Cloud - October 2022](https://www.youtube.com/watch?v=1oj9BuV2dzA)
* [Cluster API Intro and Deep Dive - May 2022 v1beta1](https://www.youtube.com/watch?v=9H8flXm_lKk)
* [Cluster API Deep Dive - Dec 2020 v1alpha3](https://youtu.be/npFO5Fixqcc)
* [Cluster API Deep Dive - Sept 2020 v1alpha3](https://youtu.be/9SfuQQeeK6Q)
* [Declarative Kubernetes Clusters with Cluster API - Oct 2020 v1alpha3](https://youtu.be/i6OWn2zRsZg)
* [TGI Kubernetes 178: ClusterAPI - ClusterClass & Managed Topologies - Dec 2020 v1beta1](https://www.youtube.com/watch?v=U9CDND0nzRI&list=PL7bmigfV0EqQzxcNpmcdTJ9eFRPBe-iZa&index=5)

**Additional ClusterAPI KubeCon talks**

* [SIG Cluster Lifecycle Intro & Future - November 2023](https://www.youtube.com/watch?v=MM0YPhIel2M)
* [Cluster API Deep Dive: Improving Performance up to 2k Clusters - November 2023](https://www.youtube.com/watch?v=bRPfmviTi3s)
* [Leveraging Cluster-API for Production-Ready Multi-Regional Infrastructures - November 2023](https://www.youtube.com/watch?v=BDjhGEVJ0Gs)
* [The Stars Look Very Different Today”: Kubernetes and Cloud Native at the SKA Observatory - November 2023](https://www.youtube.com/watch?v=quW8FbW1fVM)
* [15,000 Minecraft Players Vs One K8s Cluster. Who Wins? - November 2023](https://www.youtube.com/watch?v=4YNp2vb9NTA)
* [Cluster API Providers: Intro, Deep Dive, and Community! - April 2023](https://www.youtube.com/watch?v=QA4OhqLKJn4)
* [Ephemeral Clusters as a Service with ClusterAPI and GitOps - April 2023](https://www.youtube.com/watch?v=cXIo8C7yWvg)
* [The Power of Self-Managing Clusters - April 2023](https://www.youtube.com/watch?v=tNUH_8MFyTc)
* [How to Turn Release Management from Duty to Fun - April 2023](https://www.youtube.com/watch?v=sgP3tyGJ5tQ)
* [Tilt Your World! Lessons Learned in Improving Dev Productivity with Tilt - April 2023](https://www.youtube.com/watch?v=h6llT5Bg97g)
* [How Adobe Planned For Scale With Argo CD, Cluster API, And VCluster - October 2022](https://www.youtube.com/watch?v=p8BluR5WT5w)
* [Bare-Metal Chronicles: Intertwinement Of Tinkerbell, Cluster API And GitOps - October 2022](https://www.youtube.com/watch?v=NCFUUjTw6hA)
* [Running Isolated VirtualClusters With Kata & Cluster API - October 2022](https://www.youtube.com/watch?v=T6w3YrExorY)
* [SIG Cluster Lifecycle Intro - October 2022](https://www.youtube.com/watch?v=0Zo0cWYU0fM)
* [How to Migrate 700 Kubernetes Clusters to Cluster API with Zero Downtime - May 2022](https://www.youtube.com/watch?v=KzYV-fJ_wH0)
* [Build Your Own Cluster API Provider the Easy Way - May 2022](https://www.youtube.com/watch?v=HSdgmcAAXa8)

**Tutorials**

* [kubectl Create Cluster: Production-ready Kubernetes with Cluster API 1.0 - October 2022](https://kccncna2022.sched.com/event/1BZDs)

  [Source code](https://github.com/ykakarap/kubecon-na-22-capi-lab)
* [So You Want To Develop a Cluster API Provider? - October 2022](https://kccncna2022.sched.com/event/182Ha)

  [Source code](https://capi-samples.github.io/kubecon-na-2022-tutorial/)

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
