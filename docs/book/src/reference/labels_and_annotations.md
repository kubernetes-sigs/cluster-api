**Supported Labels:**


| Label     | Note     |
|:--------|:--------|
| cluster.x-k8s.io/cluster-name| It is set on machines linked to a cluster and external objects(bootstrap and infrastructure providers). |
| topology.cluster.x-k8s.io/owned| It is set on all the object which are managed as part of a ClusterTopology. |
|topology.cluster.x-k8s.io/deployment-name | It is set on the generated MachineDeployment objects to track the name of the MachineDeployment topology it represents. |
| cluster.x-k8s.io/provider| It is set on components in the provider manifest. The label allows one to easily identify all the components belonging to a provider. The clusterctl tool uses this label for implementing provider's lifecycle operations. |
| cluster.x-k8s.io/watch-filter | It can be applied to any Cluster API object. Controllers which allow for selective reconciliation may check this label and proceed with reconciliation of the object only if this label and a configured value is present. |
| cluster.x-k8s.io/interruptible| It is used to mark the nodes that run on interruptible instances. |
|cluster.x-k8s.io/control-plane | It is set on machines or related objects that are part of a control plane. |
| cluster.x-k8s.io/set-name| It is set on machines if they're controlled by MachineSet. |
| cluster.x-k8s.io/deployment-name| It is set on machines if they're controlled by a MachineDeployment. |
| machine-template-hash| It is applied to Machines in a MachineDeployment containing the hash of the template. |
<br>


**Supported Annotations:**

| Annotation     | Note     |
|:--------|:--------|
| clusterctl.cluster.x-k8s.io/skip-crd-name-preflight-check   | Can be placed on provider CRDs, so that clusterctl doesn't emit a warning if the CRD doesn't comply with Cluster APIs naming scheme. Only CRDs that are referenced by core Cluster API CRDs have to comply with the naming scheme.   |
| unsafe.topology.cluster.x-k8s.io/disable-update-class-name-check   | It can be used to disable the webhook check on update that disallows a pre-existing Cluster to be populated with Topology information and Class.  |
| cluster.x-k8s.io/cluster-name   | It is set on nodes identifying the name of the cluster the node belongs to.  |
|cluster.x-k8s.io/cluster-namespace    | It is set on nodes identifying the namespace of the cluster the node belongs to.   |
| cluster.x-k8s.io/machine   | It is set on nodes identifying the machine the node belongs to.   |
|  cluster.x-k8s.io/owner-kind  |  It is set on nodes identifying the owner kind.   |
| cluster.x-k8s.io/owner-name   | It is set on nodes identifying the owner name.   |
| cluster.x-k8s.io/paused   | It can be applied to any Cluster API object to prevent a controller from processing a resource. Controllers working with Cluster API objects must check the existence of this annotation on the reconciled object. |
|   cluster.x-k8s.io/disable-machine-create | It can be used to signal a MachineSet to stop creating new machines. It is utilized in the OnDelete MachineDeploymentStrategy to allow the MachineDeployment controller to scale down older MachineSets when Machines are deleted and add the new replicas to the latest MachineSet.    |
|  cluster.x-k8s.io/delete-machine  | It marks control plane and worker nodes that will be given priority for deletion when KCP or a MachineSet scales down. It is given top priority on all delete policies.    |
|  cluster.x-k8s.io/cloned-from-name  | It is the infrastructure machine annotation that stores the name of the infrastructure template resource that was cloned for the machine. This annotation is set only during cloning a template. Older/adopted machines will not have this annotation.   |
| cluster.x-k8s.io/cloned-from-groupkind   | It is the infrastructure machine annotation that stores the group-kind of the infrastructure template resource that was cloned for the machine. This annotation is set only during cloning a template. Older/adopted machines will not have this annotation.   |
|  cluster.x-k8s.io/skip-remediation  | It is used to mark the machines that should not be considered for remediation by MachineHealthCheck reconciler.   |
|  cluster.x-k8s.io/managed-by  | It can be applied to InfraCluster resources to signify that some external system is managing the cluster infrastructure. Provider InfraCluster controllers will ignore resources with this annotation. An external controller must fulfill the contract of the InfraCluster resource. External infrastructure providers should ensure that the annotation, once set, cannot be removed.  |
|  topology.cluster.x-k8s.io/dry-run  | It is an annotation that gets set on objects by the topology controller only during a server side dry run apply operation. It is used for validating update webhooks for objects which get updated by template rotation (e.g. InfrastructureMachineTemplate). When the annotation is set and the admission request is a dry run, the webhook should deny validation due to immutability. By that the request will succeed (without any changes to the actual object because it is a dry run) and the topology controller will receive the resulting object. |
|  machine.cluster.x-k8s.io/certificates-expiry    | It captures the expiry date of the machine certificates in RFC3339 format. It is used to trigger rollout of control plane machines before certificates expire. It can be set on BootstrapConfig and Machine objects. The value set on Machine object takes precedence. The annotation is only used by control plane machines. |
|  machine.cluster.x-k8s.io/exclude-node-draining  | It explicitly skips node draining if set.  |
|  machine.cluster.x-k8s.io/exclude-wait-for-node-volume-detach  | It explicitly skips the waiting for node volume detaching if set. |
|  pre-drain.delete.hook.machine.cluster.x-k8s.io  | It specifies the prefix we search each annotation for during the pre-drain.delete lifecycle hook to pause reconciliation of deletion. These hooks will prevent removal of draining the associated node until all are removed. |
| pre-terminate.delete.hook.machine.cluster.x-k8s.io   | It specifies the prefix we search each annotation for during the pre-terminate.delete lifecycle hook to pause reconciliation of deletion. These hooks will prevent removal of an instance from an infrastructure provider until all are removed. |
|  machinedeployment.clusters.x-k8s.io/revision  | It is the revision annotation of a machine deployment's machine sets which records its rollout sequence.   |
|  machinedeployment.clusters.x-k8s.io/revision-history  | It maintains the history of all old revisions that a machine set has served for a machine deployment.   |
|  machinedeployment.clusters.x-k8s.io/desired-replicas  | It is the desired replicas for a machine deployment recorded as an annotation in its machine sets. Helps in separating scaling events from the rollout process and for determining if the new machine set for a deployment is really saturated.   |
|  machinedeployment.clusters.x-k8s.io/max-replicas  | It is the maximum replicas a deployment can have at a given point, which is machinedeployment.spec.replicas + maxSurge. Used by the underlying machine sets to estimate their proportions in case the deployment has surge replicas. |
| controlplane.cluster.x-k8s.io/skip-coredns | It explicitly skips reconciling CoreDNS if set. |
|controlplane.cluster.x-k8s.io/skip-kube-proxy | It explicitly skips reconciling kube-proxy if set.|
| controlplane.cluster.x-k8s.io/kubeadm-cluster-configuration| It is a machine annotation that stores the json-marshalled string of KCP ClusterConfiguration. This annotation is used to detect any changes in ClusterConfiguration and trigger machine rollout in KCP.|
