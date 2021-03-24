---
title: Add support for managed external etcd clusters in CAPI
authors:
  - "@mrajashree"
creation-date: 2021-03-23
last-updated: 2021-03-23
status: provisional
---

# Add support for managed external etcd clusters in CAPI

## Table of Contents

- [Title](#title)
  - [Table of Contents](#table-of-contents)
  - [Glossary](#glossary)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [POC implementation](#poc-implementation)
    - [Questions/Concerns](#questions/concerns-about-implementation)
    - [Design questions](#design-questions)
    - [Security Model](#security-model)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Alternatives](#alternatives)
  - [Upgrade Strategy](#upgrade-strategy)
  - [Additional Details](#additional-details)
    - [Test Plan [optional]](#test-plan-optional)
    - [Graduation Criteria [optional]](#graduation-criteria-optional)
    - [Version Skew Strategy [optional]](#version-skew-strategy-optional)
  - [Implementation History](#implementation-history)

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

If this proposal adds new terms, or defines some, make the changes to the book's glossary when in PR stage.

## Summary

This is a proposal for adding support for provisioning external etcd clusters in Cluster API. CAPI's KubeadmControlPlane supports using an external etcd cluster. However, it currently does not support provisioning and managing this external etcd cluster. 
As per [this KEP](https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/etcdadm/2496-etcdadm), there are ongoing
efforts to rebase [etcd-manager](https://github.com/kopeio/etcd-manager) (used in kOps) on to etcdadm, so as to make etcdadm a consistent etcd solution to be used across all projects, including cluster-api.
This can be achieved for CAPI by adding a new pluggable provider that has two main components:
- A bootstrap provider, that uses [etcdadm](https://github.com/kubernetes-sigs/etcdadm) to convert a machine into an etcd member.
- A new controller that manages this etcd cluster's lifecycle

## Motivation

- Motivation behind having cluster-api provision and manage the external etcd cluster

  - Cluster API supports the use of an external etcd cluster, by allowing users to provide their external etcd cluster's endpoints.
So it would be good to add support for provisioning the etcd cluster too.
    
  - External etcd topology decouples the control plane and etcd member. So if a control plane-only node fails, or if there's a memory leak in a component like kube-apiserver, it won't directly impact an etcd member.
    
  - Etcd is resource intensive, so it is safer to have dedicated nodes for etcd, it could use more disk space, higher bandwidth. 
    Having a separate etcd cluster for these reasons could ensure a more resilient HA setup.
    
- Motivation behind using etcdadm as a pluggable etcd provider
  
  - Leveraging existing projects such as etcdadm is one way of bringing up an etcd cluster. This [KEP](https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/etcdadm/2496-etcdadm) and [etcdadm roadmap](https://github.com/kubernetes-sigs/etcdadm/blob/master/ROADMAP.md) doc indicate that this is one of the goals for etcdadm, to be integrated into cluster-api
    
  - Once the rebase of etcd-manager on etcdadm is completed, etcdadm can also provide cluster administration features that etcd-manager does, such as backups and restores.
  
  - Etcd providers can be made pluggable and we can start with adding one that uses etcdadm.

### Goals

- Introduce pluggable etcd providers in Cluster API, starting with an etcdadm based provider. This etcdadm provider will create and manage an etcd cluster.
- User should be able to create a Kubernetes cluster that uses external etcd topology in a single step.
- User should only have to create the CAPI Cluster resources, along with the required etcd provider specific resources. That should trigger creation of an etcd cluster, followed by creation of the target workload cluster that uses this etcd cluster.
- There will be a 1:1 mapping between the external etcd cluster and the workload cluster.
- Define a contract for pluggable etcd providers and define steps that control plane providers should take to use a managed external etcd cluster.
- Support following etcd cluster management actions: scale up and scale down, etcd member replacement, etcd version upgrades and rollbacks.
- The etcd providers will utilize the existing Machine objects to represent etcd members for convenience instead of adding a new machine type for etcd.
- Etcd cluster members will undergo upgrades using the rollingUpdate strategy.

### Non-Goals
- The first iteration will use IP addresses/hostnames as etcd cluster endpoints. It can not configure static endpoint(s) till the Load Balancer provider is available.
- API changes such as adding new fields within existing control plane providers, like the KubeadmControlPlane provider. We will utilize the existing external etcd endpoints field from the KubeadmConfigSpec for KCP.

## Proposal

### User Stories

- As an end user, I want to be able to create a Kubernetes cluster that uses external etcd topology using CAPI with a single step.
- On creating the required CRs in CAPI along with the new etcd provider specific CRs, CAPI should provision an etcd cluster, followed by a workload cluster that uses this external etcd cluster. 
- As an end user, I should be able to use the etcd provider CRs to specify etcd cluster size during creation and specify/modify etcd version. CAPI controllers should modify and manage the etcd clusters accordingly.

### Implementation Details/Notes/Constraints

These are some of the key differences between etcd cluster provisioning flow when using etcdadm CLI vs the new etcdadm based provider:
- Etcdadm cluster provisioning using CLI commands works as follows:

  - Run `etcdadm init` on any one of the nodes to create a single node etcd cluster. This generates the CA cert & key on that node, along with the server, peer and client certs for that node.
  - The init command also gives as output `etcdadm join` command with the client URL of the first node.
  - To add a new member to the cluster, copy the CA cert key pair from the first node to the right location (`etc/etcd/pki`) on the new node. Then run the `etcdadm join <client URL>` command.
  
- This flow can't be used within cluster-api as is, since it requires copying over certs from the first etcd node to others. Instead it will follow the design that Kubeadm uses by generating certs outside the etcd cluster.
The etcd controller running on the management cluster will generate the certs, and save them as a Secret, which then each member can lookup and populate in the `write_files` section of its cloud-init script
  
- The first node outputs the `etcdadm join <client URL>` command, but cluster-api can't directly use this output. The only way to get this command from the output would be to save the cloud-init output and parse it. Instead the etcd controller can form this command once the first etcd node is provisioned by using that node's address with port 2379.


#### Data Model Changes

The following new type/CRDs will be added for the etcdadm based provider
```go
// API Group: etcdcluster.cluster.x-k8s.io OR etcdplane.cluster.x-k8s.io
type EtcdCluster struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   EtcdClusterSpec   `json:"spec,omitempty"`
    Status EtcdClusterStatus `json:"status,omitempty"`
}

// EtcdClusterSpec defines the desired state of EtcdCluster
type EtcdClusterSpec struct {
    // Replicas is the number of etcd members in the cluster. Defaults to 3 and immutable as really user shouldn't scale up/scale down without reason, since it will affect etcd quorum.
    Replicas *int32 `json:"replicas,omitempty"`

    // InfrastructureTemplate is a required reference to a custom resource
    // offered by an infrastructure provider.
    InfrastructureTemplate corev1.ObjectReference `json:"infrastructureTemplate"`

    // +optional
    EtcdadmConfigSpec etcdbp.EtcdadmConfigSpec `json:"etcdadmConfigSpec"`

    // The RolloutStrategy to use to replace etcd machines with
    // new ones.
    // +optional
    RolloutStrategy *RolloutStrategy `json:"rolloutStrategy,omitempty"`
}

type EtcdClusterStatus struct {
    // Total number of non-terminated machines targeted by this etcd cluster
    // (their labels match the selector).
    // +optional
    ReadyReplicas int32 `json:"replicas,omitempty"`

    // +optional
    InitMachineAddress string `json:"initMachineAddress"`

    // +optional
    Initialized bool `json:"initialized"`

    // +optional
    Ready bool `json:"ready"`

    // +optional
    Endpoint string `json:"endpoint"`

    // +optional
    Selector string `json:"selector,omitempty"`
}

// API Group: bootstrap.cluster.x-k8s.io
type EtcdadmConfig struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    
    Spec   EtcdadmConfigSpec   `json:"spec,omitempty"`
    Status EtcdadmConfigStatus `json:"status,omitempty"`
}

type EtcdadmConfigSpec struct {
    // Users specifies extra users to add
    // +optional
    Users []capbk.User `json:"users,omitempty"`
    // PreEtcdadmCommands specifies extra commands to run before kubeadm runs
    // +optional
    PreEtcdadmCommands []string `json:"preEtcdadmCommands,omitempty"`

    // PostEtcdadmCommands specifies extra commands to run after kubeadm runs
    // +optional
    PostEtcdadmCommands []string `json:"postEtcdadmCommands,omitempty"`

    // +optional
    Version string `json:"version,omitempty"`

    // +optional
    EtcdadmArgs map[string]interface{} `json:"etcdadmArgs,omitempty"`
}


type EtcdadmConfigStatus struct {
    Conditions clusterv1.Conditions `json:"conditions,omitempty"`
    
    DataSecretName *string `json:"dataSecretName,omitempty"`

    Ready bool `json:"ready,omitempty"`
}
```

The following fields will be added/modified in existing CAPI types:
- Add Cluster.Spec.ManagedExternalEtcdRef as:
```go
// ManagedExternalEtcdRef is an optional reference to an etcd provider resource that holds details
// for provisioning an external etcd cluster
// +optional
ManagedExternalEtcdRef *corev1.ObjectReference `json:"managedExternalEtcdRef,omitempty"`
```
- Add Cluster.Status.ManagedExternalEtcdReady field as:
```go
// ManagedExternalEtcdReady indicates external etcd cluster is fully provisioned
// +optional
ManagedExternalEtcdReady bool `json:"managedExternalEtcdReady"`
```
- Make KubeadmConfigSpec.ClusterConfiguration.Etcd.External.Endpoints field mutable
    - The kubeadm validating webhook currently doesn't allow this field to get updated. It should be added as an [allowed path](https://github.com/mrajashree/cluster-api/commit/18c42c47d024ce17cdda39500fc0d6bd67c5aa67#diff-6603d336435f3ee62a50043413f62441b685d5c416e5cadd792ed2a536b2f55fR121) in the webhook.


#### Implementation details

**Contract for pluggable etcd providers**

##### An etcd controller’s main responsibilities are:

* Managing a set of Machines that represent an etcd member
* Managing the etcd cluster, in terms of scaling up/down/upgrading etcd version, and running healthchecks
* Populating and updating a field that will contain the etcd cluster endpoints (IP addresses of all etcd members). 
* Creating/managing two Kubernetes Secrets that will contain certs required by the API server to communicate with the etcd cluster
    * One Secret containing the etcd CA cert, by the name `{cluster.Name}-etcd`
    * One Secret containing the API server cert-key pair, by the name `{cluster.Name}-apiserver-etcd-client`

##### Required services: 
The etcd provider should install etcd with the version specified by the user for each member

##### Required fields: 
Each etcd provider should add a new API type to manage the etcd cluster lifecycle, and it should have the following required fields:
- Spec:
    - Replicas
- Status
    - Ready: To be set to true once all etcd members pass healthcheck (making calls to /health endpoint).
    - Endpoint: A comma separated string containing all etcd members endpoints.

##### Contract between etcd provider and control plane provider
- Control plane providers should check for the presence of the paused annotation (`cluster.x-k8s.io/paused`), and not continue provisioning if it is set. The KubeadmControlPlane controller does that.
- Control plane providers should for presence of `Cluster.Spec.ManagedExternalEtcdRef` field. This check should happen after the control plane is no longer paused, and before it the control plane controller starts provisioning.
- The control plane provider should "Get" the CR referred by the cluster.spec.ManagedExternalEtcdRef and check its status.endpoint field. It will parse the endpoints from this field and use these endpoints where applicable. For instance, the KCP controller will read the status.endpoint field and parse it into a string slice, and then use it as the external etcd endpoints.
- The control plane provider will read the etcd CA cert, and the client cert-key pair from the two Kubernetes Secrets named `{cluster.Name}-apiserver-etcd-client` and `{cluster.Name}-etcd`

##### Contract between etcd provider and infrastructure provider
- Each etcd member requires ports 2379 and 2380 for client and peer communication respectively. 
- In case of the infrastructure providers that use security groups, either the infrastructure provider will add a new security group specifically for etcd members, or document that user needs to create a security group allowing these two ports and use that for the etcd machines.

![etcd-controller-contract](images/managed-etcd/etcd-controller-contract.png)
---


**Etcdadm based etcd provider**

This etcd provider will have two main components:

##### Etcdadm bootstrap provider

- The Etcdadm bootstrap provider will convert a Machine to an etcd member by generating cloud-init scripts that will run the required etcdadm commands. It will do so through the EtcdadmConfig resource.
- Etcdadm bootstrap provider controller will follow the same flow as that of the kubeadm bootstrap provider
    - This controller will also use the [InitLock](https://github.com/kubernetes-sigs/cluster-api/blob/master/bootstrap/kubeadm/controllers/kubeadmconfig_controller.go#L62) way of determining which member runs the init command in case multiple EtcdadmConfig resources are created at the same time. Along with this, the controller will also check the ManagedEtcdInitialized condition on the ClusterStatus. It will be set by the machine controller after the first node is provisioned. If it is not set, the Machine corresponding to the EtcdadmConfig resource being processed will run `etcdadm init` command. If the ManagedEtcdInitialized condition is true, then the Machine will run `etcdadm join` command. 
     - For the init node, the controller will lookup existing CA cert-key pair, or generate a CA cert key and save them in a Secret in the management cluster. For subsequent etcd members, the controller will lookup the CA cert key pair.
     - The controller will generate 
  data for each etcd member, save it as a Secret and save this Secret's name on the EtcdadmConfig.Status.DataSecretName field. This cloud-init data will contain:
          - The CA certs to be written at `etc/etcd/pki`
          - Userdata provided through the EtcdadmConfig spec
          - Etcdadm commands
- The infrastructure provider controller will use this Secret the same way it would a Kubeadm bootstrap Secret, to execute the cloud-init script.
  
##### Etcd cluster controller

- The Etcd cluster lifecycle controller will manage the external etcd through a new API type called EtcdCluster(or EtcdadmCluster). This CRD will accept etcd cluster spec, which includes replicas, etcd configuration options such as version, and an infrastructure template reference. This controller will create EtcdadmConfig CRs with the user specified spec to match the number of replicas. 
- This controller is responsible for provisioning the etcd cluster, and signaling the control plane cluster once etcd is ready.
- [These are the changes required in CAPI](https://github.com/mrajashree/cluster-api/commit/18c42c47d024ce17cdda39500fc0d6bd67c5aa67#diff-626ff994de7814a6e127010bda83fd45dddd14893839ceb4d3d40210a49132f2) for the end to end flow to work

###### Create use case

- After an EtcdCluster object is created, it must bootstrap an etcd cluster with a given number of replicas.
- EtcdCluster.Spec.Replicas must be an odd number. Ideally between 3 and 7.
- Creating an EtcdCluster with > 1 replicas is equivalent to creating an EtcdCluster with 1 replica followed by scaling the EtcdCluster to the desired number of replicas.
- This is the end-to-end flow of creating a cluster using managed external etcd upon applying the following manifest (left out the DockerMachineTemplate resources since nothing will change there):

```yaml
apiVersion: cluster.x-k8s.io/v1alpha4
kind: Cluster
metadata:
  name: "my-cluster"
  namespace: "default"
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.128.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
    serviceDomain: "cluster.local"
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
    kind: DockerCluster
    name: "my-cluster"
    namespace: "default"
  controlPlaneRef:
    kind: KubeadmControlPlane
    apiVersion: controlplane.cluster.x-k8s.io/v1alpha4
    name: "my-cluster-control-plane"
    namespace: "default"
  managedExternalEtcdRef:
    kind: EtcdCluster
    apiVersion: etcdcluster.cluster.x-k8s.io/v1alpha4
    name: "my-cluster-etcd-cluster"
    namespace: "default"
---
kind: EtcdCluster
apiVersion: etcdcluster.cluster.x-k8s.io/v1alpha4
metadata:
  name: "my-cluster-etcd-cluster"
  namespace: default
spec:
  etcdadmConfigSpec:
    version: 3.4.13 #optional
    # etcdadmArgs:
    # users:
  replicas: 3
  infrastructureTemplate:
    kind: DockerMachineTemplate
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
    name: "etcd-plane"
    namespace: "default"
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: DockerCluster
metadata:
  name: "my-cluster"
  namespace: "default"
---
kind: KubeadmControlPlane
apiVersion: controlplane.cluster.x-k8s.io/v1alpha4
metadata:
  name: "abcd-control-plane"
  namespace: "default"
spec:
  replicas: 3
  infrastructureTemplate:
    kind: DockerMachineTemplate
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
    name: "abcd-control-plane"
    namespace: "default"
  kubeadmConfigSpec:
    clusterConfiguration:
      controllerManager:
        extraArgs: {enable-hostpath-provisioner: 'true'}
      apiServer:
        certSANs: [localhost, 127.0.0.1]
      etcd:
        external:
          endpoints: []
          caFile: "/etc/kubernetes/pki/etcd/ca.crt"
          certFile: "/etc/kubernetes/pki/apiserver-etcd-client.crt"
          keyFile: "/etc/kubernetes/pki/apiserver-etcd-client.key"
    initConfiguration:
      nodeRegistration:
        criSocket: /var/run/containerd/containerd.sock
        kubeletExtraArgs: {eviction-hard: 'nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%'}
    joinConfiguration:
      nodeRegistration:
        criSocket: /var/run/containerd/containerd.sock
        kubeletExtraArgs: {eviction-hard: 'nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%'}
  version: "v1.20.2"
```

![etcdcluster-sequence](images/managed-etcd/etcdcluster-sequence.png)

- Cluster.Spec will have a new field `ManagedExternalEtcdRef` of type `ObjectReference` (same as `ControlPlaneRef`). For using CAPI managed etcd cluster, user will set this field in Cluster manifest as shown above.
- Cluster controller's `reconcileControlPlane` will check if `managedExternalEtcdRef` is set on cluster spec and if External etcd cluster is not `Ready`, it will pause the control plane provisioning by setting clusterv1.Paused annotation (`cluster.x-k8s.io/paused`) on it. This will allow etcd cluster provisioning to occur first.
- Etcd cluster controller will generate a CA cert-key pair to be used for all etcd members. This external etcd CA will be saved in a Secret with name `{cluster.Name}-managed-etcd`. We can use the existing util/secret pkg in CAPI for this and add a new [Purpose](https://github.com/mrajashree/cluster-api/blob/etcdadm_bootstrap/util/secret/consts.go#L50) for managed etcd.
- The external etcd controller will also save the etcd CA cert, and the apiserver client cert-key pair in Secrets named `{cluster.Name}-etcd` and `{cluster.Name}-apiserver-etcd-client` respectively.
- Then the controller will first initialize etcd cluster by creating one EtcdadmConfig and a corresponding Machine resource. 
- The CAPI [machine controller](https://github.com/mrajashree/cluster-api/blob/etcdadm_bootstrap/controllers/machine_controller_phases.go#L322-L378) gets the machine's IP address/hostname from the InfraMachine, and as soon as the first etcd machine's IP is obtained, it will create a Secret to store this address, and set the Initialized condition on the EtcdCluster object. This step is required because etcdadm join command must have the first node's client URL.
- Once the EtcdCluster is Initialized, the etcd cluster controller will scale up the cluster to add as many members as required to match EtcdCluster.Spec.Replicas field
- At the end of every reconciliation loop, the etcd cluster controller will check if desired number of members are created by
    * Getting Machines owned by EtcdCluster
    * Running healthcheck for each Machine with the etcd `/health` endpoint at `https://client-url:2379/health`
- Once all members are ready, EtcdCluster will set the endpoints on EtcdCluster.Status.Endpoints field, and set EtcdCluster.Status.Ready to true
- The CAPI cluster controller will have a new `reconcileEtcdCluster` function, that will check if EtcdCluster is ready, and when it is ready it will resume the control plane by deleting the "paused" annotation.
- The control plane controller starts provisioning once it is no longer paused. It will first check if the Cluster spec contains the managedExternalEtcd ref. If the reference exists, it will get the endpoint from the referred etcdCluster object's status field. It will then start the provisioning of the control plane cluster using this etcd cluster.

###### Upgrade or member replacement use case

- The EtcdCluster.Spec.Replicas field will be immutable. This is to protect the etcd cluster from losing quorum by unnecessarily adding/removing healthy members.
- An etcd member will need to be replaced by the etcd controller in two cases:
  - Member is found unhealthy
  - During an upgrade
- While replacing an unhealthy etcd member, it is important to first remove the member and only then replace it by adding a new one. This ensures the cluster stays stable. [This Etcd doc](https://etcd.io/docs/v3.4/faq/#should-i-add-a-member-before-removing-an-unhealthy-member) explains why removing an unhealthy member is important in depth. Etcd's latest versions allow adding new members as learners so it doesn't affect the quorum size, but that feature is in beta. Also, adding a learner is a two-step process, where you first add a member as learner and then promote it, and etcdadm currently doesn't support it. So we can include this in a later version once the learner feature becomes GA and etcdadm incorporates the required changes.
- We can reuse this same "scale down" function when removing a healthy member during an etcd upgrade. So even upgrade will follow the same pattern of removing the member first and then adding a new one with the latest spec.
- Once the upgrade/machine replacement is completed, the etcd controller will again update the EtcdCluster.Status.Endpoint field. The control plane controller will rollout new Machines that use the updated etcd endpoints for the apiserver.

###### Periodic etcd member healthchecks
- The controller should do healthchecks of the etcd members in a loop at a certain predetermined interval (10s).
- When a member fails healthcheck for a certain amount of time (1m), replace that member with a new member

**Changes needed in docker machine controller**
- Each machine infrastructure provider sets a providerID on every InfraMachine.
- Infrastructure providers like CAPA and CAPV set this provider ID based on the instance metadata.
- CAPD on the other hand first calls `kubectl patch` to add the provider ID on the node. And only then it sets the provider ID value on the DockerMachine resource. But the problem is that in case of an etcd only cluster, the machine is not registered as a Kubernetes node. Cluster provisioning does not progress until this provider ID is set on the Machine. As a solution, CAPD will check if the bootstrap provider is etcdadm, and skip the kubectl patch process and set the provider ID on DockerMachine directly. These changes are required in the [docker machine controller](https://github.com/mrajashree/cluster-api/pull/2/files#diff-1923dd8291a9406e3c144763f526bd9797a2449a030f5768178b8d06d13c795bR307) for CAPD.

**Future work**

##### Static Etcd endpoints
- If any VMs running an etcd member get replaced, the kube-apiserver will have to be reconfigured to use the new etcd cluster endpoints. Using static endpoints for the external etcd cluster can avoid this. There are two ways of configuring static endpoints, using a load balancer or configuring DNS records.
- Load balancer can add latency because of the hops associated with routing requests, whereas DNS records will directly resolve to the etcd members.
- The DNS records can be configured such that each etcd member gets a separate sub-domain, under the domain associated with the etcd cluster. An example of this when using ec2 would be:
    - User creates a hosted zone called `external.etcd.cluster` and gives that as input to `EtcdCluster.Spec.HostedZoneName`.
    - The EtcdCluster controller creates a separate A record name within that hosted zone for each EtcdadmConfig it creates. The DNS controller will create the route53 A record once the corresponding Machine's IP address is set
- A suggestion from a CAPI meeting is to add a DNS configuration implementation in the [load balancer proposal](https://github.com/kubernetes-sigs/cluster-api/pull/4389)
- In today's CAPI meeting (April 28th) we decided to implement phase 1 with IP addresses in the absence of the load balancer provider.

##### Member replacement by adding a new member as learner
- Etcd allows adding a new member as a [learner](https://etcd.io/docs/v3.3/learning/learner/). This is useful when adding a new etcd member to the cluster, so that it doesn't increase the quorum size, and the learner gets enough time to catch up with the data from the leader.
- This is a two step process, where you first add a member by calling `member add --learner`, and by promoting the learner to a follower once it has caught up calling `member promote`. 
- This feature in etcd is currently beta. And etcdadm does not support this two step member add process in its current version.
- Once etcdadm makes changes to add members as learners, and the learner feature becomes stable, we can look into changing the upgrade logic to add members as learners first.

### POC implementation
- [etcdadm-bootstrap-provider](https://github.com/mrajashree/etcdadm-bootstrap-provider)
- [etcdadm-controller](https://github.com/mrajashree/etcdadm-controller)
- [CAPI changes](https://github.com/mrajashree/cluster-api/commit/18c42c47d024ce17cdda39500fc0d6bd67c5aa67)

### Questions/concerns about implementation 
    
- Should the machines making the external etcd be registered as a  Kubernetes node or not?
    - So far the assumption is that the external etcd cluster will only run etcd and no other kubernetes components, but are there use cases that will need these machines to also register as a kubernetes node?
    - To clarify, if we have use cases where we need certain helper processes to run on these etcd nodes, we can do it with kubelet + static pod manifests.
- Images: Will CAPI make new image for external etcd nodes, or embed etcdadm release in current image and use it for all nodes


### Security Model

* Does this proposal implement security controls or require the need to do so?

    - Kubernetes RBAC
      - The etcdadm bootstrap provider controller needs following permissions:
     
        | Resource | Permissions
        | -------- | -------- 
        | EtcdadmConfig     | `*` 
        | Cluster | get,list,watch
        | Machine | get,list,watch
        | ConfigMap | `*`
        | Secret | `*`
        | Event | `*`


      - The etcdadm controller needs following permissions:
      
        | Resource | Permissions
        | -------- | -------- 
        | EtcdadmCluster     | `*`
        | EtcdadmConfig | `*`
        | Cluster | get,list,watch
        | Machine | `*`
        | ConfigMap | `*`
        | Secret | `*`
        | Event | `*`

      - CAPI controller manager clusterRole needs `get,list,watch` access to CRDs under `etcdcluster.cluster.x-k8s.io` apiGroup.

    - Security Groups
        - Etcd nodes need to have ports 2379 and 2380 open for incoming client and peer requests

* Is any sensitive data being stored in a secret, and only exists for as long as necessary?
  
  - The CA cert key pair for Etcd will be stored in a Secret. There will be another separate Secret containing only the Etcd CA cert and client cert-key pair for the apiserver to use.

### Risks and Mitigations

- What are the risks of this proposal and how do we mitigate? Think broadly.
- How will UX be reviewed and by whom?
- How will security be reviewed and by whom?
- Consider including folks that also work outside the SIG or subproject.

## Alternatives

The `Alternatives` section is used to highlight and record other possible approaches to delivering the value proposed by a proposal.

## Upgrade Strategy

The external etcd cluster's etcd version can be upgraded through the etcdadmConfigSpec.Version field. There are other etcd configuration options that etcdadm allows which can be modified through the etcdadmConfigSpec.EtcdadmArgs field.
The etcdadm provider will use rollingUpdate strategy to upgrade etcd members. 
- The etcd controller will get all owned Machines which differ in spec from the current EtcdadmConfigSpec and add them to a NeedsRollout pool
- The rollingUpdate will only use `maxUnavailable` parameter which will be hard-coded to 1. This shouldn't be configurable else it will affect etcd quorum.
- The controller will first scale down and remove one of the etcd members. So for a 3-node cluster, quorum size is 2, and even on removing the 3rd member the quorum size stays the same.
- Then it will scale up to match replicas value (3 by default). During scale up, the new machines will get created with the updated etcdadmConfigSpec. During scale up, the machine will run `etcadm join` which calls the etcd `add member` API, and then waits for the member to be active by retrieving a test key. This ensures that the new member is successfully added to the cluster before moving on with the rest of the nodes.


## Additional Details

### Test Plan [optional]

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy.
Anything that would count as tricky in the implementation and anything particularly challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage expectations).
Please adhere to the [Kubernetes testing guidelines][testing-guidelines] when drafting this test plan.

[testing-guidelines]: https://git.k8s.io/community/contributors/devel/sig-testing/testing.md

### Graduation Criteria [optional]

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal should keep
this high-level with a focus on what signals will be looked at to determine graduation.

Consider the following in developing the graduation criteria for this enhancement:
- [Maturity levels (`alpha`, `beta`, `stable`)][maturity-levels]
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

### Version Skew Strategy [optional]

If applicable, how will the component handle version skew with other components? What are the guarantees? Make sure
this is in the test plan.

Consider the following in developing a version skew strategy for this enhancement:
- Does this enhancement involve coordinating behavior in the control plane and in the kubelet? How does an n-2 kubelet without this feature available behave when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI or CNI may require updating that component before the kubelet.

## Implementation History

- [x] 03/24/2021: Proposed idea in the CAPI weekly office hours meeting
- [x] 03/31/2021: Compile a hackmd following the CAEP template
- [x] 04/06/2021: First round of feedback from community
- [x] 04/21/2021: Shared proposal after addressing feedback at CAPI weekly office hours
- [x] 04/28/2021: Gave a demo of the POC for etcdadm controller + bootstrap provider end-to-end flow in CAPI weekly office hours
- [x] 05/24/2021: Open proposal PR

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1Ys-DOR5UsgbMEeciuG0HOgDQc8kZsaWIWJeKJ1-UfbY
