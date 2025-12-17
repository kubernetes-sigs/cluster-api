# Implementing Upgrade Plan Runtime Extensions

<aside class="note warning">

<h1>Caution</h1>

Please note Runtime SDK is an advanced feature. If implemented incorrectly, a failing Runtime Extension can severely impact the Cluster API runtime.

</aside>

## Introduction

The proposal for [Chained and efficient upgrades](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/20250513-chained-and-efficient-upgrades-for-clusters-with-managed-topologies.md)
introduced support for upgrading by more than one minor when working with Clusters using managed topologies.

According to the proposal, there are two ways to provide Cluster API the information required to compute the upgrade plan:
- By setting the list of versions in the `spec.kubernetesVersions` field in the `ClusterClass` object.
- By calling the runtime hook defined in the `spec.upgrade` field in the `ClusterClass` object.

This document defines the hook for the second option and provides recommendations on how to implement it.

<!-- TOC -->
* [Implementing Upgrade Plan Runtime Extensions](#implementing-upgrade-plan-runtime-extensions)
  * [Introduction](#introduction)
  * [Guidelines](#guidelines)
  * [Definitions](#definitions)
    * [GenerateUpgradePlan](#generateupgradeplan)
<!-- TOC -->

## Guidelines

All guidelines defined in [Implementing Runtime Extensions](implement-extensions.md#guidelines) apply to the
implementation of Runtime Extensions for upgrade plan hooks as well.

In summary, Runtime Extensions are components that should be designed, written and deployed with great caution given
that they can affect the proper functioning of the Cluster API runtime. A poorly implemented Runtime Extension could
potentially block upgrades.

Following recommendations are especially relevant:

* [Idempotence](implement-extensions.md#idempotence)
* [Deterministic result](implement-extensions.md#deterministic-result)
* [Error messages](implement-extensions.md#error-messages)
* [Error management](implement-extensions.md#error-management)
* [Avoid dependencies](implement-extensions.md#avoid-dependencies)

## Definitions

For additional details about the OpenAPI spec of the upgrade plan hooks, please download the [`runtime-sdk-openapi.yaml`]({{#releaselink repo:"https://github.com/kubernetes-sigs/cluster-api" gomodule:"sigs.k8s.io/cluster-api" asset:"runtime-sdk-openapi.yaml" version:"1.12.x"}})
file and then open it from the [Swagger UI](https://editor.swagger.io/).

### GenerateUpgradePlan

The GenerateUpgradePlan hook is called every time Cluster API is required to compute the upgrade plan.

Notably, during an upgrade, the upgrade plan is recomputed several times, ideally once each time the upgrade plan completes
a step, but the number of calls might be higher depending on e.g. the duration of the upgrade.

Example Request:

```yaml
apiVersion: hooks.runtime.cluster.x-k8s.io/v1alpha1
kind: GenerateUpgradePlanRequest
settings: <Runtime Extension settings>
cluster:
  apiVersion: cluster.x-k8s.io/v1beta1
  kind: Cluster
  metadata:
    name: test-cluster
    namespace: test-ns
  spec:
    ...
  status:
    ...
fromKubernetesVersion: "v1.29.0"
toKubernetesVersion: "v1.33.0"
```

Example Response:

```yaml
apiVersion: hooks.runtime.cluster.x-k8s.io/v1alpha1
kind: GenerateUpgradePlanResponse
status: Success # or Failure
message: "error message if status == Failure"
controlPlaneUpgrades:
- version: v1.30.0
- version: v1.31.0
- version: v1.32.3
- version: v1.33.0
 ```

Note: in this case the system will infer the list of intermediate version for workers from the list of control plane versions, taking
care of performing the minimum number of workers upgrade by taking into account the [Kubernetes version skew policy](https://kubernetes.io/releases/version-skew-policy/).

Implementers of this runtime extension can also address more sophisticated use cases by computing the response in different ways, e.g.

- Go through more patch release for a minor if necessary, e.g., v1.30.0 -> v1.30.1 -> etc.

  ```yaml
  ...
  controlPlaneUpgrades:
  - version: v1.30.0
  - version: v1.30.1
  - ...
  ```

Note: in this case the system will infer the list of intermediate version for workers from the list of control plane versions, taking
care of performing the minimum number of workers upgrade by taking into account the [Kubernetes version skew policy](https://kubernetes.io/releases/version-skew-policy/).

- Force workers to upgrade to specific versions, e.g., force workers upgrade to v1.30.0 when doing v1.29.0 -> v1.32.3
  (in this example, worker upgrade to 1.30.0 is not required by the [Kubernetes version skew policy](https://kubernetes.io/releases/version-skew-policy/), so it would
  be skipped under normal circumstances).

  ```yaml
  ...
  controlPlaneUpgrades:
  - version: v1.30.0
  - version: v1.31.0
  - version: v1.32.3
  workersUpgrades:
  - version: v1.30.0
  - version: v1.32.3
  ```

Note: in this case the system will take into consideration the provided `workersUpgrades`, and validated it is
consistent with `controlPlaneUpgrades` and also compliant with the [Kubernetes version skew policy](https://kubernetes.io/releases/version-skew-policy/).

- Force workers to upgrade to all the intermediate steps (opt out from efficient upgrades).

  ```yaml
  ...
  controlPlaneUpgrades:
  - version: v1.30.0
  - version: v1.31.0
  - version: v1.32.3
  workersUpgrades:
  - version: v1.30.0
  - version: v1.31.0
  - version: v1.32.3
  ```

Note: in this case the system will take into consideration the provided `workersUpgrades`, and validate it is
consistent with `controlPlaneUpgrades` and also compliant with the [Kubernetes version skew policy](https://kubernetes.io/releases/version-skew-policy/).

In all the cases above, the `GenerateUpgradePlanResponse` content must comply the following validation rules:

- `controlPlaneUpgrades` is the list of version upgrade steps for the control plane; it must be always specified
  unless the control plane is already at the target version.
  - there should be at least one version for every minor between `fromControlPlaneKubernetesVersion` (excluded) and `toKubernetesVersion` (included).
  - each version must be:
    - greater than `fromControlPlaneKubernetesVersion` (or with a different build number)
    - greater than the previous version in the list (or with a different build number)
    - less or equal to `toKubernetesVersion` (or with a different build number)
    - the last version in the plan must be equal to `toKubernetesVersion`

- `workersUpgrades` is the list of version upgrade steps for the workers.
  - In case the upgrade plan for workers will be left to empty, the system will automatically
    determine the minimal number of workers upgrade steps, thus minimizing impact on workloads and reducing
    the overall upgrade time.
  - If instead for any reason a custom upgrade plan for workers is required, `workersUpgrades` should be set and 
    the following rules apply to each version in the list. More specifically, each version must be:
    - equal to `fromControlPlaneKubernetesVersion` or to one of the versions in the control plane upgrade plan.
    - greater than `fromWorkersKubernetesVersion` (or with a different build number)
    - greater than the previous version in the list (or with a different build number)
    - less or equal to the `toKubernetesVersion` (or with a different build number)
    - in case of versions with the same major/minor/patch version but different build number, also the order
      of those versions must be the same for control plane and worker upgrade plan.
    - the last version in the plan must be equal to `toKubernetesVersion`
    - the upgrade plan must have all the intermediate version which workers must go through to avoid breaking rules
      defining the max version skew between control plane and workers.
