---
title: Cluster API State Metrics
authors:
  - "@tobiasgiese"
  - "@chrischdi"
reviewers:
  - "@johannesfrey"
  - "@enxebre"
  - "@sbueringer"
  - "@apricote"
  - "@fabriziopandini"
creation-date: 2022-03-03
last-updated: 2022-03-15
status: experimental
---

# Cluster API State Metrics

## Table of Contents

- [Cluster API State Metrics](#cluster-api-state-metrics)
  - [Table of Contents](#table-of-contents)
  - [Glossary](#glossary)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
    - [Future Work](#future-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
      - [Scrapable Information](#scrapable-information)
      - [Relationship to kube-state-metrics](#relationship-to-kube-state-metrics)
      - [How does kube-state-metrics work](#how-does-kube-state-metrics-work)
      - [Reuse kube-state-metrics packages](#reuse-kube-state-metrics-packages)
      - [Package structure for cluster-api-state-metrics](#package-structure-for-cluster-api-state-metrics)
    - [Security Model](#security-model)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Alternatives](#alternatives)
    - [Directly use kube-state-metrics](#directly-use-kube-state-metrics)
    - [Expose metrics by the controllers](#expose-metrics-by-the-controllers)
  - [Upgrade Strategy](#upgrade-strategy)
  - [Additional Details](#additional-details)
    - [Metrics](#metrics)
      - [Cluster CR](#cluster-cr)
      - [KubeadmControlPlane CR](#kubeadmcontrolplane-cr)
      - [MachineDeployment CR](#machinedeployment-cr)
      - [MachineSet CR](#machineset-cr)
      - [Machine CR](#machine-cr)
      - [MachineHealthCheck CR](#machinehealthcheck-cr)
    - [Gaduation Criteria](#gaduation-criteria)
  - [Implementation History](#implementation-history)

## Glossary

- `CustomResource` (CR)
- `CustomResourceDefinition` (CRD)
- Cluster API State Metrics (CASM)

Refer to the [Cluster API Book Glossary].

## Summary

This proposal outlines a new experimental component for exposing metrics specific to CAPI's CRs.

This component is derived from  [mercedes-benz/cluster-api-state-metrics] and all the merits for its inception goes to the team that created it: [@chrischdi](https://github.com/chrischdi), [@seanschneeweiss](https://github.com/seanschneeweiss), [@tobiasgiese](https://github.com/tobiasgiese).

A special thank goes to [Mercedes-Benz] who allowed the donation of this project to CNCF and to the Cluster API community.

## Motivation

As of now CAPI controllers only expose metrics that are provided by controller-runtime, which in turn are specific to the controllers internal behavior and thus do not provide any information about the state of the clusters provisioned by CAPI.

This proposal introduces a new component that can generate a new set of metrics that will help end users to monitor the state of the Cluster API resources in their [OpenMetrics] compatible monitoring system of choice.

### Goals

- Define metrics to monitor the state of the Cluster API resources, thus solving parts of the [metrics umbrella issue #1477]
- Implementing a new component, which exposes the above metrics by following the [OpenMetrics] standard
- Make metrics part of the CAPI developer workflow by making them accessible via Tilt and possible also as a E2E test artifacts
- Implement metrics for core CAPI CRs

### Non-Goals

- Implement metrics for provider specific CAPI CRs

### Future Work

- Metrics for resources of Cluster API providers (e.g., CAPA, CAPO, CAPZ, ...)
- Collection of example alerts to e.g. alert when
  - a KubeadmControlPlane is not healthy.
  - 70% of my worker Machines are not healthy.
  - 70% of my Machines are blocked on deletion.

## Proposal

This proposal introduces a CAPI specific kube-state-metrics named `cluster-api-state-metrics` to expose metrics for CAPI specific CRs.

The implementation will be deployed as a separate component by using the provided yaml files. In future it may be integrated into the core provider manifest to deploy it automatically using `clusterctl`.

### User Stories

#### Story 1

As a service provider/cluster operator, I want to have metrics for the Cluster API CRs to create alerts for cluster lifecycle symptoms that might impact workloads availability.

#### Story 2

As an application developer, I would like to deploy cluster-api-state-metrics together with the prometheus stack via Tilt. This allows further analysis by using the metrics like measuring the duration of several provisioning phases.

### Implementation Details/Notes/Constraints

#### Scrapable Information

Following Cluster API CRDs currently exist.
The *In-scope* column marks CRDs for which metrics should be exposed.
In future iterations other CRs may be added or the cluster-api-state-metrics could be extended to support provider specific CRDs too.

| Name                      | API Group/Version                       | In-scope |
|---------------------------|-----------------------------------------|----------|
| Cluster                   | `cluster.x-k8s.io/v1beta1`              | yes      |
| ClusterClass              | `cluster.x-k8s.io/v1beta1`              | no       |
| MachineDeployment         | `cluster.x-k8s.io/v1beta1`              | yes      |
| MachineSet                | `cluster.x-k8s.io/v1beta1`              | yes      |
| Machine                   | `cluster.x-k8s.io/v1beta1`              | yes      |
| KubeadmConfig             | `bootstrap.cluster.x-k8s.io/v1beta1`    | no       |
| KubeadmConfigTemplate     | `bootstrap.cluster.x-k8s.io/v1beta1`    | no       |
| KubeadmControlPlane       | `controlplane.cluster.x-k8s.io/v1beta1` | yes      |
| ClusterResourceSetBinding | `addons.cluster.x-k8s.io/v1beta1`       | no       |
| ClusterResourceSet        | `addons.cluster.x-k8s.io/v1beta1`       | no       |
| MachineHealthCheck        | `cluster.x-k8s.io/v1beta1`              | yes      |
| MachinePool               | `cluster.x-k8s.io/v1beta1`              | yes      |

The relationships between the resources can be found in the [corresponding section in the cluster-api docs].

#### Relationship to kube-state-metrics

There are several CustomResources introduced by Cluster API and Cluster API providers that are similar to core Kubernetes resources.
Because of that it may make sense to implement metrics inspired by kube-state-metrics.

The following table lists possible mappings from core resources to CRDs:

| kube-state-metrics equivalent | implement for               |
|-------------------------------|-----------------------------|
| `Machine`                     | [Pod]                       |
| `MachineSet`                  | [ReplicaSet]                |
| `MachineDeployment`           | [Deployment]                |
| `KubeadmControlPlane`         | [Statefulset], [Deployment] |

CRDs missing in this table:

- `Cluster`
- `ClusterClass`
- `ClusterResourceSet`
- `ClusterResourceSetBinding`
- `MachineHealthCheck`
- `MachinePool`
- `KubeadmConfig`
- `KubeadmConfigTemplate`

The `Cluster` CR will have important information in their status fields similar to [Pod] metrics like `status.Ready` or `status.Conditions`.

Currently it is not important to implement metrics for `KubeadmConfig` and `KubeadmConfigTemplate` because both only contain configuration data (e.g., passed via cloud-init to the machine). However they may be compared to `ConfigMaps` or `Secrets`.

#### How does kube-state-metrics work

Kube-state-metrics exposes metrics by a http endpoint to be consumed by either Prometheus itself or a compatible scraper [[1]].

Since kube-state-metrics v1.5 large performance improvements got introduced to kube-state-metrics which are documented at the [Performance Optimization Proposal](https://github.com/kubernetes/kube-state-metrics/blob/master/docs/design/metrics-store-performance-optimization.md#Proposal). This document also explains the current internals of kube-state-metrics.

It caches the current state of the metrics using an internal cache and updates this internal state on add, update and delete events of watched resources.

On requests to `/metrics` the cached data gets concatenated to a single string and returned as a response.

#### Reuse kube-state-metrics packages

Cluster-api-state-metrics should re-use as many packages provided by kube-state-metrics as possible. This allows re-use of flags, configuration and extended functionality like sharding or tls configuration without additional implementation.

An [extension mechanism](https://github.com/kubernetes/kube-state-metrics/pull/1644) was introduced to the kube-state-metrics packages which allows using its basic mechanism but defining custom metrics for Custom Resources.
The `k8s.io/kube-state-metrics/v2/pkg/customresource.RegistryFactory`[[2]] interface was [introduced](https://github.com/kubernetes/kube-state-metrics/pull/1644) to allow defining custom metrics for Custom Resources while leveraging kube-state-metrics logic.

Cluster-api-state-metrics will have to implement the `customresource.RegistryFactory` interface for each custom resource.
The interface defines the function `MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator` to be implemented which then contains the specific metric implementations.
A detailed implementation example is available at the [package documentation](https://pkg.go.dev/k8s.io/kube-state-metrics/v2@v2.4.2/pkg/customresource#RegistryFactory).

These `customresource.RegistryFactory` implementations get used in a `main.go` which configures and starts the application by using `k8s.io/kube-state-metrics/v2/pkg/app.RunKubeStateMetrics(...)`[[3]].

#### Package structure for cluster-api-state-metrics

- `/exp/state-metrics/pkg/store` contains the metric implementation exposed by the `customresource.RegistryFactory`[[2]] interface.
  - `/exp/state-metrics/pkg/store/{cluster,kubeadmcontrolplane,machinedeployment,machine,...}.go` for the custom resource specific implementation of `customresource.RegistryFactory`
  - `/exp/state-metrics/pkg/store/factory.go` for implementing the `Factories()` function which groups and exposes the `customresource.RegistryFactory` implementations of this package
- `/exp/state-metrics/main.go` which:
  - imports the capi custom resource specific metric *factories* implemented and exposed via `/exp/state-metrics/pkg/store.Factories()`
  - imports and uses `k8s.io/kube-state-metrics/v2/pkg/options.NewOptions()`[[3]] to define the same cli flags and options as kube-state-metrics, except the enabled metrics.
  - imports and uses `k8s.io/kube-state-metrics/v2/pkg/app.RunKubeStateMetrics(...)`[[3]] to start the metrics server using the given options and the custom registry factory from `store.Factories()`.

### Security Model

- RBAC definitions should be generated via kubebuilder annotations.
- RBAC definitions should only grant `get`, `list` and `watch` permissions to CRs relevant for the application.

### Risks and Mitigations

This initial implementation provides a baseline on which incremental changes can be introduced in the future. Instead of encompassing all possible use cases under a single proposal, this proposal mitigates the risk of waiting too long to consider all required use cases regarding this topic.

## Alternatives

### Directly use kube-state-metrics

> [kube-state-metrics] is a simple service that listens to the Kubernetes API server and generates metrics about the state of the objects.

On a first thought, using kube-state-metrics would be a great fit to retrieve the desired metrics. However, kube-state-metrics does not plan to implement metrics for CRs of CRDs:

> There is no meaningful extension kube-state-metrics can do other than potentially providing a library to reuse the mechanisms built in this repository. [[4]]

The linked issue also states that:

> operators should expose metrics about the objects they expose themselves. [[4]]

Because of that kube-state-metrics itself does not fit this use-case.

### Expose metrics by the controllers

To solve this in the spirit of the kube-state-metrics maintainers it may make sense to implement the metrics directly within the CAPI controllers.
This might be a working solution but would impose following disadvantages:

- Requires changes in at least 3 controllers:
  - `kubeadm-bootstrap-controller`
  - `kubeadm-control-plane-controller`
  - `capi-controller-manager`
  - And potentially in every provider-specific controller, e.g.: `capd-controller-manager`
- Potential bugs of the metrics implementation may also have a negative impact on provisioning. E.g. if the controller runs out of memory or because of crashes of the application due to `invalid memory address` or `nil pointer dereference`.

Nevertheless, including metrics directly in the controllers may be valid for future iterations, but certainly not for an experimental feature.

## Upgrade Strategy

Cluster-api-state-metrics could follow the API versions of the CAPI controllers. By using a seperate go package using its own `go.mod` file we can prevent adding transitive dependencies to the core module. A `replace` directive inside the `go.mod` can ensure that always the same version will be used for the `sigs.k8s.io/cluster-api` dependency.

## Additional Details

### Metrics

#### Cluster CR

Common labels:

- `cluster=<cluster-name>`

| metric name                     | value                                       | type  | additional labels/tags                         | xref                    |
|---------------------------------|---------------------------------------------|-------|------------------------------------------------|-------------------------|
| `capi_cluster_labels`           | `1`                                         | Gauge | `.metadata.labels`                             | common                  |
| `capi_cluster_created`          | `.metadata.creationTimestamp`               | Gauge |                                                | common                  |
| `capi_cluster_paused`           | `.spec.paused` or `annotations.HasPaused()` | Gauge |                                                |                         |
| `capi_cluster_status_condition` | `.status.conditions==<condition>`           | Gauge | `condition=<condition>` `status=<true\|false>` |                         |
| `capi_cluster_status_phase`     | `.status.phase==<phase>`                    | Gauge | `phase=<phase>`                                | [Pod], [cluster phases] |

#### KubeadmControlPlane CR

Common labels:

- `kubeadmcontrolplane=<kubeadmcontrolplane-name>`

| metric name                                                      | value                                          | type  | additional labels/tags                              | xref         |
|------------------------------------------------------------------|------------------------------------------------|-------|-----------------------------------------------------|--------------|
| `capi_kubeadmcontrolplane_labels`                                | `1`                                            | Gauge | `.metadata.labels`                                  | common       |
| `capi_kubeadmcontrolplane_created`                               | `.metadata.creationTimestamp`                  | Gauge |                                                     | common       |
| `capi_kubeadmcontrolplane_paused`                                | `annotations.HasPaused()`                      | Gauge |                                                     |              |
| `capi_kubeadmcontrolplane_status_condition`                      | `.status.conditions==<condition>`              | Gauge | `condition=<condition>` `status=<true\|false>`      |              |
| `capi_kubeadmcontrolplane_status_replicas`                       | `.status.replicas`                             | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_status_replicas_ready`                 | `.status.readyReplicas`                        | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_status_replicas_unavailable`           | `.status.unavailableReplicas`                  | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_status_replicas_updated`               | `.status.updatedReplicas`                      | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_spec_replicas`                         | `.spec.replicas`                               | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_spec_strategy_rollingupdate_max_surge` | `.spec.rolloutStrategy.rollingUpdate.maxSurge` | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_info`                                  | `1`                                            | Gauge | `version=.spec.version`                             | [Pod]        |
| `capi_kubeadmcontrolplane_owner`                                 | `1`                                            | Gauge | `owner_kind=<owner kind>` `owner_name=<owner name>` | [ReplicaSet] |

#### MachineDeployment CR

Common labels:

- `machinedeployment=<machinedeployment-name>`

| metric name                                                          | value                                         | type  | additional labels/tags                              | xref                              |
|----------------------------------------------------------------------|-----------------------------------------------|-------|-----------------------------------------------------|-----------------------------------|
| `capi_machinedeployment_labels`                                      | `1`                                           | Gauge | `.metadata.labels`                                  | common                            |
| `capi_machinedeployment_created`                                     | `.metadata.creationTimestamp`                 | Gauge |                                                     | common                            |
| `capi_machinedeployment_paused`                                      | `.Spec.Paused` or `annotations.HasPaused()`   | Gauge |                                                     |                                   |
| `capi_machinedeployment_status_condition`                            | `.status.conditions==<condition>`             | Gauge | `condition=<condition>` `status=<true\|false>`      |                                   |
| `capi_machinedeployment_status_replicas`                             | `.status.replicas`                            | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_status_replicas_available`                   | `.status.availableReplicas`                   | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_status_replicas_unavailable`                 | `.status.unavailableReplicas`                 | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_status_replicas_updated`                     | `.status.updatedReplicas`                     | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_spec_replicas`                               | `.spec.replicas`                              | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_spec_strategy_rollingupdate_max_unavailable` | `.spec.strategy.rollingUpdate.maxUnavailable` | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_spec_strategy_rollingupdate_max_surge`       | `.spec.strategy.rollingUpdate.maxSurge`       | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_status_phase`                                | `.status.phase==<phase>`                      | Gauge | `phase=<phase>`                                     | [Pod], [machinedeployment phases] |
| `capi_machinedeployment_owner`                                       | `1`                                           | Gauge | `owner_kind=<owner kind>` `owner_name=<owner name>` | [ReplicaSet]                      |

#### MachineSet CR

Common labels:

- `machineset=<machineset-name>`

| metric name                                     | value                             | type  | additional labels/tags                              | xref         |
|-------------------------------------------------|-----------------------------------|-------|-----------------------------------------------------|--------------|
| `capi_machineset_labels`                        | `1`                               | Gauge | `.metadata.labels`                                  | common       |
| `capi_machineset_created`                       | `.metadata.creationTimestamp`     | Gauge |                                                     | common       |
| `capi_machineset_paused`                        | `annotations.HasPaused()`         | Gauge |                                                     |              |
| `capi_machineset_status_available_replicas`     | `.status.availableReplicas`       | Gauge |                                                     | [ReplicaSet] |
| `capi_machineset_status_condition`              | `.status.conditions==<condition>` | Gauge | `condition=<condition>` `status=<true\|false>`      |              |
| `capi_machineset_status_replicas`               | `.status.replicas`                | Gauge |                                                     | [ReplicaSet] |
| `capi_machineset_status_fully_labeled_replicas` | `.status.fullyLabeledReplicas`    | Gauge |                                                     | [ReplicaSet] |
| `capi_machineset_status_ready_replicas`         | `.status.readyReplicas`           | Gauge |                                                     | [ReplicaSet] |
| `capi_machineset_spec_replicas`                 | `.spec.replicas`                  | Gauge |                                                     | [ReplicaSet] |
| `capi_machineset_owner`                         | `1`                               | Gauge | `owner_kind=<owner kind>` `owner_name=<owner name>` | [ReplicaSet] |

#### Machine CR

Common labels:

- `machine=<machine-name>`

| metric name                     | value                             | type  | additional labels/tags                                                                                                                       | xref                    |
|---------------------------------|-----------------------------------|-------|----------------------------------------------------------------------------------------------------------------------------------------------|-------------------------|
| `capi_machine_labels`           | `1`                               | Gauge | `.metadata.labels`                                                                                                                           | common                  |
| `capi_machine_created`          | `.metadata.creationTimestamp`     | Gauge |                                                                                                                                              | common                  |
| `capi_machine_paused`           | `annotations.HasPaused()`         | Gauge |                                                                                                                                              |                         |
| `capi_machine_status_condition` | `.status.conditions==<condition>` | Gauge | `condition=<condition>`<br>`status=<true\|false>`                                                                                            |                         |
| `capi_machine_status_phase`     | `.status.phase==<phase>`          | Gauge | `phase=<phase>`                                                                                                                              | [Pod], [machine phases] |
| `capi_machine_owner`            | `1`                               | Gauge | `owner_kind=<owner kind>`<br>`owner_name=<owner name>`                                                                                       | [ReplicaSet]            |
| `capi_machine_info`             | `1`                               | Gauge | `internal_ip=<.status.addresses>`<br>`version=<.spec.version>`<br>`provider_id=<.spec.providerID>`<br>`failure_domain=<.spec.failureDomain>` | [Pod]                   |
| `capi_machine_status_noderef`   | `1`                               | Gauge | `node=<.status.nodeRef.name>`                                                                                                                |                         |

#### MachineHealthCheck CR

Common labels:

- `machinehealthcheck=<machinehealthcheck-name>`

| metric name                                           | value                             | type  | additional labels/tags                            | xref   |
|-------------------------------------------------------|-----------------------------------|-------|---------------------------------------------------|--------|
| `capi_machinehealthcheck_labels`                      | `1`                               | Gauge | `.metadata.labels`                                | common |
| `capi_machinehealthcheck_created`                     | `.metadata.creationTimestamp`     | Gauge |                                                   | common |
| `capi_machinehealthcheck_paused`                      | `annotations.HasPaused()`         | Gauge |                                                   |        |
| `capi_machinehealthcheck_status_condition`            | `.status.conditions==<condition>` | Gauge | `condition=<condition>`<br>`status=<true\|false>` |        |
| `capi_machinehealthcheck_status_expected_machines`    | `.status.expectedMachines`        | Gauge |                                                   |        |
| `capi_machinehealthcheck_status_current_healthy`      | `.status.currentHealthy`          | Gauge |                                                   |        |
| `capi_machinehealthcheck_status_remediations_allowed` | `.status.remediationsAllowed`     | Gauge |                                                   |        |

### Gaduation Criteria

The initial plan is to add cluster-api-state-metrics as an experimental feature under `exp/` and allow it to be enabled via `tilt`.

## Implementation History

- [x] 03/02/2022: Proposed idea in an issue or [community meeting]
- [x] 03/15/2022: Compile a Google Doc following the CAEP template
- [x] 03/16/2022: Present proposal at a [community meeting]
- [x] 03/16/2022: First round of feedback from community
- [x] 04/11/2022: Open proposal PR

<!-- Links -->

[Cluster API Book Glossary]: https://cluster-api.sigs.k8s.io/reference/glossary.html
[mercedes-benz/cluster-api-state-metrics]: https://github.com/mercedes-benz/cluster-api-state-metrics/
[Mercedes-Benz]: https://opensource.mercedes-benz.com/
[OpenMetrics]: https://github.com/OpenObservability/OpenMetrics/blob/main/specification/OpenMetrics.md
[metrics umbrella issue #1477]: https://github.com/kubernetes-sigs/cluster-api/issues/1477
[corresponding section in the cluster-api docs]: https://cluster-api.sigs.k8s.io/developer/crd-relationships.html#worker-machines-relationships
[Deployment]: https://github.com/kubernetes/kube-state-metrics/blob/master/docs/deployment-metrics.md
[ReplicaSet]: https://github.com/kubernetes/kube-state-metrics/blob/master/docs/replicaset-metrics.md
[Pod]: https://github.com/kubernetes/kube-state-metrics/blob/master/docs/pod-metrics.md
[StatefulSet]: https://github.com/kubernetes/kube-state-metrics/blob/master/docs/statefulset-metrics.md
[kube-state-metrics]: https://github.com/kubernetes/kube-state-metrics
[1]: https://github.com/kubernetes/kube-state-metrics
[2]: https://github.com/kubernetes/kube-state-metrics/blob/master/pkg/customresource/registry_factory.go#L29
[3]: https://github.com/kubernetes/kube-state-metrics/blob/master/pkg/app/server.go
[4]: https://github.com/kubernetes/kube-state-metrics/issues/457
[machine phases]: https://github.com/kubernetes-sigs/cluster-api/blob/main/api/v1beta1/machine_phase_types.go
[cluster phases]: https://github.com/kubernetes-sigs/cluster-api/blob/main/api/v1beta1/cluster_phase_types.go
[machinedeployment phases]: https://github.com/kubernetes-sigs/cluster-api/blob/07c0a4809361927b15cde2747b34142b7c7ead15/api/v1beta1/machinedeployment_types.go#L222-L224
[community meeting]: https://docs.google.com/document/d/1ushaVqAKYnZ2VN_aa3GyKlS4kEd6bSug13xaXOakAQI
