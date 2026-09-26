# Experimental Feature: MachinePoolPreflightChecks (alpha)

The `MachinePoolPreflightChecks` feature can provide additional safety by surfacing when a MachinePool's desired Kubernetes version is unsafe relative to the Control Plane, for example when it would violate the Kubernetes or kubeadm version skew, or when the Control Plane is not yet stable.

As with [`MachineSetPreflightChecks`](./machineset-preflight-checks.md), a failing check is not treated as a controller error, whereas a check that cannot be *evaluated* (e.g. the Control Plane cannot be read) is treated as a genuine error and retried with backoff.

The one intentional difference from MachineSets is what a failing check does. MachineSets gate Machine creation and remediation until the checks pass; MachinePool preflight checks do not block or otherwise gate any scaling operation, because the infrastructure provider (not the MachinePool controller) owns MachinePool scaling. Instead, a failing check surfaces via:

* the `PreflightCheckSucceeded` condition (set to `False`), which is included in the MachinePool `Ready` condition summary, and
* a `Warning` event on the MachinePool.

A failing check also requeues the MachinePool on a short, fixed timer so that `Ready` refreshes promptly once the underlying condition (e.g. a Control Plane upgrade) resolves; this is only a re-check timer and never gates or delays scaling. As with the MachineSet preflight condition, a MachinePool may therefore transiently report `Ready=False` (for example while the Control Plane is upgrading) and self-heal once the check passes again.

**Feature gate name**: `MachinePoolPreflightChecks`

**Variable name to enable/disable the feature gate**: `EXP_MACHINE_POOL_PREFLIGHT_CHECKS`

Note: The `MachinePool` feature gate (`EXP_MACHINE_POOL`) must also be enabled.

## Supported PreflightChecks

### `ControlPlaneIsStable`

* This preflight check ensures that the ControlPlane is currently stable i.e. the ControlPlane is currently neither provisioning, upgrading.
* For Clusters with a managed topology it also checks if a control plane upgrade is pending.
* This preflight check is only performed if:
  * The Cluster uses a ControlPlane provider.
  * ControlPlane version is defined (`ControlPlane.spec.version` is set).

### `KubernetesVersionSkew`

* This preflight check ensures that the MachinePool and the ControlPlane conform to the [Kubernetes version skew](https://kubernetes.io/releases/version-skew-policy/#kubelet).
* This preflight check is only performed if:
    * The Cluster uses a ControlPlane provider.
    * ControlPlane version is defined (`ControlPlane.spec.version` is set).
    * MachinePool version is defined (`MachinePool.spec.template.spec.version` is set).

### `KubeadmVersionSkew`

* This preflight check ensures that the MachinePool and the ControlPlane conform to the [kubeadm version skew](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/#kubeadm-s-skew-against-kubeadm).
* This preflight check is only performed if:
  * The Cluster uses a ControlPlane provider.
  * ControlPlane version is defined (`ControlPlane.spec.version` is set).
  * MachinePool version is defined (`MachinePool.spec.template.spec.version` is set).
  * MachinePool uses the `Kubeadm` Bootstrap provider.

### `ControlPlaneVersionSkew`

* This preflight check ensures that the MachinePool and the ControlPlane have the same version. The idea behind this
  check is that it doesn't make sense to keep a Machine on an old version, if we already know based on the control
  plane version that the version has to be propagated soon.
* This preflight check is only performed if:
  * The Cluster has a managed topology
  * The Cluster uses a ControlPlane provider.
  * ControlPlane version is defined (`ControlPlane.spec.version` is set).
  * MachinePool version is defined (`MachinePool.spec.template.spec.version` is set).

## Configuring MachinePool PreflightChecks

Per default all preflight checks are enabled for all MachinePools including new and existing MachinePools.
The enabled preflight checks can be overwritten with the `--machinepool-preflight-checks` command-line flag.

It is also possible to opt-out of one or all of the preflight checks on a per MachinePool basis by specifying a
comma-separated list of the preflight checks via the `machinepool.cluster.x-k8s.io/skip-preflight-checks` annotation
on the MachinePool or on the corresponding BootstrapConfigTemplate (annotation on the MachinePool has higher priority).

Examples:
* To opt out of all the preflight checks set the `machinepool.cluster.x-k8s.io/skip-preflight-checks: All` annotation.
* To opt out of the `ControlPlaneIsStable` preflight check set the `machinepool.cluster.x-k8s.io/skip-preflight-checks: ControlPlaneIsStable` annotation.
* To opt out of multiple preflight checks set the `machinepool.cluster.x-k8s.io/skip-preflight-checks: ControlPlaneIsStable,KubernetesVersionSkew` annotation.
