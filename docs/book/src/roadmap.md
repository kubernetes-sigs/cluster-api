# Cluster API Roadmap

This roadmap is a constant work in progress, subject to frequent revision. Dates are approximations.


## v0.3.7 (v1alpha3+) ~ June/July 2020

|Area|Description|Issue/Proposal|
|--|--|--|
|Testing|E2E Test plan|[Spreadsheet](https://docs.google.com/spreadsheets/d/1uB3DyacOLctRjbI6ov7mVoRb6PnM4ktTABxBygt5sKI/edit#gid=0)
|Testing|Enable webhooks in integration tests|[#2788](https://github.com/kubernetes-sigs/cluster-api/issues/2788)|
|Control Plane|KubeadmControlPlane robustness|[#2753](https://github.com/kubernetes-sigs/cluster-api/issues/2753)|
|Control Plane|KubeadmControlPlane adoption|[#2214](https://github.com/kubernetes-sigs/cluster-api/issues/2214)|
|Extensibility|Clusterctl library should support extensible templating|[#2339](https://github.com/kubernetes-sigs/cluster-api/issues/2339)|
|Cluster Lifecycle|ClusterResourceSet experiment|[#2395](https://github.com/kubernetes-sigs/cluster-api/issues/2395)|
|Core Improvements|Library to watch remote workload clusters|[#2414](https://github.com/kubernetes-sigs/cluster-api/issues/2414)|
|API, UX|Support and define conditions on cluster api objects|[#1658](https://github.com/kubernetes-sigs/cluster-api/issues/1658)|
|Extensibility, Infrastructure|Support spot instances|[#1876](https://github.com/kubernetes-sigs/cluster-api/issues/1876)|
|Extensibility|Machine pre-deletion hooks|[#1514](https://github.com/kubernetes-sigs/cluster-api/issues/1514)|
|Integration|Autoscaler|[#2530](https://github.com/kubernetes-sigs/cluster-api/issues/2530)|

## v0.4 (v1alpha4) ~ Q4 2020

|Area|Description|Issue/Proposal|
|--|--|--|
|UX, Bootstrap|Machine bootstrap failure detection|[#2554](https://github.com/kubernetes-sigs/cluster-api/issues/2554)|
|Extensibility|Support pluggable machine load balancers|[#1250](https://github.com/kubernetes-sigs/cluster-api/issues/1250)|
|Tooling Improvements| Define clusterctl inventory specification & have providers implement it|TBA|
|Core Improvements|Move away from corev1.ObjectReference|[#2318](https://github.com/kubernetes-sigs/cluster-api/issues/2318)|
|Dependency|Kubeadm v1beta2 types and support|[#2769](https://github.com/kubernetes-sigs/cluster-api/issues/2769)|


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
|Conformance| Define Cluster API provider conformance|TBA|
|Core Improvements|Pluggable MachineDeployment upgrade strategies|[#1754](https://github.com/kubernetes-sigs/cluster-api/issues/1754)|
|UX|Simplified cluster creation experience|[#1227](https://github.com/kubernetes-sigs/cluster-api/issues/1227)|
|Bootstrap, Infrastructure|Document approaches for infrastructure providers to consider for securing sensitive bootstrap data|[#1739](https://github.com/kubernetes-sigs/cluster-api/issues/1739)|
|Dependency|Clusterctl manages cert-manager lifecycle|[#2635](https://github.com/kubernetes-sigs/cluster-api/issues/2635)|
