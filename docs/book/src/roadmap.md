# Cluster API Roadmap

This roadmap is a constant work in progress, subject to frequent revision. Dates are approximations.

## v0.4 (v1alpha4) ~ Q1 2021

|Area|Description|Issue/Proposal|
|--|--|--|
|Operator, Providers|Move to a single manager watching all namespaces for each provider|[#3042](https://github.com/kubernetes-sigs/cluster-api/issues/3042)
|Clusterctl|Redefine the scope of clusterctl move|[#3354](https://github.com/kubernetes-sigs/cluster-api/issues/3354)
|Extensibility|Support pluggable machine load balancers|[#1250](https://github.com/kubernetes-sigs/cluster-api/issues/1250)|
|Core Improvements|Move away from corev1.ObjectReference|[#2318](https://github.com/kubernetes-sigs/cluster-api/issues/2318)|
|Dependency|Kubeadm v1beta2 types and support|[#2769](https://github.com/kubernetes-sigs/cluster-api/issues/2769)|
|UX, Bootstrap|Machine bootstrap failure detection with sentinel files|[#3716](https://github.com/kubernetes-sigs/cluster-api/issues/3716)|
|Operator|Management cluster operator|[#3427](https://github.com/kubernetes-sigs/cluster-api/issues/3427)|
|Features, KubeadmControlPlane|Support for MachineHealthCheck based remediation|[#2976](https://github.com/kubernetes-sigs/cluster-api/issues/2976)|
|Features, KubeadmControlPlane|KubeadmControlPlane Spec should be fully mutable|[#2083](https://github.com/kubernetes-sigs/cluster-api/issues/2083)|
|Testing, Clusterctl|Implement a new E2E test for verifying clusterctl upgrades|[#3690](https://github.com/kubernetes-sigs/cluster-api/issues/3690)|
|UX, Kubeadm|Insulate users from kubeadm API version changes|[#2769](https://github.com/kubernetes-sigs/cluster-api/issues/2769)|
|Cleanup|Generate v1alpha4 types, remove support for v1alpha2|[#3428](https://github.com/kubernetes-sigs/cluster-api/issues/3428)|
|Cleanup|Remove Status.Phase and other boolean fields in favor of conditions in all types|[#3153](https://github.com/kubernetes-sigs/cluster-api/issues/3153)|
|Cleanup|Deprecate Status.{FailureMessage, FailureReason} in favor of conditions in types and contracts|[#3692](https://github.com/kubernetes-sigs/cluster-api/issues/3692)|
|UX, Clusterctl|Support plugins in clusterctl to make provider-specific setup easier|[#3255](https://github.com/kubernetes-sigs/cluster-api/issues/3255)|
|Tooling, Visibility|Distributed Tracing|[#3760](https://github.com/kubernetes-sigs/cluster-api/issues/3760)|
|Bootstrap Improvements|Support composition of bootstrapping of kubeadm, cloud-init/ignition/talos/etc... and secrets transport|[#3761](https://github.com/kubernetes-sigs/cluster-api/issues/3761)|
|Bootstrap Improvements|Add ignition support experiment as a bootstrap provider|[#3430](https://github.com/kubernetes-sigs/cluster-api/issues/3430)|
|Integration|Autoscaler scale to and from zero|[#2530](https://github.com/kubernetes-sigs/cluster-api/issues/2530)|
|API, Contracts|Support multiple kubeconfigs for a provider|[#3661](https://github.com/kubernetes-sigs/cluster-api/issues/3661)|
|API, Networking|Http proxy support for egress traffic|[#3751](https://github.com/kubernetes-sigs/cluster-api/issues/3751)|
|Features, Integration|Windows support for worker nodes|[#3616](https://github.com/kubernetes-sigs/cluster-api/pull/3616)|
|Clusterctl, UX|Provide "at glance" view of cluster conditions|[#3802](https://github.com/kubernetes-sigs/cluster-api/issues/3802)|

## v1beta1/v1 ~ TBA

|Area|Description|Issue/Proposal|
|--|--|--|
|Maturity, Feedback|Submit the project for Kubernetes API Review|TBA|

---

## Backlog

> Items within this category have been identified as potential candidates for the project
> and can be moved up into a milestone if there is enough interest.

|Area|Description|Issue/Proposal|
|--|--|--|
|Security|Machine attestation for secure kubelet registration|[#3762](https://github.com/kubernetes-sigs/cluster-api/issues/3762)|
|Conformance| Define Cluster API provider conformance|TBA|
|Core Improvements|Pluggable MachineDeployment upgrade strategies|[#1754](https://github.com/kubernetes-sigs/cluster-api/issues/1754)|
|UX|Simplified cluster creation experience|[#1227](https://github.com/kubernetes-sigs/cluster-api/issues/1227)|
|Bootstrap, Infrastructure|Document approaches for infrastructure providers to consider for securing sensitive bootstrap data|[#1739](https://github.com/kubernetes-sigs/cluster-api/issues/1739)|
