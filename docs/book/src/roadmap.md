# Cluster API Roadmap

The roadmap is inspired by the project [goals](introduction.html#goals); it is a constant work in progress, 
subject to frequent revision. Dates are approximations.


## v1.2 ~ Q2 2022

> Items within this category have been selected as candidate for the release;
> actual implementation depends on contributors' bandwidth.

|Area|Description|Issue/Proposal|
|--|--|--|
|Security|Set up Cluster API on oss-fuzz|[#6059](https://github.com/kubernetes-sigs/cluster-api/issues/6059)|
|UX, Clusterctl|Explore alternatives to rate limited GitHub API for clusterctl|[#3982](https://github.com/kubernetes-sigs/cluster-api/issues/3982)|
|API, Contracts|Inconsistent provider behaviour with API server ports|[#5517](https://github.com/kubernetes-sigs/cluster-api/issues/5517)|
|API, Core Improvements|Represent Failure domains availability|[#4031](https://github.com/kubernetes-sigs/cluster-api/issues/4031)|
|Core Improvements|Support syncing Node labels |[#493](https://github.com/kubernetes-sigs/cluster-api/issues/493)|
|Core Improvements|Drain workload clusters of service Type=Loadbalancer on teardown|[#3075](https://github.com/kubernetes-sigs/cluster-api/issues/3075)|
|Core Improvements|Bubble up conditions for worker nodes|[#3486](https://github.com/kubernetes-sigs/cluster-api/issues/3486)|
|Core Improvements|Support multiple kubeconfigs for a provider|[#3661](https://github.com/kubernetes-sigs/cluster-api/issues/3661)|
|Core Improvements|Block on unsupported Kubernetes version|[#4321](https://github.com/kubernetes-sigs/cluster-api/issues/4321)|
|Core Improvements|Improve certificate management in Cluster API|[#5490](https://github.com/kubernetes-sigs/cluster-api/issues/5490)|
|Core Improvements|Add support for RolloutAfter to MachineDeployments|[#4536](https://github.com/kubernetes-sigs/cluster-api/issues/4536)|
|Core Improvements|Add support for RolloutAfter to ClusterClass|[#5218](https://github.com/kubernetes-sigs/cluster-api/issues/5218)|
|Core Improvements|MHC should provide support for checking Machine conditions|[#5450](https://github.com/kubernetes-sigs/cluster-api/issues/5450)|
|Core Improvements|MachinePool workers support in ClusterClass|[#5991](https://github.com/kubernetes-sigs/cluster-api/issues/5991)|
|Core Improvements|Allow using a ClusterClass across namespaces|[#5673](https://github.com/kubernetes-sigs/cluster-api/issues/5673)|
|Extensibility|Introduce the runtime SDK|[#6181](https://github.com/kubernetes-sigs/cluster-api/pull/6181)|
|Extensibility|Introduce topology mutation hooks| |
|Extensibility|Introduce lifecycle hooks for enabling addon orchestration|[#5491](https://github.com/kubernetes-sigs/cluster-api/issues/5491)|
|Extensibility|Continuous ClusterResourceSetStrategy|[#4807](https://github.com/kubernetes-sigs/cluster-api/issues/4807)|
|Visibility|Integrate with the Cluster API metrics project| |
|Visibility|Improve CAPI logs and ensure consistent with updated K8s guidelines| |

For a detailed view of all the issues in the milestone see [milestone v1.2](https://github.com/kubernetes-sigs/cluster-api/issues?q=is%3Aopen+is%3Aissue+milestone%3Av1.2). 

---

## Backlog

> Items within this category have been identified as potential candidates for the project
> and can be moved up into a milestone if there is enough interest/volounteers for implementation.

|Area|Description|Issue/Proposal|
|--|--|--|
|Security|Machine attestation for secure kubelet registration|[#3762](https://github.com/kubernetes-sigs/cluster-api/issues/3762)|
|UX, Clusterctl|Support plugins in clusterctl to make provider-specific setup easier|[#3255](https://github.com/kubernetes-sigs/cluster-api/issues/3255)|
|API, Contracts|Support multiple kubeconfigs for a provider|[#3661](https://github.com/kubernetes-sigs/cluster-api/issues/3661)|
|Core Improvements|Cleaner separation of kubeadm and machine bootstrapping|[#2318](https://github.com/kubernetes-sigs/cluster-api/issues/2318)|
|Core Improvements|Move away from corev1.ObjectReference|[#5294](https://github.com/kubernetes-sigs/cluster-api/issues/5294)|
|Extensibility|Support pluggable machine load balancers|[#1250](https://github.com/kubernetes-sigs/cluster-api/issues/1250)|
|Visibility|Add tracing to Cluster API reconcile loops| |
|Conformance| Define Cluster API provider conformance||

For a detailed view of all the issues in the milestone see [milestone Next](https://github.com/kubernetes-sigs/cluster-api/issues?q=is%3Aopen+is%3Aissue+milestone%3ANext).