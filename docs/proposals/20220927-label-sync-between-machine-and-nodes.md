---
title: Label Sync Between Machines and underlying Kubernetes Nodes
authors:
- "@arvinderpal" (original proposal author)
- "@enxebre"     (original proposal author)
- "@fabriziopandini"
reviewers:
- @sbueringer
- @oscar
- @vincepri
creation-date: 2022-02-10
last-updated: 2022-02-26
status: implementable
replaces:
superseded-by:
---

# Label Sync Between Machines and underlying Kubernetes Nodes

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Glossary](#glossary)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
    - [Story 1](#story-1)
    - [Story 2](#story-2)
    - [Story 3](#story-3)
  - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
  - [Label domains & prefixes](#label-domains--prefixes)
    - [Synchronization of CAPI Labels](#synchronization-of-capi-labels)
    - [Delay between Node Create and Label Sync](#delay-between-node-create-and-label-sync)
- [Alternatives](#alternatives)
  - [Use KubeadmConfigTemplate capabilities](#use-kubeadmconfigtemplate-capabilities)
  - [Apply labels using kubectl](#apply-labels-using-kubectl)
  - [Apply label using external label synchronizer tools](#apply-label-using-external-label-synchronizer-tools)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

## Summary

This document discusses how labels placed on a Machine can be kept in sync with the corresponding Kubernetes Node.

## Motivation

Managing labels on Kubernetes nodes has been a long standing [issue](https://github.com/kubernetes-sigs/cluster-api/issues/493) in Cluster API.

The following challenges have been identified through various iterations:

- Define how labels propagate from Machine to Node.
- Define how labels propagate from ClusterClass to KubeadmControlPlane/MachineDeployments/Machine Pools, and ultimately to Machines.
- Define how to prevent that label propagation triggers unnecessary rollouts.

With the "divide and conquer" principle in mind this proposal aims to address the first point only, while the remaining points are going to be addressed in separated, complementary efforts.

### Goals

- Support label sync from Machine to the linked Kubernetes node, limited to `node-role.kubernetes.io/` prefix and the `node-restriction.kubernetes.io` domain.
- Support syncing labels from Machine to the linked Kubernetes node for the Cluster API owned `node.cluster.x-k8s.io` domain.

### Non-Goals

- Support for arbitrary/user-specified label prefixes.

## Proposal

### User Stories

#### Story 1

As a cluster admin/user, I would like a declarative and secure means by which to assign roles to my nodes. For example, when I do `kubectl get nodes`, I want the ROLES column to list my assigned roles for the nodes.

#### Story 2

As a cluster admin/user, for the purpose of workload placement, I would like a declarative and secure means by which I can add/remove labels on groups of nodes. For example, I want to run my outward facing nginx pods on a specific set of nodes (e.g. md-secure-zone) to reduce exposure to the company's publicly trusted server certificates.

#### Story 3

As a cluster admin/user, I want that Cluster API label management on Kubernetes Nodes doesn't conflict with labels directly managed by users or by other controllers.

### Implementation Details/Notes/Constraints

While designing a solution for syncing labels between Machine and underlying Kubernetes Nodes two main concerns have been considered:

- Security, because Node labels can be used to schedule and/or limit workloads to a predetermined set of nodes.
- Impact on other components applying labels on Kubernetes Nodes, like e.g. Kubeadm, CPI etc.

### Label domains & prefixes

The idea of scoping synchronization to a well defined set of labels is a first answer to security/concurrency concerns; labels to be managed by Cluster API have been selected based on following criteria:

- The `node-role.kubernetes.io` label has been used widely in the past to identify the role of a Kubernetes Node (e.g. `node-role.kubernetes.io/worker=''`). For example, `kubectl get node` looks for this specific label when displaying the role to the user.

- The `node-restriction.kubernetes.io/` domain is recommended in the [Kubernetes docs](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#noderestriction) for things such as workload placement; please note that in this case
  we are considering the entire domain, thus also labels like `example.node-restriction.kubernetes.io/fips=true` fall in this category.

- Cluster API owns a specific domain: `node.cluster.x-k8s.io`.

#### Synchronization of CAPI Labels

The synchronization of labels between a Machine object and its linked Node is limited to the domains and prefixes described in the section above.

The synchronization process is going to use [server side apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/) in order to ensure that Cluster API will manage only the subset of labels coming from Machine objects and ignores labels applied to Nodes concurrently by other users/other components.

This requires to define an identity/manager name to be used by CAPI when performing this operation; additionally during implementation we are going to identify and address eventual additional steps required to properly transition existing Nodes to SSA, if required.
From a preliminary investigation, the risk is that the manager that created the object and the new SSA manager will become co-owner of the same labels, and this will prevent deletion of labels.

The synchronization process will be implemented in the Machine controller. Reconciliation is triggered both when Machine object changes, or when the Node changes (the machine controller watches Nodes within the workload cluster).

#### Delay between Node Create and Label Sync

The Node object is first created when kubelet joins a node to the workload cluster (i.e. kubelet is up and running). There may be a delay (potentially several seconds) before the machine controller kicks in to apply the labels on the Node.

Kubernetes supports both equality and inequality requirements in label selection. In an equality based selection, the user wants to place a workload on node(s) matching a specific label (e.g. Node.Labels contains `my.prefix/foo=bar`). The delay in applying the label on the node, may cause a subsequent delay in the placement of the workload, but this is likely acceptable.

In an inequality based selection, the user wants to place a workload on node(s) that do not contain a specific label (e.g. Node.Labels not contain `my.prefix/foo=bar`). The case is potentially problematic because it relies on the absence of a label and this can occur if the pod scheduler runs during the delay interval.

One way to address this is to use kubelet's `--register-with-taints` flag. Newly minted nodes can be tainted via the taint `node.cluster.x-k8s.io/uninitialized:NoSchedule`. Assuming workloads don't have this specific toleration, then nothing should be scheduled. KubeadmConfigTemplate provides the means to set taints on nodes (see  JoinConfiguration.NodeRegistration.Taints).

The process of tainting the nodes, can be carried out by the user and can be documented as follows:

```
If you utilize inequality based selection for workload placement, to prevent unintended scheduling of pods during the initial node startup phase, it is recommend that you specify the following taint in your KubeadmConfigTemplate:
`node.cluster.x-k8s.io/uninitialized:NoSchedule`
```

After the node has come up and the machine controller has applied the labels, the machine controller will also remove this specific taint if it's present.

During the implementation we will consider also automating the insertion of the taint via CABPK in order to simplify UX;
in this case, the new behaviour should be documented in the contract as optional requirement for bootstrap providers.

The `node.cluster.x-k8s.io/uninitialized:NoSchedule` taint should only be applied on the worker nodes. It should not be applied on the control plane nodes as it could prevent other components like CPI from initializing which will block cluster creation.


## Alternatives

### Use KubeadmConfigTemplate capabilities

Kubelet supports self-labeling of nodes via the `--node-labels` flag. CAPI users can specify these labels via `kubeletExtraArgs.node-labels` in the KubeadmConfigTemplate. There are a few shortcomings:
- [Security concerns](https://github.com/kubernetes/enhancements/tree/master/keps/sig-auth/279-limit-node-access) have restricted the allowed set of labels; In particular, kubelet's ability to self label in the `*.kubernetes.io/` and `*.k8s.io/` namespaces is mostly prohibited. Using prefixes outside these namespaces (i.e. unrestricted prefixes) is discouraged.
- Labels are only applicable at creation time. Adding/Removing a label would require a MachineDeployment rollout. This is undesirable especially in baremetal environments where provisioning can be time consuming.

### Apply labels using kubectl

The documented approach for specifying restricted labels in Cluster API as of today is to utilize [kubectl](https://cluster-api.sigs.k8s.io/user/troubleshooting.html#labeling-nodes-with-reserved-labels-such-as-node-rolekubernetesio-fails-with-kubeadm-error-during-bootstrap). This introduces the potential for human error and also goes against the general declarative model adopted by the project.

### Apply label using external label synchronizer tools

Users could also implement their own label synchronizer in their tooling, but this may not be practical for most.

## Implementation History

- [ ] 09/27/2022: First Draft of this document
- [ ] 09/28/2022: First Draft of this document presented in the Cluster API office hours meeting
