# Experimental Feature: MachineSetPreflightChecks (alpha)

The `MachineSetPreflightChecks` feature can provide additional safety while creating new Machines for a MachineSet.

**Feature gate name**: `MachineSetPreflightChecks`

**Variable name to enable/disable the feature gate**: `EXP_MACHINE_SET_PREFLIGHT_CHECKS`

The following preflight checks are performed when the feature is enabled:
* ControlPlaneIsStable
* KubeadmVersionSkew
* KubernetesVersionSkew