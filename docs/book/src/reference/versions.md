# Cluster API and Kubernetes version support

<!-- TOC -->
* [Cluster API and Kubernetes version support](#cluster-api-and-kubernetes-version-support)
  * [Version support policies](#version-support-policies)
    * [Cluster API release support](#cluster-api-release-support)
      * [Skip upgrades](#skip-upgrades)
      * [Downgrades](#downgrades)
      * [Cluster API release vs API versions](#cluster-api-release-vs-api-versions)
      * [Cluster API release vs contract versions](#cluster-api-release-vs-contract-versions)
      * [Supported Cluster API - Cluster API provider version Skew](#supported-cluster-api---cluster-api-provider-version-skew)
    * [Kubernetes versions support](#kubernetes-versions-support)
      * [Maximum version skew between various Kubernetes components](#maximum-version-skew-between-various-kubernetes-components)
  * [Supported versions matrix by provider or component](#supported-versions-matrix-by-provider-or-component)
    * [Core provider (`cluster-api-controller`)](#core-provider-cluster-api-controller)
    * [Kubeadm Bootstrap provider (`kubeadm-bootstrap-controller`)](#kubeadm-bootstrap-provider-kubeadm-bootstrap-controller-)
      * [Kubeadm configuration API Support](#kubeadm-configuration-api-support)
    * [Kubeadm Control Plane provider (`kubeadm-control-plane-controller`)](#kubeadm-control-plane-provider-kubeadm-control-plane-controller)
      * [Bootstrap provider Support](#bootstrap-provider-support)
      * [Etcd API Support](#etcd-api-support)
      * [CoreDNS Support](#coredns-support)
    * [Other providers](#other-providers)
    * [clusterctl](#clusterctl)
  * [Annexes](#annexes)
    * [Kubernetes version Support and Cluster API deployment model](#kubernetes-version-support-and-cluster-api-deployment-model)
    * [Kubernetes version specific notes](#kubernetes-version-specific-notes)
<!-- TOC -->

## Version support policies

### Cluster API release support

This paragraph documents the general rules defining how we determine Cluster API supported releases.

A Cluster API release correspond to a release in the [GitHub repository](https://github.com/kubernetes-sigs/cluster-api/releases)
for this project, and the corresponding images published in the Kubernetes docker registry.

For the sake of this document, the most important artifacts included in a Cluster API release are:

- The Cluster API Core provider image
- The Kubeadm Bootstrap provider image
- The Kubeadm Control Plane provider image
- The clusterctl binary

The Cluster API team will release a new Cluster API version approximately every four months (3 releases each year).
See [release cycle](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-cycle.md) and [release calendars](https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/release/releases) for more details about Cluster API releases management.

The Cluster API team actively supports the latest two minor releases (N, N-1); support in this context means that we:

- Have CI signal with E2E tests, unit tests, CVE scans etc.
- Accept bug fixes, perform golang or dependency bumps, etc. 
- Periodically cut patch releases

On top of supporting the N and N-1 releases, the Cluster API team also maintains CI signal for the Cluster API N-2 releases 
in case we have to do an emergency patch release.  
- If there is a need for an emergency patch, e.g. to fix a critical security issue, please bring this up to maintainers
  and it will be considered on a case-by-case basis. 

All considered, each Cluster API minor release is supported for a period of roughly 12 months:

- The first eight months of this timeframe will be considered the standard support period for a minor release.
- The next four months the minor release will be considered in maintenance mode.
- At the end of the four-month maintenance mode period, the minor release will be considered EOL (end of life) and 
  cherry picks to the associated branch are to be closed soon afterwards.

The table below documents support matrix for Cluster API versions (versions older than v1.0 omitted).

| Minor Release | Status                  | Supported Until (including maintenance mode)                                                |
|---------------|-------------------------|---------------------------------------------------------------------------------------------|
| v1.11.x       | Under development       |                                                                                             |
| v1.10.x       | Standard support period | in maintenance mode when v1.12.0 will be released, EOL when v1.13.0 will be released        |
| v1.9.x        | Standard support period | in maintenance mode when v1.11.0 will be released, EOL when v1.12.0 will be released        |
| v1.8.x        | Maintenance mode        | Maintenance mode since 2025-04-22 - v1.10.0 release date, EOL when v1.11.0 will be released |
| v1.7.x        | EOL                     | EOL since 2025-04-22 - v1.10.0 release date                                                 |
| v1.6.x        | EOL                     | EOL since 2024-12-10 - v1.9.0 release date                                                  |
| v1.5.x        | EOL                     | EOL since 2024-08-12 - v1.8.0 release date                                                  |
| v1.4.x        | EOL                     | EOL since 2024-04-16 - v1.7.0 release date                                                  |
| v1.3.x        | EOL                     | EOL since 2023-12-05 - v1.6.0 release date                                                  |
| v1.2.x        | EOL                     | EOL since 2023-07-25 - v1.5.0 release date                                                  |
| v1.1.x        | EOL                     | EOL since 2023-03-28 - v1.4.0 release date                                                  |
| v1.0.x        | EOL                     | EOL since 2022-12-01 - v1.3.0 release date                                                  |

#### Skip upgrades

Cluster API supports at maximum n-3 minor version skip upgrades.

For example, if you are running Cluster API v1.6.x, you can upgrade up to Cluster API v1.9.x skipping intermediate
minor versions (v1.6 is v1.9 minus three minor versions).

<aside class="note warning">

<h1>Warning</h1>

Upgrades outside from version older n-3 might lead to a management cluster in a non-functional state.

 </aside>

#### Downgrades

Cluster API does not support version downgrades.

<aside class="note warning">

<h1>Warning</h1>

Version downgrades might lead to a management cluster in a non-functional state.

 </aside>

#### Cluster API release vs API versions

Each Cluster API release can support one or more API versions.

An API version is determined from the GroupVersion defined in the top-level `api/` package of a specific Cluster API release, and it is used
in the `apiVersion` field of Cluster API custom resources.

An API version is considered deprecated when a new API version is published.

API deprecation and removal follow the [Kubernetes Deprecation Policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/);
Cluster API maintainers might decide to support API versions longer than what is defined in the Kubernetes policy.

| API Version | Status         | Supported Until                                                                  |
|-------------|----------------|----------------------------------------------------------------------------------|
| v1beta1     | Supported      | at least 9 months or 3 minor releases after a newer API version will be released |
| v1alpha4    | Not served (*) | EOL since 2023-12-05 - v1.6.0 release date                                       |
| v1alpha3    | Not served (*) | EOL since 2023-07-25 - v1.5.0 release date                                       |

(*) Cluster API stopped to serve v1alpha3 API types from the v1.5 release and v1alpha4 types starting from the v1.6 release. 
Those types still exist in Cluster API while we work to a fix (or a workaround) for [10051](https://github.com/kubernetes-sigs/cluster-api/issues/10051). 

<aside class="note warning">

<h1>Warning</h1>

Note: Removal of a deprecated APIVersion in Kubernetes can cause issues with garbage collection by the kube-controller-manager.
This means that some objects which rely on garbage collection for cleanup - e.g. MachineSets and their descendent objects, 
like Machines and InfrastructureMachines, may not be cleaned up properly if those objects were created with an APIVersion 
which is no longer served.

To avoid these issues it’s advised to ensure a restart of the kube-controller-manager is done after upgrading to a version
of Cluster API which drops support for an APIVersion - e.g. v1.5 and v1.6.

This can be accomplished with any Kubernetes control-plane rollout, including a Kubernetes version upgrade, or by manually
stopping and restarting the kube-controller-manager.

</aside>

#### Cluster API release vs contract versions

Each Cluster API contract version defines a set of rules a provider is expected to comply with in order to interact with a specific Cluster API release.
Those rules can be in the form of CustomResourceDefinition (CRD) fields and/or expected behaviors to be implemented.
See [provider contracts](../developer/providers/contracts/overview.md)

Each Cluster API release supports only one contract version, and by convention the supported contract version matches
the newest API version in the same Cluster API release.

| Contract Version | Status    | Supported Until                                                               |
|------------------|-----------|-------------------------------------------------------------------------------|
| v1beta1          | Supported | After a newer API contract will be released                                   |
| v1alpha4         | EOL       | EOL since 2023-12-05 - v1.6.0 release date; removal planned for v1.13, Apr 26 |
| v1alpha3         | EOL       | EOL since 2023-07-25 - v1.5.0 release date; removal planned for v1.13, Apr 26 |

See [11919](https://github.com/kubernetes-sigs/cluster-api/issues/11919) for details about the v1alpha3/v1alpha4 removal plan.

#### Supported Cluster API - Cluster API provider version Skew

When running a Cluster API release, all the provider installed in the same management cluster MUST
implement the CustomResourceDefinition (CRD) fields and/or expected behaviors defined by the release's contract version.

As a corollary, provider's version number and provider's API version number are not required to match Cluster API versions.

<aside class="note">

The Cluster API command line tool, `clusterctl`, will take care of ensuring all the providers are on the
same contract version both during init and upgrade of a management cluster.

</aside>

### Kubernetes versions support

This paragraph documents the general rules defining how the Cluster API team determines supported Kubernetes versions for every
Cluster API release. 

When a new Cluster API release is cut, we will document the Kubernetes version compatibility matrix the release
has been tested with in the [table](#supported-versions-matrix-by-provider-or-component) below.

Each Cluster API minor release supports (when it's initially created):
* 4 Kubernetes minor releases for the management cluster (N - N-3)
* 6 Kubernetes minor releases for the workload cluster (N - N-5)

When a new Kubernetes minor release is available, the Cluster API team will try to support it in an upcoming Cluster API
patch release, thus extending the support matrix for the latest supported Cluster API minor release to:
* 5 Kubernetes minor releases for the management cluster (N - N-4)
* 7 Kubernetes minor releases for the workload cluster (N - N-6)

For example, Cluster API v1.7.0 would support the following Kubernetes versions:
* v1.26.x to v1.29.x for the management cluster
* v1.24.x to v1.29.x for the workload cluster
* When Kubernetes 1.30 is released, it will be supported in v1.7.x (but not in v1.6.x)

<aside class="note warning">

<h1>Warning</h1>

Cluster API support for older Kubernetes version is not a replacement/alternative for upstream Kubernetes support policies!

Support for versions of Kubernetes which itself are out of support is limited to "Cluster API can start a Cluster with this Kubernetes version"
and "Cluster API  can upgrade to the next Kubernetes version"; it does not include any extended support to Kubernetes itself.

</aside>

See [Kubernetes version Support and Cluster API deployment model](#kubernetes-version-support-and-cluster-api-deployment-model) 
to understand how the way you deploy Cluster API might affect the Kubernetes version support matrix for a Cluster.

On a final comment, let's praise all the contributors keeping care of such a wide support matrix.
If someone is looking for opportunities to help with the project, this is definitely an area where additional hands
and eyes will be more than welcome and greatly beneficial to the entire community.

#### Maximum version skew between various Kubernetes components

Standard [Kubernetes version Skew Policy](https://kubernetes.io/releases/version-skew-policy/)
defines the maximum version skew supported between various Kubernetes components within a single cluster.

Notably, version skew between various Kubernetes components also define constraints to be observed
by Cluster API, Cluster API providers or Cluster API users when performing Kubernetes version upgrades.

In some cases, also Cluster API and/or Cluster API providers are defining additional version skew constraints. For instance:
- If you are using kubeadm as a bootstrapper, you must abide to the [kubeadm skew policy](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/#version-skew-policy). 
- If you are using image builder, all the Kubernetes components on a single machine are of the same version
- If your Cluster has a defined topology, with `Cluster.spec.topology` set and referencing a ClusterClass, 
  Cluster API enforces a single Kubernetes version for all the machines in the cluster.

## Supported versions matrix by provider or component

### Core provider (`cluster-api-controller`)

The following table defines the support matrix for the Cluster API core provider.
See [Cluster API release support](#cluster-api-release-support) and [Kubernetes versions support](#kubernetes-versions-support).

|                  | v1.8, _Maintenance Mode_ | v1.9              | v1.10             |
|------------------|--------------------------|-------------------|-------------------|
| Kubernetes v1.24 |                          |                   |                   |
| Kubernetes v1.25 | ✓ (only workload)        |                   |                   |
| Kubernetes v1.26 | ✓ (only workload)        | ✓ (only workload) |                   |
| Kubernetes v1.27 | ✓                        | ✓ (only workload) | ✓ (only workload) |
| Kubernetes v1.28 | ✓                        | ✓                 | ✓ (only workload) |
| Kubernetes v1.29 | ✓                        | ✓                 | ✓                 |
| Kubernetes v1.30 | ✓                        | ✓                 | ✓                 |
| Kubernetes v1.31 | ✓ >= v1.8.1              | ✓                 | ✓                 |
| Kubernetes v1.32 |                          | ✓ >= v1.9.1       | ✓                 |

See also [Kubernetes version specific notes](#kubernetes-version-specific-notes).

### Kubeadm Bootstrap provider (`kubeadm-bootstrap-controller`) 

For each version of the Cluster API core provider, there is a corresponding version of the Kubeadm Bootstrap provider.

The Kubeadm Bootstrap provider also follows the same support rules defined in [Cluster API release support](#cluster-api-release-support) 
and [Kubernetes versions support](#kubernetes-versions-support).

As a consequence, the support matrix for the Kubeadm Bootstrap provider is the same as the one
defined for the Cluster API [Core provider](#core-provider-cluster-api-controller).

#### Kubeadm configuration API Support

When creating new machines, the Kubeadm Bootstrap provider generates kubeadm init/join configuration files
using the [kubeadm API](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/control-plane-flags/) version recommended for the target Kubernetes version.

|                  | kubeadm API Version                                                                |
|------------------|------------------------------------------------------------------------------------|
| Kubernetes v1.24 | [v1beta3](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta3/) |
| Kubernetes v1.25 | [v1beta3](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta3/) |
| Kubernetes v1.26 | [v1beta3](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta3/) |
| Kubernetes v1.27 | [v1beta3](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta3/) |
| Kubernetes v1.28 | [v1beta3](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta3/) |
| Kubernetes v1.29 | [v1beta3](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta3/) |
| Kubernetes v1.30 | [v1beta3](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta3/) |
| Kubernetes v1.31 | [v1beta4](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta4/) |
| Kubernetes v1.32 | [v1beta4](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta4/) |

### Kubeadm Control Plane provider (`kubeadm-control-plane-controller`)

For each version of the Cluster API core provider, there is a corresponding version of the Kubeadm Control Plane provider.

The Kubeadm Control Plane provider also follows the same support rules defined in [Cluster API release support](#cluster-api-release-support) 
and [Kubernetes versions support](#kubernetes-versions-support).

As a consequence, the support matrix for the Kubeadm Control Plane provider is the same as the one
defined for the Cluster API [Core provider](#core-provider-cluster-api-controller).

#### Bootstrap provider Support

The Kubeadm Control Plane requires the Kubeadm Bootstrap provider of the same version.

#### Etcd API Support

The Kubeadm Control Plane provider communicates with  the API server and etcd members of every Workload Cluster whose control plane it owns.
All the Cluster API Kubeadm Control Plane providers currently supported are using [etcd v3 API](https://etcd.io/docs/v3.2/rfc/v3api/) when communicating with etcd.

#### CoreDNS Support

Each version of the Kubeadm Control Plane can upgrade up to a max CoreDNS version.
Notably, the Max CoreDNS version could change also with patch releases.

| KCP Version | Max CoreDNS Version |
|-------------|---------------------|
| v1.5        | v1.10.1             |
| >= v1.5.1   | v1.11.1             |
| v1.6        | v1.11.1             |
| v1.7        | v1.11.1             |
| v1.8        | v1.11.3             |
| >= v1.8.9   | v1.12.0             |
| >= v1.8.12  | v1.12.1             |
| v1.9        | v1.11.3             |
| >= v1.9.4   | v1.12.0             |
| >= v1.9.7   | v1.12.1             |
| v1.10       | v1.12.1             |

See [corefile-migration](https://github.com/coredns/corefile-migration)

### Other providers

Cluster API has a vibrant ecosystem of awesome providers maintained by independent teams and hosted outside of
the Cluster API [GitHub repository](https://github.com/kubernetes-sigs/cluster-api/).

To understand the list of supported version of a specific provider, its own Kubernetes support matrix, supported API versions, 
supported contract version and specific skip upgrade rules, please see its documentation. Please refer to [providers list](providers.md)

In general, if a provider version M says it is compatible with Cluster API version N, then it MUST be compatible 
with a subset of the Kubernetes versions supported by Cluster API version N.

### clusterctl

It is strongly recommended to always use the latest patch version of [clusterctl](../clusterctl/overview.md), in order to get all the fixes/latest changes.

In case of upgrades, clusterctl should be upgraded first and then used to upgrade all the other components.

## Annexes

### Kubernetes version Support and Cluster API deployment model

The most common deployment model for Cluster API assumes Core provider, Kubeadm Bootstrap provider, and Kubeadm Control Plane provider
and at least one infrastructure provider running on the Management Cluster, all managing the lifecycle
of a set of _separate_ Workload clusters.

![Management/Workload Separate Clusters](../images/management-workload-separate-clusters.png)

In this scenario, the Kubernetes version of the Management and Workload Clusters are allowed to be different.
Additionally, Management Clusters and Workload Clusters can be upgraded independently and in any order.

In another deployment model for Cluster API,  the Cluster API providers are used not only to managing the
lifecycle of _separate_ Workload clusters, but also to manage the lifecycle of the Management cluster itself.
This cluster is also referred to as a "self-hosted" cluster.

![Management/Workload Same Cluster](../images/management-workload-same-cluster.png)

The Kubernetes version of the "self-hosted" cluster is limited to the Kubernetes version currently supported
for the Management clusters.

### Kubernetes version specific notes

**1.31**:

* All providers:
  * It is not possible anymore to continuously apply CRDs that are setting `caBundle` to an invalid value (in our case `Cg==`). Instead of setting a dummy value the `caBundle` field should be dropped ([#10972](https://github.com/kubernetes-sigs/cluster-api/pull/10972)).
* Kubeadm Bootstrap provider:
  * `kubeadm` dropped the `control-plane update-status` phase which was used in ExperimentalRetryJoin ([#10983](https://github.com/kubernetes-sigs/cluster-api/pull/10983)).
  * `kubeadm` introduced the experimental `ControlPlaneKubeletLocalMode` feature gate which will be automatically enabled by CAPI for upgrades to v1.31 to not cause network disruptions ([#10947](https://github.com/kubernetes-sigs/cluster-api/pull/10947)).

**1.29**:
* In-tree cloud providers are now switched off by default. Please use DisableCloudProviders and DisableKubeletCloudCredentialProvider feature flags if you still need this functionality. (https://github.com/kubernetes/kubernetes/pull/117503)

**1.24**:
* Kubeadm Bootstrap provider:
  * `kubeadm` now sets both the `node-role.kubernetes.io/control-plane` and `node-role.kubernetes.io/master` taints on control plane nodes.
  * `kubeadm` now only sets the `node-role.kubernetes.io/control-plane` label on control plane nodes (the `node-role.kubernetes.io/master` label is not set anymore).
* Kubeadm Bootstrap provider and Kubeadm Control Plane provider
  * `criSocket` without a scheme prefix has been deprecated in the kubelet since a while. `kubeadm` now shows a warning if no scheme is present and eventually the support for `criSocket`'s without prefix will be dropped. Please adjust the `criSocket` accordingly (e.g. `unix:///var/run/containerd/containerd.sock`) if you are configuring the `criSocket` in CABPK or KCP resources.
