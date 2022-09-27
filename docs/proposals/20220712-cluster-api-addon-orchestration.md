---
title: Cluster API Add-Ons Orchestration
authors:
  - "@fabriziopandini"
  - "@jont828"
  - "@jackfrancis"
reviewers:
  - "@CecileRobertMichon"
  - "@elmiko"
  - "@g-gaston"
  - "@enxebre"
  - "@dlipovetsky"
  - "@sbueringer"
  - "@fabriziopandini"
  - "@g-gaston"
  - "@killianmuldoon"
creation-date: 2022-07-12
last-updated: 2022-09-29
status: implementable
see-also:
replaces:
superseded-by:
---

# Cluster API Add-On Orchestration

- Ref: https://github.com/kubernetes-sigs/cluster-api/issues/5491
- Ref: https://docs.google.com/document/d/1TdbfXC2_Hhg0mH7-7hXcT1Gg8h6oXKrKbnJqbpFFvjw/edit?usp=sharing

## Table of Contents

- [Cluster API Add-On Orchestration](#cluster-api-add-on-orchestration)
  - [Table of Contents](#table-of-contents)
  - [Glosssary](#glosssary)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non goals](#non-goals)
    - [Future work](#future-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
      - [Story 3](#story-3)
      - [Story 4](#story-4)
      - [Story 5](#story-5)
    - [ClusterAddonProvider Functionality](#clusteraddonprovider-functionality)
    - [ClusterAddonProvider Design](#clusteraddonprovider-design)
      - [ClusterAddonProvider for Helm](#clusteraddonprovider-for-helm)
      - [HelmChartProxy](#helmchartproxy)
      - [HelmReleaseProxy](#helmreleaseproxy)
      - [Controller Design](#controller-design)
      - [Defining the list of Cluster add-ons to install](#defining-the-list-of-cluster-add-ons-to-install)
      - [Installing and lifecycle managing the package manager](#installing-and-lifecycle-managing-the-package-manager)
      - [Mapping HelmChartProxies to Helm charts](#mapping-helmchartproxies-to-helm-charts)
      - [Mapping Kubernetes version to add-ons version](#mapping-kubernetes-version-to-add-ons-version)
      - [Generating package configurations](#generating-package-configurations)
      - [Maintaining an inventory of add-ons installed in a Cluster](#maintaining-an-inventory-of-add-ons-installed-in-a-cluster)
      - [Surface Cluster add-ons status](#surface-cluster-add-ons-status)
      - [Upgrade strategy](#upgrade-strategy)
      - [Managing dependencies](#managing-dependencies)
    - [Example: Installing Calico CNI](#example-installing-calico-cni)
    - [Additional considerations](#additional-considerations)
      - [HelmChartProxy design](#helmchartproxy-design)
      - [Provider contracts](#provider-contracts)
  - [Cluster API enhancements](#cluster-api-enhancements)
    - [clusterctl](#clusterctl)
  - [Alternatives](#alternatives)
    - [Why not ClusterResourceSets?](#why-not-clusterresourcesets)
    - [Existing package management controllers](#existing-package-management-controllers)

## Glosssary 

**Package management tool:** a tool ultimately responsible[^1] for installing Kubernetes applications, e.g. [Helm](https://helm.sh/) or [carvel-kapp](https://carvel.dev/kapp/).

**Add-on:** an application that extends the functionality of Kubernetes.

**Cluster add-on:** an add-on that is essential to the proper function of the Cluster like e.g. networking (CNI) or integration with the underlying infrastructure provider (CPI, CSI). In some cases the lifecycle of Cluster add-on is strictly linked to the Cluster lifecycle (e.g. CPI must be upgraded in lock step with the Cluster upgrade).

**Helm**: A CNCF graduated project that serves as a package manager widely used in the community. Note that Helm packages are Helm charts.

[^1]: Package management tools usually have broader scope that goes over and behind the act of installing a Kuberentes application, like e.g. defining formats for packaging Kubernetes applications, defining repository/marketplaces where users can find libraries of existing applications, templating systems for customizing applications etc.

## Summary

Cluster API is designed to manage the lifecycle of Kubernetes Clusters through declarative specs, and currently users rely on their own package management tool of choice for Kubernetes application management (e.g Helm, kapp etc.).

This proposal aims to provide a solution for orchestrating Cluster add-ons around the Cluster lifecycle that is consistent with the Cluster API design philosophy by using a declarative spec.

This solution will likely require at least three iterations:

1. The foundation (the one described in this document) will aim to provide a minimal viable solution for this space, integration with the package management tool of choice (Helm in this case), explore and address basic concerns about orchestration, such as identifying a single source of truth for add-on configuration and a reconciliation mechanism to ensure the expected configuration is applied.

2. Advanced orchestration will build on the foundation from the first iteration and will explore and add scenarios like configuring add-ons to be upgraded before the Cluster upgrade starts. This work will leverage the lifecycle hooks introduced in Cluster API v1.2.

3. Add-on dependencies can be explored after the orchestration becomes more mature. This iteration can consider use cases where there are dependencies between add-ons and eventually introduce capabilities to account for that in add-on orchestration. For example, add-ons might need to be installed or ugpraded in a certain order and certain versions of add-ons might be incompatible with each other. The scope of this work is TBD.

## Motivation

ClusterResourceSets are the current solution for managing add-ons in Cluster API. However, ClusterResourceSets have been designed as a temporary measure until a better alternative is available. In particular, an add-on solution in Cluster API should be in line with the [goals of Cluster API](https://cluster-api.sigs.k8s.io/#goals) by aiming “to reuse and integrate existing ecosystem components rather than duplicating their functionality.” These components include other projects in SIG Cluster Lifecycle such as [Cluster Add-ons](https://github.com/kubernetes-sigs/cluster-addons) and well established tools like Helm, kapp, ArgoCD, and Flux. 

This proposal does not intend to solve the issue of add-ons in all of Kubernetes. Rather, it aims to bring add-ons into the Cluster API cluster lifecycle by managing add-ons using a declarative spec and orchestrating existing package management tools.

### Goals

- To design a solution for orchestrating Cluster add-ons.
- To leverage existing package management tools such as Helm for all the foundational capabilities of add-on management, i.e. add-on packages/repository, templating/configuration, add-on creation, upgrade and deletion etc. 
- To make add-on management in Cluster API modular and pluggable, and to make it simple for developers to build a Cluster API Add-on Provider based on any package management tool, just like with infrastructure and bootstrap providers.

### Non goals

- To implement a full fledged package management tool in Cluster API; there are already several awesome package management tools in the ecosystem, and CAPI should not reinvent the wheel. 
- To provide a mechanism for altering, customizing, or dealing with single Kubernetes resources defining a Cluster add-on, i.e. Deployments, Services, ServiceAccounts. Cluster API should treat add-ons as opaque components and delegate all the operations impacting add-on internals to the package management tool.
- To expect users to use a specific package management tool.
- To implement a solution for installing add-ons on the management cluster itself.

### Future work

- Handle the upgrade of Cluster add-ons when the workload Cluster is upgraded to a new Kubernetes version.
- Provide support for advanced orchestration use cases leveraging on Cluster API’s lifecycle hooks for Cluster API.
- Introduce capabilities to reconcile add-ons in an order that respects dependencies between add-ons.
- Deprecate or remove the ClusterResourceSet experimental feature.

## Proposal

This document introduces the concept of a ClusterAddonProvider, a new pluggable component to be deployed on the Management Cluster. A ClusterAddonProvider must be implemented using a specified package management tool and will act as an intermediary/proxy between Cluster API and the chosen package management solution.

This proposal also introduces a ClusterAddonProvider for [Helm](https://helm.sh/) as a concrete example of how a ClusterAddonProvider would work with a package management tool. Users can configure ClusterAddonProvider for Helm through a new Kubernetes CRD called HelmChartProxy to be deployed on the Management Cluster. HelmChartProxy will act as an intermediary (proxy) between Cluster API and the Helm to install packages onto workload clusters. The implementation of ClusterAddonProvider for Helm can be used as a reference to implement ClusterAddonProviders for other package managers, i.e. ClusterAddonProvider for kapp.

### User Stories

#### Story 1

As a developer or Cluster operator, I would like Cluster add-on providers to automatically include or install the package management tool.

#### Story 2

As a developer or Cluster operator, I would like Cluster add-on providers to expose configurations native to the package management tool to install Cluster add-ons into the newly created Workload Cluster.

#### Story 3

As a developer or Cluster operator, I would like Cluster add-on providers to leverage existing capabilities of the package management tool, such as identifying the proper version of Cluster add-ons package to install onto the workload Cluster.

#### Story 4

As a developer or Cluster operator, I would like Cluster add-on providers to use information available in the Management Cluster (e.g. service or pod CIDR on the Cluster object) for customizing add-ons configurations specifically for the newly created workload Cluster.

#### Story 5

As a developer or Cluster operator, I would like Cluster add-on providers to update add-ons configurations when information available in the Management Cluster (e.g add-on configuration) changes.

### ClusterAddonProvider Functionality

A ClusterAddonProvider is a new component for bringing add-ons into the Cluster API lifecycle. It serves as a general solution to Cluster API Add-ons with multiple concrete implementations, similar to the relationship between Cluster API and infrastructure providers. It aims to do so handle add-on lifecycle management in a holistic way by providing the following capabilities:

- Provide or install a package management tool
- Determine which add-ons to install or uninstall on each Cluster
- Provide a default configuration for each add-on
- Configure an add-on by resolving or inherit information from the Cluster configuration (i.e. service or pod CIDR for CNI)
- Install add-ons during Cluster creation (immediately after the API server is available). This operation usually brings in some additional requirements such as: 
  - Resolve the add-on version for the Cluster’s Kubernetes version
  - Wait for the add-on to be actually running
- Provide a mechanism for upgrading an add-on before, during, or after the Cluster upgrade and cleaning up the previous version
- Support for out-of band upgrades for add-ons not linked to the Cluster lifecycle, i.e. CVE fix
- Configure pre and post installation steps for each add-on, thus allowing to manage migrations/operations such as:
  - CPI migration from in-tree to out-of-tree
  - CoreDNS configuration migration from one version to another
- Orchestrate add-on deletion before Cluster deletion

This proposal addresses a subset of these problems as part of a first iteration by designing a way for Cluster API to orchestrate an external package management tool for add-on management. An external package management tool is expected to provide all the foundational capabilities for add-on management like add-on packages and repositories, templating/configuration, and management of the creation, upgrade, and deletion workflow etc.

### ClusterAddonProvider Design

When discussing the design of a ClusterAddonProvider, ClusterAddonProvider for Helm will be used as an example of a concrete implementation.

#### ClusterAddonProvider for Helm

ClusterAddonProvider for Helm proposes two new CRDs named HelmChartProxy and HelmReleaseProxy. HelmChartProxy is used to specify a Helm chart with configuration values and a selector identifying the Clusters the add-on applies to. HelmReleaseProxy contains information about the underlying Helm release installed on a cluster from a Helm chart. All the configurations will be done from the HelmChartProxy and the HelmReleaseProxy is simply used to maintain an inventory as well as surface information. Note that the specific fields in both CRDs are subject to change as the project evolves.

This is a link to the prototype for [ClusterAddonProvider for Helm](https://github.com/Jont828/cluster-api-addon-provider-helm), and a demo can be found in this [Cluster API Office Hours recording](https://www.youtube.com/watch?v=dipR1Tzh4jE). The prototype implements HelmChartProxy and HelmReleaseProxy as well as controllers for both CRDs.

#### HelmChartProxy

HelmChartProxy is a namespaced CRD that serves as the user interface for defining and configuring Helm charts as well as selecting which workload clusters it will be installed on. 

```go
// HelmChartProxySpec defines the desired state of HelmChartProxy.
type HelmChartProxySpec struct {
  // ClusterSelector selects Clusters in the same namespace with a label that matches the specified label selector. The Helm 
  // chart will be installed on all selected Clusters. If a Cluster is no longer selected, the Helm release will be uninstalled.
  ClusterSelector metav1.LabelSelector `json:"clusterSelector"`

  // ChartName is the name of the Helm chart in the repository.
  ChartName string `json:"chartName"`

  // RepoURL is the URL of the Helm chart repository.
  RepoURL string `json:"repoURL"`

  // ReleaseName is the release name of the installed Helm chart. If it is not specified, a
  // name will be generated.
  // +optional
  ReleaseName string `json:"releaseName,omitempty"`

  // ReleaseNamespace is the namespace the Helm release will be installed on each selected
  // Cluster. If it is not specified, it will be set to the default namespace.
  // +optional
  ReleaseNamespace string `json:"namespace,omitempty"`

  // Version is the version of the Helm chart. If it is not specified, the chart will use 
  // and be kept up to date with the latest version.
  // +optional
  Version string `json:"version,omitempty"`

  // ValuesTemplate is an inline YAML representing the values for the Helm chart. This YAML supports Go templating to reference
  // fields from each selected workload Cluster and programatically create and set values.
  // +optional
  ValuesTemplate string `json:"valuesTemplate,omitempty"`
}

// HelmChartProxyStatus defines the observed state of HelmChartProxy.
type HelmChartProxyStatus struct {
  // Conditions defines current state of the HelmChartProxy.
  // +optional
  Conditions clusterv1.Conditions `json:"conditions,omitempty"`

  // MatchingClusters is the list of references to Clusters selected by the ClusterSelector.
  // +optional
  MatchingClusters []corev1.ObjectReference `json:"matchingClusters"`
}
```

#### HelmReleaseProxy

HelmReleaseProxy is a namespaced resource representing a single Helm release installed on a selected workload Cluster. HelmReleaseProxies are created and managed by the HelmChartProxy controller. The parent HelmChartProxy will be responsible for keeping its HelmReleaseProxy children up-to-date as its lifecycle is updated (for example, if the `values` properties of a HelmChartProxy are updated, its child HelmReleaseProxy resources will be updated as well).

```go
// HelmReleaseProxySpec defines the desired state of HelmReleaseProxy.
type HelmReleaseProxySpec struct {
  // ClusterRef is a reference to the Cluster to install the Helm release on.
  ClusterRef corev1.ObjectReference `json:"clusterRef"`

  // ChartName is the name of the Helm chart in the repository.
  ChartName string `json:"chartName"`

  // RepoURL is the URL of the Helm chart repository.
  RepoURL string `json:"repoURL"`

  // ReleaseName is the release name of the installed Helm chart. If it is not specified, a
  // name will be generated.
  // +optional
  ReleaseName string `json:"releaseName,omitempty"`

  // ReleaseNamespace is the namespace the Helm release will be installed on the referenced 
  // Cluster. If it is not specified, it will be set to the default namespace.
  // +optional
  ReleaseNamespace string `json:"namespace"`

  // Version is the version of the Helm chart. If it is not specified, the chart will use 
  // and be kept up to date with the latest version.
  // +optional
  Version string `json:"version,omitempty"`

  // Values is an inline YAML representing the values for the Helm chart. This YAML is the result of the rendered
  // Go templating with the values from the referenced workload Cluster.
  // +optional
  Values string `json:"values,omitempty"`
}

// HelmReleaseProxyStatus defines the observed state of HelmReleaseProxy.
type HelmReleaseProxyStatus struct {
  // Conditions defines current state of the HelmReleaseProxy.
  // +optional
  Conditions clusterv1.Conditions `json:"conditions,omitempty"`

  // Status is the current status of the Helm release.
  // +optional
  Status string `json:"status,omitempty"`

  // Revision is the current revision of the Helm release.
  // +optional
  Revision int `json:"revision,omitempty"`
}
```

#### Controller Design

The HelmChartProxy controller is responsible for maintaining our list of HelmReleaseProxies and resolving any referenced Cluster fields. The controller will:

- Find all workload clusters matching the cluster selector within the same namespace.
- Resolve the referenced Cluster fields in the values map based on each selected cluster.
- Create a HelmReleaseProxy for each selected workload cluster if one does not exist with the resolved values.
- Update the HelmReleaseProxy for each selected workload cluster if it exists and the version or resolved values have changed.
- Delete the HelmReleaseProxy for each selected workload cluster if the cluster no longer matches the cluster selector.

The HelmReleaseProxy controller is responsible for creating, updating, and deleting the Helm release installed on the workload cluster. The controller will:

- Install a Helm release on the workload cluster if it does not exist.
- Update the existing Helm release if the HelmReleaseProxy values or version do not match the existing Helm release.
- Delete the Helm release if the HelmReleaseProxy is deleted.

This design of HelmReleaseProxy is especially convenient for the controller because a `Create()` maps directly to a `helm install` operation, a `Update()` maps directly to a `helm upgrade` operation, and a `Delete()` maps directly to a `helm uninstall` operation.

Note that the HelmReleaseProxy controller contains a static configuration and is only responsible for ensuring there is a Helm release matching the HelmReleaseProxy. The HelmReleaseProxy gets created, updated, and deleted by the HelmChartProxy controller.

#### Defining the list of Cluster add-ons to install

HelmChartProxy allows users to define the add-ons to install and which clusters they should be installed on using the `clusterSelector`. The specified Helm chart is then installed on every cluster that includes a label matching the cluster selector and makes it part of the orchestration. If a selected cluster no longer matches the cluster selector label, then the Helm release will be uninstalled. 

#### Installing and lifecycle managing the package manager

ClusterAddonProvider for Helm imports Helm v3 as a library, making this a no-op. However, it is worth noting that each version of Helm only supports certain version of Kubernetes. For example, the [Helm docs](https://helm.sh/docs/topics/version_skew/#supported-version-skew) mention that Helm 3.9 supports Kubernetes 1.21 through 1.24. In this case, we will document the version support matrix for ClusterAddonProvider for Helm if it differs from the CAPI support matrix.

Additionally, other package managers like kapp require in-cluster components such as the kapp-controller. In this case, ClusterAddonProvider should include those in-cluster components as well.

#### Mapping HelmChartProxies to Helm charts

A Helm chart is uniquely identified using the URL of its repository, the name of the chart, and the chart version. As a result, each HelmChartProxy uses the `repoURL`, `chartName`, and `version` fields to uniquely identify a Helm chart and determine which version to install. This allows HelmChartProxy to be compatible with any valid Helm repository and chart.

#### Mapping Kubernetes version to add-ons version

By definition, the Cluster add-on lifecycle is strictly linked to the Cluster lifecycle. As a result, there is a strict correlation between the Cluster’s Kubernetes version and the add-on version to be installed in a Cluster.

Some add-ons have an explicit version correlation such as Cloud Provider Interface following the “Versioning Policy for External Cloud Providers.” Other add-ons like Calico define a case by case compatability matrix in their [docs](https://projectcalico.docs.tigera.io/getting-started/kubernetes/requirements#supported-versions).

On top of that, in many Cluster API installations, the add-on version an operator would like to use is limited to a list of certified versions defined in each organization/product vendor.

We can rely upon existing functionality and capabilities in Helm to enforce Kubernetes version-specific requirements in a chart, including whether or not to install anything at all. It's worth noting that other add-on tools might not provide this functionality out of the box.

#### Generating package configurations 

Each package manager has its own specific configuration format. Helm charts, for example, can be configured using the `values.yaml` file where each chart can define its own configurable fields in YAML format.

For ClusterAddonProvider for Helm, a solution is relatively simple. HelmChartProxy contains a `valuesTemplate` field which allows the contents of a `values.yaml` file to be passed in as a inline YAML string.

Additionally, the `valuesTemplate` field functions as a Go template. This allows `valuesTemplate` to reference fields from the Cluster definition, i.e. cluster name, namespace, control plane ref, pod CIDRs. These template fields will be resolved dynamically based on the actual values for each workload cluster selected by HelmChartProxy’s cluster selector.

In a future iteration we might consider to leverage template variables in ClusterClass. If a HelmChartProxy is selecting clusters with a managed topology, then the Go template configuration can benefit from `Cluster.spec.topology.variables`, and from the strong typing/validation ensured by the variable schema defined in corresponding ClusterClass.

#### Maintaining an inventory of add-ons installed in a Cluster

Maintaining an inventory of add-ons installed is useful for a number of reasons. The most straightforward is for users to see at a glance which add-ons are installed on a cluster. On the other hand, the inventory is useful for add-on orchestration as well. It allows the controller to be able to distinguish between an add-on installed through the orchestrator and an add-on installed out of band. This means that on add-on deletion, the controller will only uninstall add-ons it is responsible for and won’t remove existing add-ons.

First, the simplest is to let the package manager maintain the list. For example, `helm list` lists all the releases on a cluster which already handles maintaining an inventory of addons. However, there is no way to distinguish between out of band add-ons and add-ons installed through the orchestrator. Additionally, any ClusterAddonProvider would need to rely on their package manager to have a suitable implementation of listing packages on a cluster.

The solution implemented in the ClusterAddonProvider for Helm is to use HelmReleaseProxy to maintain an inventory. Each HelmReleaseProxy corresponds to a Helm release installed through HelmChartProxy. The controller only orchestrates Helm releases with a corresponding HelmReleaseProxy, ensuring that out of band Helm releases won’t be affected. 

Additionally, each HelmReleaseProxy includes a `clusterv1.ClusterLabelName` and an additional label indicating the HelmChartProxy it corresponds to. This provides a way to query for all Helm releases installed on a Cluster and to query for all Helm releases belonging to a HelmChartProxy.

#### Surface Cluster add-ons status

ClusterAddonProviders are responsible for installing add-ons on an arbitrary number of workload clusters. As such, it would be beneficial to see the status of an add-on from the management cluster and avoid having to fetch the kubeconfig of the workload cluster.

In ClusterAddonProvider for Helm, the status of a Helm release can be conveyed through the HelmReleaseProxy status fields like `status`, `revision`, and `namespace`. Additional information can be surfaced in the `conditions` field.

#### Upgrade strategy

Users will be able to upgrade add-ons across multiple Clusters at once by updating the `version` field in the HelmChartProxy to trigger a Helm upgrade for all the Helm releases it manages. Additionally, HelmChartProxy allows users to upgrade their add-ons in a declarative manner.

Note that hooks and automatic triggers for add-on upgrades when a Cluster is upgraded to a new Kubernetes version is out of scope for this iteration. Users will need to determine if an add-on version upgrade is needed, and if so, set a compatible version in the HelmChartProxy.

In future iterations, we plan to handle the upgrade of Cluster add-ons when the workload Cluster is upgraded to a new Kubernetes version by leveraging the new runtime hooks feature.

#### Managing dependencies

Managing dependencies between HelmChartProxies is out of scope for this project. However, Helm charts define their dependencies in the [Chart.yaml file](https://helm.sh/docs/topics/charts/#the-chartyaml-file) and will install them automatically. If a Helm chart has a critical dependency on another chart, it is the chart owner's responsibility to enable it by default.

### Example: Installing Calico CNI

Developers and Cluster operators often want to install a CNI on their workload clusters. HelmChartProxy controller can be used to install the Calico CNI on each selected workload cluster. In this example, the value configuration can be leveraged to initialize the `installation.calicoNetwork.ipPools` array by creating an element for every pod CIDR in the Cluster.

```yaml
apiVersion: addons.cluster.x-k8s.io/v1alpha1
kind: HelmChartProxy
metadata:
  name: calico-cni
spec:
  clusterSelector:
    matchLabels:
      calicoCNI: enabled
  releaseName: calico
  repoURL: https://projectcalico.docs.tigera.io/charts
  chartName: tigera-operator
  valuesTemplate: |
    installation:
      cni:
        type: Calico
        ipam:
          type: HostLocal
      calicoNetwork:
        bgp: Disabled
        mtu: 1350
        ipPools:{{range $i, $cidr := .Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks }}
        - cidr: {{ $cidr }}
          encapsulation: None
          natOutgoing: Enabled
          nodeSelector: all(){{end}}
```

### Additional considerations

#### HelmChartProxy design

The scope of the cluster values accessible using Go templating must be defined along with use cases. An option would be to expose the Builtin struct type used in ClusterClass or construct a similar type. Additionally, while resources like the control plane and cluster have a 1:1 correlation with each selected cluster, resources like MachineDeployments or Machines could have any number and can’t be selected in a similar way. As a result, additional work is needed to decide if there’s a use case for including information in those types.

There has also been discussion about whether HelmChartProxy and HelmReleaesProxy should be Cluster wide namespaced. This would impact whether an instance of HelmChartProxy would select Clusters across all namespaces or only Clusters within the same namespace. The CRDs are namespaced for the sake of simplicity as well as consistency with all the other Cluster API CRDs, but it is worth noting that it can be inconvenient if a user has several namespaces and wants to install the same Helm chart on all of them.

#### Provider contracts

As it stands, there is no formalized provider contract between ClusterAddonProviders that a ClusterAddonProvider must implement. That being said, the design of ClusterAddonProvider for Helm does suggest that certain patterns and components can be reused between different ClusterAddonProviders. These patterns can include:

- Designing a CRD like HelmChartProxy to specify an add-on to install on certain Clusters.
- Using a label selector of some kind to install an add-on on a selected workload Cluster and to uninstall an add-on a Cluster that is unselected.
- Using Go templates in a field like `valuesTemplate` to dynamically resolve configurations based on each Cluster's definition.
- Designing a CRD like HelmReleaseProxy to:
  - Maintain an inventory of add-ons and differentiate between managed add-ons and out of band add-ons.
  - Provide a modular design such that the HelmChartProxy controller does not need to implement any Helm operations and only make Kubernetes client calls to create, update, or delete a HelmReleaseProxy.

## Cluster API enhancements

Despite the fact that most of the heavy lifting for add-on management is delegated to external ClusterAddonProvider components, over time we would like to provide Cluster API’s users an integrated and seamless user experience for add-on orchestration.

### clusterctl

ClusterAddonProvider is a new component to be installed in a Management Cluster, and in order to make this operation simpler, we can extend clusterctl in order to manage ClusterAddonProvider implementations in clusterctl init, clusterctl upgrade, etc.

Note: clusterctl assumes all the providers to be linked to a CAPI contract; while it is not clear at this stage if the same applies to ClusterAddonProvider. This should be further investigated.

## Alternatives

### Why not ClusterResourceSets?

This proposal introduces ClusterAddonProviders as opposed to overhauling ClusterResourceSets for the following reasons:

- ClusterResourceSets do not follow the Cluster API convention of using a declarative spec and having controllers create, delete, or update resources to build the desired state. Even though there has been work to add a Reconcile or an "apply always" mode to ClusterResourceSets, it is [explicitly stated](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20200220-cluster-resource-set.md#non-goalsfuture-work) that deleting resources from Clusters is a non-goal.
- Users are already using package management tools, and being able to use their preferred package management tool reduces user friction towards adopting an add-on solution.
- ClusterResourceSets have relatively fewer user experts compared to existing package management tools like Helm.
- ClusterResourceSets fail to take advantage of functionality in existing package management tools like dependency installation, and evolving ClusterResourceSets would require re-inventing the functionality of existing package management tools.
- ClusterResourceSets do not orchestrate deletion of add-ons and was not designed with that in mind. In order to orchestrate deletion of add-ons, it would require a significant redesign of the code and interface. Doing so would still not address the above issues compared to designing a new solution with all of this in mind.

### Existing package management controllers

It is worth noting that there are other projects have integrated a package management tool with a Kubernetes controller. For example, Flux has developed a [Helm controller](https://github.com/fluxcd/helm-controller) as part of its GitOps toolkit for syncing Clusters with sources of configuration. It defines a `HelmRelease` CRD and offers [integration with Cluster API](https://fluxcd.io/flux/components/helm/helmreleases/#remote-clusters--cluster-api) where users can create a `HelmRelease` to install an add-on on a specified Cluster.

Given that this proposal aims to explore the topic of orchestration between Cluster API and add-on tools, we still believe that using a simpler, homegrown version of HelmRelease CRD and avoiding dependencies with other projects allows us to iterate faster while we are still figuring out the best orchestration patterns between the two components.

However, we encourage users to explore both solutions and provide feedback, so we can better figure out next steps.