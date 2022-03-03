---

title: Label Sync Between MachineDeployment and underlying Kubernetes Nodes
authors:
  - "@arvinderpal"
  - "@enxebre"
reviewers:
  - "@fabriziopandini"
  - "@neolit123"
  - "@sbueringer"
  - "@vincepri"
  - "@randomvariable"
creation-date: 2022-02-10
last-updated: 2022-02-26
status: implementable
replaces:
superseded-by:

---

# Label Sync Between MachineDeployment and underlying Kubernetes Nodes

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

## Summary

Labels on worker nodes are commonly used for node role assignment and workload placement. This document discusses a means by which labels placed on MachineDeployments can be kept in sync with labels on the underlying Kubernetes Nodes. 

## Motivation

Kubelet supports self-labeling of nodes via the `--node-labels` flag. CAPI users can specify these labels via `kubeletExtraArgs.node-labels` in the KubeadmConfigTemplate. There are a few shortcomings: 
- [Security concerns](https://github.com/kubernetes/enhancements/tree/master/keps/sig-auth/279-limit-node-access) have restricted the allowed set of labels; In particular, kubelet's ability to self label in the `*.kubernetes.io/` and `*.k8s.io/` namespaces is mostly prohibited. Using prefixes outside these namespaces (i.e. unrestricted prefixes) is discouraged.  
- Labels are only applicable at creation time. Adding/Removing a label would require a MachineDeployment rollout. This is undesirable especially in baremetal environments where provisioning can be time consuming. 

The documented approach for specifying restricted labels in CAPI is to utilize [kubectl](https://cluster-api.sigs.k8s.io/user/troubleshooting.html#labeling-nodes-with-reserved-labels-such-as-node-rolekubernetesio-fails-with-kubeadm-error-during-bootstrap). This introduces the potential for human error and also goes against the general declarative model adopted by CAPI.

Users could also implement their own label synchronizer in their tooling, but this may not be practical for most. 

### Goals

- Support label synchronization for MachineDeployments as well as KubeadmControlPlane (KCP).
- Support a one-time application of labels that fall under the following restricted prefixes: `node-role.kubernetes.io/*` and `node-restriction.kubernetes.io/*`
- Support synchronizing labels that fall under a CAPI specific prefix: `node.cluster.x-k8s.io/*`

### Future Goals

- Label sync for MachinePools nodes. 

### Non-Goals

- Support for arbitrary/user-specified label prefixes. 

## Proposal

CAPI will define its own label prefix:

```
node.cluster.x-k8s.io/*
```

Labels placed on the Machine that match the above prefix will be authoritative -- Cluster API will act as the ultimate authority for anything under the prefix. Labels with the above prefix should be set only within Cluster API CRDs.

In addition to the above, CAPI will support one-time application of the following labels during Machine creation:

```
node-role.kubernetes.io/*
node-restriction.kubernetes.io/*
```

The `node-role.kubernetes.io` prefix has been used widely in the past to identify the role of a node (e.g. `node-role.kubernetes.io/worker=''`). For example, `kubectl get node` looks for this specific label when displaying the role to the user.

The use of the `node-restriction.kubernetes.io/*` prefix is recommended in the [Kubernetes docs](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#noderestriction) for things such as workload placement.

It's important to note that the these specific labels are *not* kept in sync, but rather applied only once after the Machine and its corresponding Node are first created. Taking authoritative ownership of these prefixes would preclude other entities from adding and removing such labels on the Node. 

### User Stories

#### Story 1

As a cluster admin/user, I would like a declarative and secure means by which to assign roles to my nodes. For example, when I do `kubectl get nodes`, I want the ROLES column to list my assigned roles for the nodes.

#### Story 2

As a cluster admin/user, for the purpose of workload placement, I would like a declarative and secure means by which I can add/remove labels on groups of nodes. For example, I want to run my outward facing nginx pods on a specific set of nodes (e.g. md-secure-zone) to reduce exposure to the company's publicly trusted server certificates.

### Implementation Details/Notes/Constraints

#### Label propogation from MachineDeployment to Machine

Utilize CAPI metadata Propagation. CAPI supports [metadata propagation](https://cluster-api.sigs.k8s.io/developer/architecture/controllers/metadata-propagation.html?highlight=propagation#metadata-propagation) across core API resources. For the MachineDeployment types, template labels propagate to MachineSets top-level and MachineSets template metadata:
```
MachineDeployment.spec.template.metadata.labels => MachineSet.labels, MachineSet.spec.template.metadata.labels
```
Similarly, on MachineSet, template labels propagate to Machines:
```
MachineSet.spec.template.metadata.labels => Machine.labels
```

As of this writing, changing the `MachineDeployment.spec.template.metadata.labels` will trigger a rollout. This is undesirable. CAPI will be updated to instead ignore updates of labels that fall within the CAPI prefix. There is precedence for this with the handling of `MachineDeploymentUniqueLabel`. 

#### Synchronization of CAPI Labels

Labels that fall under the CAPI specific prefix `node.cluster.k8s.io/*` will need to be kept in sync between the Machine and Node objects. Synchronization must handle these two scenarios: 
(A) Label is added/removed on a Machine and must be added/removed on the corresponding Node.
(B) Label is added/removed on the Node and must be removed/re-added on the Node to bring it in sync with the labels on Machine.


#### Machine Controller Changes

On each Machine reconcile event, the machine controller will fetch the corresponding Node (i.e. Machine.Status.NodeRef). If the Machine has labels specified, then the controller will ensure those labels are applied to the Node. 

Similarly, today the machine controller watches Nodes within the workload cluster, where changes to the Node trigger a reconcile of its corresponding Machine object. This should allow us to capture changes made directly to the labels on the Node (e.g. user accidently deletes a label that falls under CAPI's managed prefix).

#### One-time Apply of Standard Kubernetes Labels

The machine controller will apply the standard kubernetes labels, if specified, to the Node immediately after the Node enters the Ready state (ProviderID is set). We will enforce this one-time application by making the labels immutable via a validating webhook. 

#### Delay between Node Create and Label Sync 

The Node object is first created when kubeadm joins a node to the workload cluster (i.e. kubelet is up and running). There may be a delay (potentially several seconds) before the machine controller kicks in to apply the labels on the Node.

Kubernetes supports both equality and inequality requirements in label selection. In an equality based selection, the user wants to place a workload on node(s) matching a specific label (e.g. Node.Labels contains `my.prefix/foo=bar`). The delay in applying the label on the node, may cause a subsequent delay in the placement of the workload, but this is likely acceptable.

In an inequality based selection, the user wants to place a workload on node(s) that do not contain a specific label (e.g. Node.Labels not contain `my.prefix/foo=bar`). The case is potentially problematic because it relies on the absence of a label and this can occur if the pod scheduler runs during the delay interval.

One way to address this is to use kubelet's `--register-with-taints` flag. Newly minted nodes can be tainted via the taint `cluster.x-k8s.io=label-sync-pending:NoSchedule`. Assuming workloads don't have this specific toleration, then nothing should be scheduled. KubeadmConfigTemplate provides the means to set taints on nodes (see  JoinConfiguration.NodeRegistration.Taints).

The process of tainting the nodes, can be carried out by the user and can be documented as follows: 

```
If you utilize inequality based selection for workload placement, to prevent unintended scheduling of pods during the initial node startup phase, it is recommend that you specify the following taint in your KubeadmConfigTemplate:
`cluster.x-k8s.io=label-sync-pending:NoSchedule`
```

After the node has come up and the machine controller has applied the labels, the machine controller will also remove this specific taint if it's present. 

In the future, we may consider automating the insertion of the taint via CABPK. 

## Alternatives

### Label propogation from MachineDeployment to Machine

Option #1: Introduce a new field Spec.Node.Labels

Instead of using the top-level metadata, a new field would be introduced:

```
MachineDeployment.Spec.Node.Labels => MachineSet.Spec.Node.Labels => Machine.Spec.Node.Labels
```

The benefit being that it provides a clear indication to the user that these labels will be synced to the Kubernetes Node(s). This will, however, require api changes. 

Option #2: Propogate labels in the top-level metadata

We could propogate labels in the top-level metadata. These are however are not currently propagated. One option is to only propogate top-level labels matching the aforementioned prefixes. The propogation would follow a similar path to that described earlier:

```
MachineDeployment.labels => MachineSet.labels => Machine.labels
```

## Implementation History

- [ ] MM/DD/YYYY: Proposed idea in an issue or [community meeting]
