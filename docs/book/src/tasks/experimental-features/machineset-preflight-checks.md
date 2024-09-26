# Experimental Feature: MachineSetPreflightChecks (beta)

The `MachineSetPreflightChecks` feature can provide additional safety while creating new Machines and remediating existing unhealthy Machines of a MachineSet.

When a MachineSet creates machines under certain circumstances, the operation fails or leads to a new machine that will be deleted and recreated in a short timeframe,
leading to unwanted Machine churn. Some of these circumstances include, but not limited to, creating a new Machine when Kubernetes version skew could be violated or 
joining a Machine when the Control Plane is upgrading leading to failure because of mixed kube-apiserver version or due to the cluster load balancer delays in adapting
to the changes.

Enabling `MachineSetPreflightChecks` provides safety in such circumstances by making sure that a Machine is only created when it is safe to do so.


**Feature gate name**: `MachineSetPreflightChecks`

**Variable name to enable/disable the feature gate**: `EXP_MACHINE_SET_PREFLIGHT_CHECKS`

## Supported PreflightChecks

### `ControlPlaneIsStable`

* This preflight check ensures that the ControlPlane is currently stable i.e. the ControlPlane is currently neither provisioning nor upgrading.   
* This preflight check is only performed if:
  * The Cluster uses a ControlPlane provider.
  * ControlPlane version is defined (`ControlPlane.spec.version` is set).

### `KubernetesVersionSkew`

* This preflight check ensures that the MachineSet and the ControlPlane conform to the [Kubernetes version skew](https://kubernetes.io/releases/version-skew-policy/#kubelet).
* This preflight check is only performed if:
    * The Cluster uses a ControlPlane provider.
    * ControlPlane version is defined (`ControlPlane.spec.version` is set).
    * MachineSet version is defined (`MachineSet.spec.template.spec.version` is set).

### `KubeadmVersionSkew`

* This preflight check ensures that the MachineSet and the ControlPlane conform to the [kubeadm version skew](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/#kubeadm-s-skew-against-kubeadm).
* This preflight check is only performed if:
  * The Cluster uses a ControlPlane provider.
  * ControlPlane version is defined (`ControlPlane.spec.version` is set).
  * MachineSet version is defined (`MachineSet.spec.template.spec.version` is set).
  * MachineSet uses the `Kubeadm` Bootstrap provider.

## Opting out of PreflightChecks

Once the feature flag is enabled the preflight checks are enabled for all the MachineSets including new and existing MachineSets.
It is possible to opt-out of one or all of the preflight checks on a per MachineSet basis by specifying a comma-separated list of the preflight checks on the 
`machineset.cluster.x-k8s.io/skip-preflight-checks` annotation on the MachineSet.  

Examples: 
* To opt out of all the preflight checks set the `machineset.cluster.x-k8s.io/skip-preflight-checks: All` annotation.
* To opt out of the `ControlPlaneIsStable` preflight check set the `machineset.cluster.x-k8s.io/skip-preflight-checks: ControlPlaneIsStable` annotation.
* To opt out of multiple preflight checks set the `machineset.cluster.x-k8s.io/skip-preflight-checks: ControlPlaneIsStable,KubernetesVersionSkew` annotation.

<aside class="note">

<h1>Pro-tip: Set annotation through MachineDeployment</h1>

Because of the [metadata propagation](../../reference/api/metadata-propagation.md#machinedeployment) rules in Cluster API you can set the `machineset.cluster.x-k8s.io/skip-preflight-checks` annotation 
on a MachineDeployment and it will be automatically set on the MachineSets of that MachineDeployment, including any new MachineSets created when the MachineDeployment performs a rollout.

</aside>
