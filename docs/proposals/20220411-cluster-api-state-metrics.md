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
last-updated: 2022-09-07
status: experimental
---

# Cluster API State Metrics

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

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
  - [Use kube-state-metrics custom resource configuration](#use-kube-state-metrics-custom-resource-configuration)
  - [Security Model](#security-model)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
  - [Reuse kube-state-metrics packages](#reuse-kube-state-metrics-packages)
    - [Package structure for cluster-api-state-metrics](#package-structure-for-cluster-api-state-metrics)
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

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Glossary

- `CustomResource` (CR)
- `CustomResourceDefinition` (CRD)
- Cluster API State Metrics (CASM)

Refer to the [Cluster API Book Glossary].

## Summary

This proposal outlines adding kube-state-metrics as a new component for exposing metrics specific to CAPI's CRs.

This solution is derived from [mercedes-benz/cluster-api-state-metrics] and all the merits for its inception goes to the team that created it: [@chrischdi](https://github.com/chrischdi), [@seanschneeweiss](https://github.com/seanschneeweiss), [@tobiasgiese](https://github.com/tobiasgiese).

A special thank goes to [Mercedes-Benz] who allowed the donation of this project to CNCF and to the Cluster API community.

Also it builds up on the custom resource feature of [kube-state-metrics] which was introduced in v2.5.0 and improved in v2.6.0.

## Motivation

As of now CAPI controllers only expose metrics that are provided by controller-runtime, which in turn are specific to the controllers internal behavior and thus do not provide any information about the state of the clusters provisioned by CAPI.

This proposal introduces a kube-state-metrics and Custom Resource configuration to generate a new set of metrics that will help end users to monitor the state of the Cluster API resources in their [OpenMetrics] compatible monitoring system of choice.

### Goals

- Define metrics to monitor the state of the Cluster API resources, thus solving parts of the [metrics umbrella issue #1477]
- Adding Custom Resource configuration for kube-state-metrics, which then exposes the below metrics by following the [OpenMetrics] standard
- Make metrics part of the CAPI developer workflow by making them accessible via Tilt
- Implement metrics for core CAPI CRs

### Non-Goals

- Implement metrics for provider specific CAPI CRs

### Future Work

- Metrics for resources of Cluster API providers (e.g., CAPA, CAPO, CAPZ, ...)
- Collection of example alerts to e.g. alert when
  - a KubeadmControlPlane is not healthy.
  - 70% of my worker Machines are not healthy.
  - 70% of my Machines are blocked on deletion.
- Auto-generation of the metric definition from markers at the type definitions.
- Introduction of generic `*_labels` metrics which work in the same way as `kube_*_labels` metrics.
  - This needs implementation on kube-state-metrics side to also include using the configuration from the existing flags.
  - For now, a custom user-specified `Info` metric could get configured to create a customized `*_labels` metric which exposes explicitly listed labels.

## Proposal

This proposal introduces a CAPI specific configuration for kube-state-metrics to expose metrics for CAPI specific CRs.

The configuration could be deployed using the kube-state-metrics helm chart and should leverage the custom-resource configuration file.

In future the configuration for kube-state-metrics may be added to the release artifacts of CAPI.

### User Stories

#### Story 1

As a service provider/cluster operator, I want to have metrics for the Cluster API CRs to create alerts for cluster lifecycle symptoms that might impact workloads availability.

#### Story 2

As an application developer, I would like to deploy kube-state-metrics including the CAPI configuration together with the prometheus stack via Tilt. This allows further analysis by using the metrics like measuring the duration of several provisioning phases.

### Implementation Details/Notes/Constraints

#### Scrapable Information

Following Cluster API CRDs currently exist.
The *In-scope* column marks CRDs for which metrics should be exposed.
In future iterations other CRs or configuration for provider specific CRDs could be added.

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

### Use kube-state-metrics custom resource configuration

kube-state-metrics v2.5.0 introduced an experimental feature to create metrics for CRDs.
The feature for v2.5.0 did not fulfill all requirements for the metrics of this proposal.
Because of that improvements got contributed to kube-state-metrics to improve the feature and support more use-cases, including the metrics of this proposal.

For the implementation of this proposal a proper configuration file for kube-state-metrics should be provided and kube-state-metrics should be used to expose the metrics.

### Security Model

- RBAC definitions should be generated for markers at the type definitions in future.
- RBAC definitions should only grant `get`, `list` and `watch` permissions to CRs relevant for the application.

### Risks and Mitigations

This initial implementation provides a baseline on which incremental changes can be introduced in the future. Instead of encompassing all possible use cases under a single proposal, this proposal mitigates the risk of waiting too long to consider all required use cases regarding this topic.

## Alternatives

### Reuse kube-state-metrics packages

Cluster-api-state-metrics could re-use packages provided by kube-state-metrics to implement the metrics. This allows re-use of flags, configuration and extended functionality like sharding or tls configuration without additional implementation.

When first writing the proposal, this was the favoured option because at that state, kube-state-metrics did not support configuration for custom resource metrics. As of today this is not the case anymore.

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

### Expose metrics by the controllers

It may make sense to implement the metrics directly within the CAPI controllers.
This might be a working solution but would impose following disadvantages:

- Requires changes in at least 3 controllers:
  - `kubeadm-bootstrap-controller`
  - `kubeadm-control-plane-controller`
  - `capi-controller-manager`
  - And potentially in every provider-specific controller, e.g.: `capd-controller-manager`
- Potential bugs of the metrics implementation may also have a negative impact on provisioning. E.g. if the controller runs out of memory or because of crashes of the application due to `invalid memory address` or `nil pointer dereference`.

Nevertheless, including metrics directly in the controllers may be valid for future iterations, but certainly not for an experimental feature.

## Upgrade Strategy

After being introduced and validated to be valuable, the configuration file for kube-state-metrics could be provided as release artifact.
A user could also update its kube-state-metrics configuration after upgrading Cluster API to expose the latest compatible metrics.
The conversion webhooks provided by the controllers for the Custom Resources allow to still expose metrics during the version shift, even when the default APIVersion gets changed by the Cluster API upgrade.

## Additional Details

### Metrics

#### Cluster CR

Common labels:

- `cluster=<cluster-name>`

| metric name                        | value                             | type  | additional labels/tags                                                                                                                                                                                               | xref                    |
|------------------------------------|-----------------------------------|-------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------|
| `capi_cluster_created`             | `.metadata.creationTimestamp`     | Gauge |                                                                                                                                                                                                                      | common                  |
| `capi_cluster_annotation_paused` * | `1`                               | Gauge | `paused_value=<paused annotation value>`                                                                                                                                                                             |                         |
| `capi_cluster_info`                | `1`                               | Gauge |                                                                                                                                                                                                                      |                         |
| `capi_cluster_spec_paused`         | `.spec.paused`                    | Gauge | `topology_version=.spec.topology.version`<br>`topology_class=.spec.topology.class`<br>`control_plane_endpoint_host=.spec.controlPlaneEndpoint.host`<br>`control_plane_endpoint_port=.spec.controlPlaneEndpoint.port` |                         |
| `capi_cluster_status_condition`    | `.status.conditions==<condition>` | Gauge | `condition=<condition>` `status=<true\|false>`                                                                                                                                                                       |                         |
| `capi_cluster_status_phase`        | `.status.phase==<phase>`          | Gauge | `phase=<phase>`                                                                                                                                                                                                      | [Pod], [cluster phases] |

*: A metric will only be exposed if the annotation existst. If so it will always have a value of `1` and expose a label which contains its value. Prometheus would drop labels having an empty value, which is why an empty value would be equal to a not set annotation otherwise.

#### KubeadmControlPlane CR

Common labels:

- `kubeadmcontrolplane=<kubeadmcontrolplane-name>`

| metric name                                                      | value                                          | type  | additional labels/tags                              | xref         |
|------------------------------------------------------------------|------------------------------------------------|-------|-----------------------------------------------------|--------------|
| `capi_kubeadmcontrolplane_created`                               | `.metadata.creationTimestamp`                  | Gauge |                                                     | common       |
| `capi_kubeadmcontrolplane_annotation_paused` *                   | `1`                                            | Gauge | `paused_value=<paused annotation value>`            |              |
| `capi_kubeadmcontrolplane_status_condition`                      | `.status.conditions==<condition>`              | Gauge | `condition=<condition>` `status=<true\|false>`      |              |
| `capi_kubeadmcontrolplane_status_replicas`                       | `.status.replicas`                             | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_status_replicas_ready`                 | `.status.readyReplicas`                        | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_status_replicas_unavailable`           | `.status.unavailableReplicas`                  | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_status_replicas_updated`               | `.status.updatedReplicas`                      | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_spec_replicas`                         | `.spec.replicas`                               | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_spec_strategy_rollingupdate_max_surge` | `.spec.rolloutStrategy.rollingUpdate.maxSurge` | Gauge |                                                     | [Deployment] |
| `capi_kubeadmcontrolplane_info`                                  | `1`                                            | Gauge | `version=.spec.version`                             | [Pod]        |
| `capi_kubeadmcontrolplane_owner`                                 | `1`                                            | Gauge | `owner_kind=<owner kind>` `owner_name=<owner name>` | [ReplicaSet] |

*: A metric will only be exposed if the annotation existst. If so it will always have a value of `1` and expose a label which contains its value. Prometheus would drop labels having an empty value, which is why an empty value would be equal to a not set annotation otherwise.

#### MachineDeployment CR

Common labels:

- `machinedeployment=<machinedeployment-name>`

| metric name                                                          | value                                         | type  | additional labels/tags                              | xref                              |
|----------------------------------------------------------------------|-----------------------------------------------|-------|-----------------------------------------------------|-----------------------------------|
| `capi_machinedeployment_created`                                     | `.metadata.creationTimestamp`                 | Gauge |                                                     | common                            |
| `capi_machinedeployment_annotation_paused` *                         | `1`                                           | Gauge | `paused_value=<paused annotation value>`            |                                   |
| `capi_machinedeployment_spec_paused`                                 | `.Spec.Paused`                                | Gauge |                                                     |                                   |
| `capi_machinedeployment_status_condition`                            | `.status.conditions==<condition>`             | Gauge | `condition=<condition>` `status=<true\|false>`      |                                   |
| `capi_machinedeployment_status_replicas`                             | `.status.replicas`                            | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_status_replicas_available`                   | `.status.availableReplicas`                   | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_status_replicas_ready`                       | `.status.readyReplicas`                       | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_status_replicas_unavailable`                 | `.status.unavailableReplicas`                 | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_status_replicas_updated`                     | `.status.updatedReplicas`                     | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_spec_replicas`                               | `.spec.replicas`                              | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_spec_strategy_rollingupdate_max_unavailable` | `.spec.strategy.rollingUpdate.maxUnavailable` | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_spec_strategy_rollingupdate_max_surge`       | `.spec.strategy.rollingUpdate.maxSurge`       | Gauge |                                                     | [Deployment]                      |
| `capi_machinedeployment_status_phase`                                | `.status.phase==<phase>`                      | Gauge | `phase=<phase>`                                     | [Pod], [machinedeployment phases] |
| `capi_machinedeployment_owner`                                       | `1`                                           | Gauge | `owner_kind=<owner kind>` `owner_name=<owner name>` | [ReplicaSet]                      |

*: A metric will only be exposed if the annotation existst. If so it will always have a value of `1` and expose a label which contains its value. Prometheus would drop labels having an empty value, which is why an empty value would be equal to a not set annotation otherwise.

#### MachineSet CR

Common labels:

- `machineset=<machineset-name>`

| metric name                                     | value                             | type  | additional labels/tags                              | xref         |
|-------------------------------------------------|-----------------------------------|-------|-----------------------------------------------------|--------------|
| `capi_machineset_created`                       | `.metadata.creationTimestamp`     | Gauge |                                                     | common       |
| `capi_machineset_annotation_paused` *           | `1`                               | Gauge | `paused_value=<paused annotation value>`            |              |
| `capi_machineset_status_available_replicas`     | `.status.availableReplicas`       | Gauge |                                                     | [ReplicaSet] |
| `capi_machineset_status_condition`              | `.status.conditions==<condition>` | Gauge | `condition=<condition>` `status=<true\|false>`      |              |
| `capi_machineset_status_replicas`               | `.status.replicas`                | Gauge |                                                     | [ReplicaSet] |
| `capi_machineset_status_fully_labeled_replicas` | `.status.fullyLabeledReplicas`    | Gauge |                                                     | [ReplicaSet] |
| `capi_machineset_status_ready_replicas`         | `.status.readyReplicas`           | Gauge |                                                     | [ReplicaSet] |
| `capi_machineset_spec_replicas`                 | `.spec.replicas`                  | Gauge |                                                     | [ReplicaSet] |
| `capi_machineset_owner`                         | `1`                               | Gauge | `owner_kind=<owner kind>` `owner_name=<owner name>` | [ReplicaSet] |

*: A metric will only be exposed if the annotation existst. If so it will always have a value of `1` and expose a label which contains its value. Prometheus would drop labels having an empty value, which is why an empty value would be equal to a not set annotation otherwise.

#### Machine CR

Common labels:

- `machine=<machine-name>`

| metric name                        | value                             | type  | additional labels/tags                                                                                                                       | xref                    |
|------------------------------------|-----------------------------------|-------|----------------------------------------------------------------------------------------------------------------------------------------------|-------------------------|
| `capi_machine_created`             | `.metadata.creationTimestamp`     | Gauge |                                                                                                                                              | common                  |
| `capi_machine_annotation_paused` * | `1`                               | Gauge | `paused_value=<paused annotation value>`                                                                                                     |                         |
| `capi_machine_status_condition`    | `.status.conditions==<condition>` | Gauge | `condition=<condition>`<br>`status=<true\|false>`                                                                                            |                         |
| `capi_machine_status_phase`        | `.status.phase==<phase>`          | Gauge | `phase=<phase>`                                                                                                                              | [Pod], [machine phases] |
| `capi_machine_owner`               | `1`                               | Gauge | `owner_kind=<owner kind>`<br>`owner_name=<owner name>`                                                                                       | [ReplicaSet]            |
| `capi_machine_info`                | `1`                               | Gauge | `internal_ip=<.status.addresses>`<br>`version=<.spec.version>`<br>`provider_id=<.spec.providerID>`<br>`failure_domain=<.spec.failureDomain>` | [Pod]                   |
| `capi_machine_status_noderef`      | `1`                               | Gauge | `node=<.status.nodeRef.name>`                                                                                                                |                         |

*: A metric will only be exposed if the annotation existst. If so it will always have a value of `1` and expose a label which contains its value. Prometheus would drop labels having an empty value, which is why an empty value would be equal to a not set annotation otherwise.

#### MachineHealthCheck CR

Common labels:

- `machinehealthcheck=<machinehealthcheck-name>`

| metric name                                           | value                             | type  | additional labels/tags                                 | xref         |
|-------------------------------------------------------|-----------------------------------|-------|--------------------------------------------------------|--------------|
| `capi_machinehealthcheck_created`                     | `.metadata.creationTimestamp`     | Gauge |                                                        | common       |
| `capi_machinehealthcheck_annotation_paused` *         | `1`                               | Gauge | `paused_value=<paused annotation value>`               |              |
| `capi_machinehealthcheck_owner`                       | `1`                               | Gauge | `owner_kind=<owner kind>`<br>`owner_name=<owner name>` | [ReplicaSet] |
| `capi_machinehealthcheck_status_condition`            | `.status.conditions==<condition>` | Gauge | `condition=<condition>`<br>`status=<true\|false>`      |              |
| `capi_machinehealthcheck_status_expected_machines`    | `.status.expectedMachines`        | Gauge |                                                        |              |
| `capi_machinehealthcheck_status_current_healthy`      | `.status.currentHealthy`          | Gauge |                                                        |              |
| `capi_machinehealthcheck_status_remediations_allowed` | `.status.remediationsAllowed`     | Gauge |                                                        |              |

*: A metric will only be exposed if the annotation existst. If so it will always have a value of `1` and expose a label which contains its value. Prometheus would drop labels having an empty value, which is why an empty value would be equal to a not set annotation otherwise.

### Gaduation Criteria

The initial plan is to add kube-state-metrics as to the `./hack/observability` directory and allow it to be enabled via `tilt`.

## Implementation History

- [x] 09/07/2022: Updated proposal to match current implementation state, removed `_labels` metrics due to lack of functionality in kube-state-metrics
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
[Deployment]: https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/workload/deployment-metrics.md
[ReplicaSet]: https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/workload/replicaset-metrics.md
[Pod]: https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/workload/pod-metrics.md
[StatefulSet]: https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/workload/statefulset-metrics.md
[kube-state-metrics]: https://github.com/kubernetes/kube-state-metrics
[1]: https://github.com/kubernetes/kube-state-metrics
[2]: https://github.com/kubernetes/kube-state-metrics/blob/master/pkg/customresource/registry_factory.go#L29
[3]: https://github.com/kubernetes/kube-state-metrics/blob/master/pkg/app/server.go
[4]: https://github.com/kubernetes/kube-state-metrics/issues/457
[machine phases]: https://github.com/kubernetes-sigs/cluster-api/blob/main/api/v1beta1/machine_phase_types.go
[cluster phases]: https://github.com/kubernetes-sigs/cluster-api/blob/main/api/v1beta1/cluster_phase_types.go
[machinedeployment phases]: https://github.com/kubernetes-sigs/cluster-api/blob/07c0a4809361927b15cde2747b34142b7c7ead15/api/v1beta1/machinedeployment_types.go#L222-L224
[community meeting]: https://docs.google.com/document/d/1ushaVqAKYnZ2VN_aa3GyKlS4kEd6bSug13xaXOakAQI
