---
title: KCP upgrade scale-in support 
authors:
  - "@jantilles"
reviewers:
  - "@janedoe"
creation-date: 2020-03-16
last-updated: 2020-03-16
status: implementable
see-also:
  - "/docs/proposals/20191017-kubeadm-based-control-plane.md"
---

# Kubeadm Control Plane scale in support

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Title](#title)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
    - [Implementation Details](#implementation-details)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Design Details](#design-details)
    - [Test Plan](#test-plan)
  - [Implementation History](#implementation-history)

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

## Summary

This proposal outlines a new functionality for Kubeadm Control Plane. This includes scale-in during upgrade of KCP and necessary checks to ensure that KCP does not run single replica in case of scale-in is required.

## Motivation

New Kubeadm Control Plane default implementation does not support scale-in during upgrade. Main motivation for the current design is to ensure upgrading of a control plane with a single replica. This design is also limited by the expectation of automated provisioning mechanism with some level of additional resources to first scale-out the KCP.

These are challenging limitation in baremetal use cases. Extra baremetal machines might not be available and it is not good starting point to expect operators to run their resources idle for possible upgrades. This is extremely vital in all baremetal workflow clusters where all resources are running without any idle machines.   

### Goals

- To support KCP scale-in during upgrade through RollingUpdate.
- To add ControlPlaneDeploymentStrategyType.
- To support maxUnavailable or maxSurge
- Keep current design as a default implementation to ensure upgrading of a control plane with a single replica. 

## Proposal

### User Stories

#### Story 1 

As an operator I would like to upgrade my baremetal control plane machines without having additional resources for scale-out first.

#### Story 2

As an operator I want control plane provider to make sure that my control plane is not upgraded if KCP is running a single replica and extra resources are not available.

#### Identified Features

1. Control plane provider must be able to scale-in the number of replicas of a control plane from 3 replicas and above, meeting user story 1.
2. Control plane provider should fall into default implementation when extra resources are available.
3. Upgrade should fail when KCP is running a single replica and extra resources are not available, meeting user story 2.
4. With maxUnavailable and maxSurge set control plane provider should behavour the way it don't break consensus of ETCD.
5. Control plane provider should take care of that amouth of KCP replicas is always n/2+1 during update. 

### Implementation Details

### New API changes

KubeadmControlPlane:

```

type ControlPlaneDeploymentStrategyType string

const (
	RollingUpdateControlPlaneDeploymentStrategyType ControlPlaneDeploymentStrategyType = "RollingUpdate"
)

// ControlPlaneDeploymentStrategy describes how to replace existing control planes
// with new ones.
type ControlPlaneDeploymentStrategy struct {
	// Type of deployment. Currently the only supported strategy is
	// "RollingUpdate".
	// Default is RollingUpdate.
	// +optional
	Type ControlPlaneDeploymentStrategyType `json:"type,omitempty"`

	// Rolling update config params. Present only if
	// MachineDeploymentStrategyType = RollingUpdate.
	// +optional
	RollingUpdate *ControlPlaneRollingUpdateDeployment `json:"rollingUpdate,omitempty"`
}

// ControlPlaneRollingUpdateDeployment is used to control the desired behavior of rolling update.
type ControlPlaneRollingUpdateDeployment struct {
	// The maximum number of machines that can be unavailable during the update.
	// Value can be an absolute number (ex: 5) or a percentage of desired
	// machines (ex: 10%).
	// Absolute number is calculated from percentage by rounding down.
	// This can not be 0 if MaxSurge is 0.
	// Defaults to 0.
	// Example: when this is set to 30%, the old MachineSet can be scaled
	// down to 70% of desired machines immediately when the rolling update
	// starts. Once new machines are ready, old MachineSet can be scaled
	// down further, followed by scaling up the new MachineSet, ensuring
	// that the total number of machines available at all times
	// during the update is at least 70% of desired machines.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// The maximum number of machines that can be scheduled above the
	// desired number of machines.
	// Value can be an absolute number (ex: 5) or a percentage of
	// desired machines (ex: 10%).
	// This can not be 0 if MaxUnavailable is 0.
	// Absolute number is calculated from percentage by rounding up.
	// Defaults to 1.
	// Example: when this is set to 30%, the new MachineSet can be scaled
	// up immediately when the rolling update starts, such that the total
	// number of old and new machines do not exceed 130% of desired
	// machines. Once old machines have been killed, new MachineSet can
	// be scaled up further, ensuring that total number of machines running
	// at any time during the update is at most 130% of desired machines.
	// +optional
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
}

// KubeadmControlPlaneSpec defines the desired state of KubeadmControlPlane.
type KubeadmControlPlaneSpec struct {
	// Number of desired machines. Defaults to 1. When stacked etcd is used only
	// odd numbers are permitted, as per [etcd best practice](https://etcd.io/docs/v3.3.12/faq/#why-an-odd-number-of-cluster-members).
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// The deployment strategy to use to replace existing machines with
	// new ones.
	// +optional
	Strategy *ControlPlaneDeploymentStrategy `json:"strategy,omitempty"`

	// Version defines the desired Kubernetes version.
	// +kubebuilder:validation:MinLength:=1
	Version string `json:"version"`

	// InfrastructureTemplate is a required reference to a custom resource
	// offered by an infrastructure provider.
	InfrastructureTemplate corev1.ObjectReference `json:"infrastructureTemplate"`

	// KubeadmConfigSpec is a KubeadmConfigSpec
	// to use for initializing and joining machines to the control plane.
	KubeadmConfigSpec cabpkv1.KubeadmConfigSpec `json:"kubeadmConfigSpec"`

	// UpgradeAfter is a field to indicate an upgrade should be performed
	// after the specified time even if no changes have been made to the
	// KubeadmControlPlane
	// +optional
	UpgradeAfter *metav1.Time `json:"upgradeAfter,omitempty"`
}
```
With the following changes:

- KCP is enable to use rolling update and setting maxSurge = 0, it should scale in first.

### Controller changes

```
func (r *KubeadmControlPlaneReconciler) upgradeControlPlaneWithScaleIn(ctx context.Context, cluster *clusterv1.Cluster, 
kcp *controlplanev1.KubeadmControlPlane, ownedMachines internal.FilterableMachineCollection, requireUpgrade internal.FilterableMachineCollection) (ctrl.Result, error)

```

By introducing new upgradeControlPlaneWithScaleIn function we can keep default implementation untouched. New function reverse the default **upgradeControlPlane** function logic. It first scale down one replica, after replica is successfully deleted, it will create new replica and continues upgrade KCP replicas one-by-one. 

### Risks and Mitigations

- As this updated proposal does add rolling update strategy and support for maxUnavailable or maxSurge, we need to discuss if design really support for example scale in or out multiple replicas of KCP.
- If we allow scale in and out of multiple replicas at the same time, how does this affect the overal design. 
- If the rollback of KCP is in future plans it needs to be fullfill the baremetal use cases. 

### Test Plan [optional]

Standard unit/integration & e2e behavioral test plans will apply.

## Implementation History

- [ ] 03/09/2020: Proposed idea in an issue [Scale in support for KCP]: https://github.com/kubernetes-sigs/cluster-api/issues/2587
- [ ] 03/16/2020: Proposal PR created 
- [ ] 03/18/2020: Proposal PR updated
