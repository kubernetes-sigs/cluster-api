---
title: Kubeadm Based Control Plane Management
authors:
  - "@detiber”
  - "@chuckha”
  - "@randomvariable"
  - "@dlipovetsky"
  - "@amy"
reviewers:
  - "@ncdc"
  - "@timothysc"
  - "@vincepri"
  - "@akutz"
  - "@jaypipes"
  - "@pablochacin"
  - "@rsmitty"
  - "@CecileRobertMichon"
  - "@hardikdr"
  - "@sbueringer"
creation-date: 2019-10-17
last-updated: 2020-09-07
status: implementable
---

# Kubeadm Based Control Plane Management

## Table of Contents

- [Control Plane Management](#control-plane-management)
  - [Table of Contents](#table-of-contents)
  - [Glossary](#glossary)
    - [References](#references)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
      - [Additional goals of the default kubeadm machine-based Implementation](#additional-goals-of-the-default-kubeadm-machine-based-implementation)
    - [Non-Goals / Future Work](#non-goals--future-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Identified features from user stories](#identified-features-from-user-stories)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
      - [New API Types](#new-api-types)
      - [Modifications required to existing API Types](#modifications-required-to-existing-api-types)
      - [Behavioral Changes from v1alpha2](#behavioral-changes-from-v1alpha2)
      - [Behaviors](#behaviors)
        - [Create](#create)
        - [Scale Up](#scale-up)
        - [Scale Down](#scale-down)
        - [Delete of the entire KubeadmControlPlane (kubectl delete controlplane my-controlplane)](#delete-of-the-entire-kubeadmcontrolplane-kubectl-delete-controlplane-my-controlplane)
        - [Cluster upgrade (using create-swap-and-delete)](#cluster-upgrade-using-create-swap-and-delete)
          - [Constraints and Assumptions](#constraints-and-assumptions)
        - [Control plane healthcheck](#control-plane-healthcheck)
        - [Adoption of pre-v1alpha3 Control Plane Machines](#adoption-of-pre-v1alpha3-control-plane-machines)
      - [Code organization](#code-organization)
    - [Risks and Mitigations](#risks-and-mitigations)
      - [etcd membership](#etcd-membership)
      - [Upgrade where changes needed to KubeadmConfig are not currently possible](#upgrade-where-changes-needed-to-kubeadmconfig-are-not-currently-possible)
  - [Design Details](#design-details)
    - [Test Plan](#test-plan)
    - [Graduation Criteria](#graduation-criteria)
      - [Alpha -> Beta Graduation](#alpha---beta-graduation)
    - [Upgrade Strategy](#upgrade-strategy)
  - [Alternatives](#alternatives)
  - [Implementation History](#implementation-history)

## Glossary

The lexicon used in this document is described in more detail [here](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/book/src/reference/glossary.md).  Any discrepancies should be rectified in the main Cluster API glossary.

### References

[Kubernetes Control Plane Management Overview](https://docs.google.com/document/d/1nlWKEr9OP3IeZO5W2cMd3A55ZXLOXcnc6dFu0K9_bms/edit#)

## Summary

This proposal outlines a new process for Cluster API to manage control plane machines as a single concept. This includes
upgrading, scaling up, and modifying the underlying image (e.g. AMI) of the control plane machines.

The control plane covered by this document is defined as the Kubernetes API server, scheduler, controller manager, DNS
and proxy services, and the underlying etcd data store.

## Motivation

During 2019 we saw control plane management implementations in each infrastructure provider. Much like
bootstrapping was identified as being reimplemented in every infrastructure provider and then extracted into Cluster API
Bootstrap Provider Kubeadm (CABPK), we believe we can reduce the redundancy of control plane management across providers
and centralize the logic in Cluster API. We also wanted to ensure that any default control plane management that we
for the default implementation would not preclude the use of alternative control plane management solutions. 

### Goals

- To establish new resource types for control plane management
- To support single node and multiple node control plane instances, with the requirement that the infrastructure provider supports some type of a stable endpoint for the API Server (Load Balancer, VIP, etc).
- To enable scaling of the number of control plane nodes
- To enable declarative orchestrated control plane upgrades
- To provide a default machine-based implementation using kubeadm
- To provide a kubeadm-based implementation that is infrastructure provider agnostic
- To enable declarative orchestrated replacement of control plane machines, such as to roll out an OS-level CVE fix.
- To manage a kubeadm-based, "stacked etcd" control plane
- To manage a kubeadm-based, "external etcd" control plane (using a pre-existing, user-managed, etcd clusters).
- To manage control plane deployments across failure domains.
- To support user-initiated remediation:
  E.g. user deletes a Machine. Control Plane Provider reconciles by removing the corresponding etcd member and updating related metadata

### Non-Goals / Future Work

Non-Goals listed in this document are intended to scope bound the current v1alpha3 implementation and are subject to change based on user feedback over time.

- To manage non-machine based topologies, e.g.
  - Pod based control planes.
  - Non-node control planes (i.e. EKS, GKE, AKS).
- To define a mechanism for providing a stable API endpoint for providers that do not currently have one, follow up work for this will be tracked on [this issue](https://github.com/kubernetes-sigs/cluster-api/issues/1687)
- To predefine the exact contract/interoperability mechanism for alternative control plane providers, follow up work for this will be tracked on [this issue](https://github.com/kubernetes-sigs/cluster-api/issues/1727)
- To manage CA certificates outside of what is provided by Kubeadm bootstrapping
- To manage etcd clusters in any topology other than stacked etcd (externally managed etcd clusters can still be leveraged).
- To address disaster recovery constraints, e.g. restoring a control plane from 0 replicas using a filesystem or volume snapshot copy of data persisted in etcd.
- To support rollbacks, as there is no data store rollback guarantee for Kubernetes. Consumers should perform backups of the cluster prior to performing potentially destructive operations.
- To mutate the configuration of live, running clusters (e.g. changing api-server flags), as this is the responsibility of the [component configuration working group](https://github.com/kubernetes/community/tree/master/wg-component-standard).
- To support auto remediation. Using such a mechanism to automatically replace machines may lead to unintended behaviours and we want to have real world feedback on the health indicators chosen and remediation strategies developed prior to attempting automated remediation.
- To provide configuration of external cloud providers (i.e. the [cloud-controller-manager](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/)).This is deferred to kubeadm.
- To provide CNI configuration. This is deferred to external, higher level tooling.
- To provide the upgrade logic to handle changes to infrastructure (networks, firewalls etc…) that may need to be done to support a control plane on a newer version of Kubernetes (e.g. a cloud controller manager requires updated permissions against infrastructure APIs). We expect the work on [add-on components](https://github.com/kubernetes/community/tree/master/sig-cluster-lifecycle#cluster-addons)) to help to resolve some of these issues.
- To provide automation around the horizontal or vertical scaling of control plane components, especially as etcd places hard performance limits beyond 3 nodes (due to latency).
- To support upgrades where the infrastructure does not rely on a Load Balancer for access to the API Server.
- To implement a fully modeled state machine and/or Conditions, a larger effort for Cluster API more broadly is being organized on [this issue](https://github.com/kubernetes-sigs/cluster-api/issues/1658))

## Proposal

### User Stories

1. As a cluster operator, I want my Kubernetes clusters to have multiple control plane machines to meet my SLOs with application developers.
2. As a developer, I want to be able to deploy the smallest possible cluster, e.g. to meet my organization’s cost requirements.
3. As a cluster operator, I want to be able to scale up my control plane to meet the increased demand that workloads are placing on my cluster.
4. As a cluster operator, I want to be able to remove a control plane replica that I have determined is faulty and should be replaced.
5. As a cluster operator, I want my cluster architecture to be always consistent with best practices, in order to have reliable cluster provisioning without having to understand the details of underlying data stores, replication etc…
6. As a cluster operator, I want to know if my cluster’s control plane is healthy in order to understand if I am meeting my SLOs with my end users.
7. As a cluster operator, I want to be able to quickly respond to a Kubernetes CVE by upgrading my clusters in an automated fashion.
8. As a cluster operator, I want to be able to quickly respond to a non-Kubernetes CVE that affects my base image or Kubernetes dependencies by upgrading my clusters in an automated fashion.
9. As a cluster operator, I would like to upgrade to a new minor version of Kubernetes so that my cluster remains supported.
10. As a cluster operator, I want to know that my cluster isn’t working properly after creation. I have ended up with an API server I can access, but kube-proxy isn’t functional or new machines are not registering themselves with the control plane.

#### Identified features from user stories

1. Based on the function of kubeadm, the control plane provider must be able to scale the number of replicas of a control plane from 1 to X, meeting user stories 1 through 4.
2. To address user story 5, the control plane provider must provide validation of the number of replicas in a control plane. Where the stacked etcd topology is used (i.e., in the default implementation), the number of replicas must be an odd number, as per [etcd best practice](https://etcd.io/docs/v3.3.12/faq/#why-an-odd-number-of-cluster-members). When external etcd is used, any number is valid.
3. In service of user story 5, the kubeadm control plane provider must also manage etcd membership via kubeadm as part of scaling down (`kubeadm` takes care of adding the new etcd member when joining).
4. The control plane provider should provide indicators of health to meet user story 6 and 10. This should include at least the state of etcd and information about which replicas are currently healthy or not. For the default implementation, health attributes based on artifacts kubeadm installs on the cluster may also be of interest to cluster operators.
5. The control plane provider must be able to upgrade a control plane’s version of Kubernetes as well as updating the underlying machine image on where applicable (e.g. virtual machine based infrastructure).

### Implementation Details/Notes/Constraints

#### New API Types

See [kubeadm_control_plane_types.go](https://github.com/kubernetes-sigs/cluster-api/blob/master/controlplane/kubeadm/api/v1alpha3/kubeadm_control_plane_types.go)

With the following validations:

- If `KubeadmControlPlane.Spec.KubeadmConfigSpec` does not define external etcd (webhook):
  - `KubeadmControlPlane.Spec.Replicas` is an odd number.
  - Configuration of external etcd is determined by introspecting the provided `KubeadmConfigSpec`.
- `KubeadmControlPlane.Spec.Replicas` is >= 0 or is nil
- `KubeadmControlPlane.Spec.Version` should be a valid semantic version
- `KubeadmControlPlane.Spec.KubeadmConfigSpec` allows mutations required for supporting following use cases:
    - Change of imagesRepository/imageTags (with validation of CoreDNS supported upgrade)
    - Change of node registration options
    - Change of pre/post kubeadm commands 
    - Change of cloud init files

And the following defaulting:

- `KubeadmControlPlane.Spec.Replicas: 1` 

#### Modifications required to existing API Types

- Add `Cluster.Spec.ControlPlaneRef` defined as:

```go
    // ControlPlaneRef is an optional reference to a provider-specific resource that holds
    // the details for provisioning the Control Plane for a Cluster.
    // +optional
    ControlPlaneRef *corev1.ObjectReference `json:"controlPlaneRef,omitempty"`
```

- Add `Cluster.Status.ControlPlaneReady` defined as:

```go
    // ControlPlaneReady defines if the control plane is ready.
    // +optional
    ControlPlaneReady bool `json:"controlPlaneReady,omitempty"`
```

#### Behavioral Changes from v1alpha2

- If Cluster.Spec.ControlPlaneRef is set:
  - [Status.ControlPlaneInitialized](https://github.com/kubernetes-sigs/cluster-api/issues/1243) is set based on the value of Status.Initialized for the referenced resource.
  - Status.ControlPlaneReady is set based on the value of Status.Ready for the referenced resource, this field is intended to eventually replace Status.ControlPlaneInitialized as a field that will be kept up to date instead of set only once.
- Current behavior will be preserved if `Cluster.Spec.ControlPlaneRef` is not set.
- CA certificate secrets that were previously generated by the Kubeadm bootstrapper will now be generated by the KubeadmControlPlane Controller, maintaining backwards compatibility with the previous behavior if the KubeadmControlPlane is not used.
- The kubeconfig secret that was previously created by the Cluster Controller will now be generated by the KubeadmControlPlane Controller, maintaining backwards compatibility with the previous behavior if the KubeadmControlPlane is not used.

#### Behaviors

##### Create

- After a KubeadmControlPlane object is created, it must bootstrap a control plane with a given number of replicas.
- `KubeadmControlPlane.Spec.Replicas` must be an odd number.
- Can create an arbitrary number of control planes if etcd is external to the control plane, which will be determined by introspecting `KubeadmControlPlane.Spec.KubeadmConfigSpec`.
- Creating a KubeadmControlPlane with > 1 replicas is equivalent to creating a KubeadmControlPlane with 1 replica followed by scaling the KubeadmControlPlane to the desired number of replicas
- The kubeadm bootstrapping configuration provided via `KubeadmControlPlane.Spec.KubeadmConfigSpec` should specify the `InitConfiguration`, `ClusterConfiguration`, and `JoinConfiguration` stanzas, and the KubeadmControlPlane controller will be responsible for splitting the config and passing it to the underlying Machines created as appropriate.
  - This is different than current usage of `KubeadmConfig` and `KubeadmConfigTemplate` where it is recommended to specify `InitConfiguration`/`ClusterConfiguration` OR `JoinConfiguration` but not both.
- The underlying query used to find existing Control Plane Machines is based on the following hardcoded label selector:

```yaml
  selector:
    matchLabels:
      cluster.x-k8s.io/cluster-name: my-cluster
      cluster.x-k8s.io/control-plane: ""
```

- Generate CA certificates if they do not exist
- Generate the kubeconfig secret if it does not exist

Given the following `cluster.yaml` file:

```yaml
kind: Cluster
apiVersion: cluster.x-k8s.io/v1alpha3
metadata:
  name: my-cluster
  namespace: default
spec:
  clusterNetwork:
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  controlPlaneRef:
    kind: KubeadmControlPlane
    apiVersion: cluster.x-k8s.io/v1alpha3
    name: my-controlplane
    namespace: default
  infrastructureRef:
    kind: AcmeCluster
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
    name: my-acmecluster
    namespace: default
---
kind: KubeadmControlPlane
apiVersion: cluster.x-k8s.io/v1alpha3
metadata:
  name: my-control-plane
  namespace: default
spec:
  replicas: 1
  version: v1.16.0
  infrastructureTemplate:
    kind: AcmeProviderMachineTemplate
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
    namespace: default
    name: my-acmemachinetemplate
  kubeadmConfigSpec:
    initConfiguration:
      nodeRegistration:
        name: '{{ ds.meta_data.local_hostname }}'
        kubeletExtraArgs:
          cloud-provider: acme
    clusterConfiguration:
      apiServer:
        extraArgs:
          cloud-provider: acme
      controllerManager:
        extraArgs:
          cloud-provider: acme
    joinConfiguration:
      controlPlane: {}
      nodeRegistration:
        name: '{{ ds.meta_data.local_hostname }}'
        kubeletExtraArgs:
          cloud-provider: acme
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: AcmeProviderMachineTemplate
metadata:
  name: my-acmemachinetemplate
  namespace: default
spec:
  osImage:
    id: objectstore-123456abcdef
  instanceType: θ9.medium
  iamInstanceProfile: "control-plane.cluster-api-provider-acme.x-k8s.io"
  sshKeyName: my-ssh-key
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha2
kind: AcmeCluster
metadata:
  name: my-acmecluster
  namespace: default
spec:
  region: antarctica-1
```

![controlplane-init-1](images/controlplane/controlplane-init-1.png)
![controlplane-init-2](images/controlplane/controlplane-init-2.png)
![controlplane-init-3](images/controlplane/controlplane-init-3.png)
![controlplane-init-4](images/controlplane/controlplane-init-4.png)

##### Scale Up

- Allow scale up a control plane with stacked etcd to only odd numbers, as per
  [etcd best practice](https://etcd.io/docs/v3.3.12/faq/#why-an-odd-number-of-cluster-members).
- However, allow a control plane using an external etcd cluster to scale up to other numbers such as 2 or 4.
- Scale up operations must not be done in conjunction with:
  - Adopting machines
  - Upgrading machines
- Scale up operations are blocked based on Etcd and control plane health checks.
  - See [Health checks](#Health checks) below.
- Scale up operations creates the next machine in the failure domain with the fewest number of machines. 

![controlplane-init-6](images/controlplane/controlplane-init-6.png)

##### Scale Down

- Allow scale down a control plane with stacked etcd to only odd numbers, as per
  [etcd best practice](https://etcd.io/docs/v3.3.12/faq/#why-an-odd-number-of-cluster-members).
- However, allow a control plane using an external etcd cluster to scale down to other numbers such as 2 or 4.
- Scale up operations must not be done in conjunction with:
  - Adopting machines
  - Upgrading machines
- Scale up operations are blocked based on Etcd and control plane health checks.
  - See [Health checks](#Health checks) below.
- Scale down operations removes the oldest machine in the failure domain that has the most control-plane machines on it 

![controlplane-init-7](images/controlplane/controlplane-init-7.png)

##### Delete of the entire KubeadmControlPlane (kubectl delete controlplane my-controlplane)

- KubeadmControlPlane deletion should be blocked until all the worker nodes are deleted.
- Completely removing the control plane and issuing a delete on the underlying machines.
- User documentation should focus on deletion of the Cluster resource rather than the KubeadmControlPlane resource.

##### KubeadmControlPlane rollout (using create-swap-and-delete)

- Triggered by: 
    - Changes to Version
    - Changes to the kubeadmConfigSpec
    - Changes to the infrastructureRef
    - The `upgradeAfter` field, which can be set to a specific time in the future
        - Set to `nil` or the zero value of `time.Time` if no upgrades are desired
        - An upgrade will run when that timestamp is passed
        - Good for scheduling upgrades/SLOs
        - Set `upgradeAfter` to now (in RFC3339 form) if an upgrade is required immediately

- Rollout operations rely on scale up and scale down which are be blocked based on Etcd and control plane health checks
  - See [Health checks](#Health checks) below.

- The rollout algorithm is the following:
  - Find Machines that have an outdated spec
  - If there is a machine requiring rollout
    - Scale up control plane creating a machine with the new spec
    - Scale down control plane by removing one of the machine that needs rollout (the oldest out-of date machine in the failure domain that has the most control-plane machines on it)
    
- In order to determine if a Machine need rollouts includes:
    - The infrastructureRef link used by each machine at creation time is stored in annotations at machine level.
    - The kubeadmConfigSpec used by each machine at creation time is stored in annotations at machine level.
        - If the annotation is not present (machine is either old or adopted), we won't roll out on any possible changes made in KCP's ClusterConfiguration given that we don't have enough information to make a decision.
           Users should use KCP.Spec.UpgradeAfter field to force a rollout in this case.
    
- The controller should tolerate the manual removal of a replica during the upgrade process. A replica that fails during the upgrade may block the completion of the upgrade. Removal or other remedial action may be necessary to allow the upgrade to complete.

###### Constraints and Assumptions

* A stable endpoint (provided by DNS or IP) for the API server will be required in order to allow for machines to maintain a connection to control plane machines as they are swapped out during upgrades. This proposal is agnostic to how this is achieved, and is being tracked in https://github.com/kubernetes-sigs/cluster-api/issues/1687. The control plane controller will use the presence of the apiEndpoints status field of the cluster object to  determine whether or not to proceed. This behaviour is currently implicit in the implementations for cloud providers that provider a load balancer construct.

* Infrastructure templates are expected to be immutable, so infrastructure template contents do not have to hashed in order to detect
  changes.

##### Health checks

- Will be used during scaling and upgrade operations.

###### Etcd (external)

Etcd connectivity is the only metric used to assert etcd cluster health.

###### Etcd (stacked)

Etcd is considered healthy if:
- There are an equal number of control plane Machines and members in the etcd cluster.
  - This ensures there are no members that are unaccounted for.
- Each member reports the same list of members.
  - This ensures that there is only one etcd cluster.
- Each member does not have any active alarms.
  - This ensures there is nothing wrong with the individual member.

The KubeadmControlPlane controller uses port-forwarding to get to a specific etcd member.

###### Kubernetes Control Plane

- For stacked control planes, we will present etcd quorum status within the `KubeadmControlPlane.Status.Ready` field, and also report the number of active cluster members through `KubeadmControlPlane.Status.ReadyReplicas`.

- There are an equal number of control plane Machines and api server pods checked.
  - This ensures that Cluster API is tracking all control plane machines.
- Each control plane node has an api server pod that has the Ready condition.
  - This ensures that the API server can contact etcd and is ready to accept requests.
- Each control plane node has a controller manager pod that has the Ready condition.
  - This ensures the control plane can manage default Kubernetes resources.

##### Adoption of pre-v1alpha3 Control Plane Machines

- Existing control plane Machines will need to be updated with labels matching the expected label selector.
- The KubeadmConfigSpec can be re-created from the referenced KubeadmConfigs for the Machines matching the label selector.
  - If there is not an existing initConfiguration/clusterConfiguration only the joinConfiguration will be populated.
- In v1alpha2, the Cluster API Bootstrap Provider is responsible for generating certificates based upon the first machine to join a cluster. The OwnerRef for these certificates are set to that of the initial machine, which causes an issue if that machine is later deleted. For v1alpha3, control plane certificate generation will be replicated in the KubeadmControlPlane provider. Given that for v1alpha2 these certificates are generated with deterministic names, i.e. prefixed with the cluster name, the migration mechanism should replace the owner reference of these certificates during migration. The bootstrap provider will need to be updated to only fallback to the v1alpha2 secret generation behavior if Cluster.Spec.ControlPlaneRef is nil.
- To ease the adoption of v1alpha3, the migration mechanism should be built into Cluster API controllers.

#### Code organization

The types introduced in this proposal will live in the `cluster.x-k8s.io` API group. The controller(s) will also live inside `sigs.k8s.io/cluster-api`.

### Risks and Mitigations

#### etcd membership

- If the leader is selected for deletion during a replacement for upgrade or scale down, the etcd cluster will be unavailable during that period as leader election takes place. Small time periods of unavailability should not significantly impact the running of the managed cluster’s API server.
- Replication of the etcd log, if done for a sufficiently large data store and saturates the network, machines may fail leader election, bringing down the cluster. To mitigate this, the control plane provider will only create machines serially, ensuring cluster health before moving onto operations for the next machine.
- When performing a scaling operation, or an upgrade using create-swap-delete, there are periods when there are an even number of nodes. Any network partitions or host failures that occur at this point will cause the etcd cluster to split brain. Etcd 3.4 is under consideration for Kubernetes 1.17, which brings non-voting cluster members, which can be used to safely add new machines without affecting quorum. [Changes to kubeadm](https://github.com/kubernetes/kubeadm/issues/1793) will be required to support this and is out of scope for the time frame of v1alpha3.

#### Upgrade where changes needed to KubeadmConfig are not currently possible

- We don't anticipate that this will immediately cause issues, but could potentially cause problems when adopt new versions of the Kubeadm configuration that include features such as kustomize templates. These potentially would need to be modified as part of an upgrade.

## Design Details

### Test Plan

Standard unit/integration & e2e behavioral test plans will apply.

### Graduation Criteria

#### Alpha -> Beta Graduation

This work is too early to detail requirements for graduation to beta. At a minimum, etcd membership and quorum risks will need to be addressed prior to beta.

### Upgrade Strategy

- v1alpha2 managed clusters that match certain known criteria should be able to be adopted as part of the upgrade to v1alpha3, other clusters should continue to function as they did previously.

## Alternatives

For the purposes of designing upgrades, two existing lifecycle managers were examined in detail: kops and Cloud Foundry Container Runtime. Their approaches are detailed in the accompanying "[Cluster API Upgrade Experience Reports](https://docs.google.com/document/d/1RnUG9mHrS_qmhmm052bO6Wu0dldCamwPmTA1D0jCWW8/edit#)" document.

## Implementation History

- [x] 10/17/2019: Initial Creation
- [x] 11/19/2019: Initial KubeadmControlPlane types added [#1765](https://github.com/kubernetes-sigs/cluster-api/pull/1765)
- [x] 12/04/2019: Updated References to ErrorMessage/ErrorReason to FailureMessage/FailureReason
- [x] 12/04/2019: Initial stubbed KubeadmControlPlane controller added [#1826](https://github.com/kubernetes-sigs/cluster-api/pull/1826)
- [x] 07/09/2020: Document updated to reflect latest changes