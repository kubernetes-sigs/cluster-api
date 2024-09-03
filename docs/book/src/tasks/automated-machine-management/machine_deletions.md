# Machine deletion process

Machine deletions occur in various cases, for example:
* Control plane (e.g. KCP) or MachineDeployment rollouts
* Scale downs of MachineDeployments / MachineSets
* Machine remediations
* Machine deletions (e.g. `kubectl delete machine`)

This page describes how Cluster API deletes Machines.

Machine deletion can be broken down into the following phases:
1. Machine deletion is triggered (i.e. the `metadata.deletionTimestamp` is set)
2. Machine controller waits until all pre-drain hooks succeeded, if any are registered
    * Pre-drain hooks can be registered by adding annotations with the `pre-drain.delete.hook.machine.cluster.x-k8s.io` prefix to the Machine object
3. Machine controller checks if the Machine should be drained, drain is skipped if:
    * The Machine has the `machine.cluster.x-k8s.io/exclude-node-draining` annotation
    * The `Machine.spec.nodeDrainTimeout` field is set and already expired (unset or `0` means no timeout)
4. If the Machine should be drained, the Machine controller evicts all relevant Pods from the Node (see details in [Node drain](#node-drain))
5. Machine controller checks if we should wait until all volumes are detached, this is skipped if:
    * The Machine has the `machine.cluster.x-k8s.io/exclude-wait-for-node-volume-detach` annotation
    * The `Machine.spec.nodeVolumeDetachTimeout` field is set and already expired (unset or `0` means no timeout)
6. If we should wait for volume detach, the Machine controller waits until `Node.status.volumesAttached` is empty
    * Typically the volumes are getting detached by CSI after the corresponding Pods have been evicted during drain
7. Machine controller waits until all pre-terminate hooks succeeded, if any are registered
    * Pre-terminate hooks can be registered by adding annotations with the `pre-terminate.delete.hook.machine.cluster.x-k8s.io` prefix to the Machine object
8. Machine controller deletes the `InfrastructureMachine` object (e.g. `DockerMachine`) of the Machine and waits until it is gone
9. Machine controller deletes the `BootstrapConfig` object (e.g. `KubeadmConfig`) of the machine and waits until it is gone
10. Machine controller deletes the Node object in the workload cluster
    * Node deletion will be retried until either the Node object is gone or `Machine.spec.nodeDeletionTimeout` is expired (`0` means no timeout, but the field defaults to 10s)
    * Note: Nodes are usually also deleted by [cloud controller managers](https://kubernetes.io/docs/concepts/architecture/cloud-controller/), which is why Cluster API per default only tries to delete Nodes for 10s.

Note: There are cases where Node drain, wait for volume detach and Node deletion is skipped. For these please take a look at the 
implementation of the [`isDeleteNodeAllowed` function](https://github.com/kubernetes-sigs/cluster-api/blob/v1.8.0/internal/controllers/machine/machine_controller.go#L346).

## Node drain

This section describes details of the Node drain process in Cluster API. Cluster API implements Node drain aligned
with `kubectl drain`. One major difference is that the Cluster API controller does not actively wait during `Reconcile` 
until all Pods are drained from the Node. Instead it continuously evicts Pods and requeues after 20s until all relevant
Pods have been drained from the Node or until the `Machine.spec.nodeDrainTimeout` is reached (if configured).

Node drain can be broken down into the following phases:
* Node is cordoned (i.e. the `Node.spec.unschedulable` field is set, which leads to the `node.kubernetes.io/unschedulable:NoSchedule` taint being added to the Node)
  * This prevents that Pods that already have been evicted are rescheduled to the same Node. Please only tolerate this taint 
    if you know what you are doing! Otherwise it can happen that the Machine controller is stuck continuously evicting the same Pods. 
* Machine controller calculates the list of Pods that should be evicted. These are all Pods on the Node, except:
  * Pods belonging to an existing DaemonSet (orphaned DaemonSet Pods have to be evicted as well)
  * Mirror Pods, i.e. Pods with the `kubernetes.io/config.mirror` annotation (usually static Pods managed by kubelet, like `kube-apiserver`)
* If there are no (more) Pods that have to be evicted and all Pods that have been evicted are gone, Node drain is completed
* Otherwise an eviction will be triggered for all Pods that have to be evicted. There are various reasons why an eviction call could fail:
  * The eviction would violate a PodDisruptionBudget, i.e. not enough Pod replicas would be available if the Pod would be evicted
  * The namespace is in terminating, in this case the `kube-controller-manager` is responsible for setting the `.metadata.deletionTimestamp` on the Pod
  * Other errors, e.g. a connection issue when calling the eviction API at the workload cluster
* Please note that when an eviction goes through, this only means that the `.metadata.deletionTimestamp` is set on the Pod, but the 
  Pod also has to be terminated and the Pod object has to go away for the drain to complete.
* These steps are repeated every 20s until all relevant Pods have been drained from the Node

Special cases:
* If the Node doesn't exist anymore, Node drain is entirely skipped
* If the Node is `unreachable` (i.e. the Node `Ready` condition is in status `Unknown`):
  * Pods with `.metadata.deletionTimestamp` more than 1s in the past are ignored
  * Pod evictions will use 1s `GracePeriodSeconds`, i.e. the `terminationGracePeriodSeconds` field from the Pod spec will be ignored.
  * Note: PodDisruptionBudgets are still respected, because both of these changes are only relevant if the call to trigger the Pod eviction goes through.
    But Pod eviction calls are rejected when PodDisruptionBudgets would be violated by the eviction.

### Observability

The drain process can be observed through the `DrainingSucceeded` condition on the Machine and various logs.

**Example condition**

To determine which Pods are blocking the drain and why you can take a look at the `DrainingSucceeded` condition on the Machine, e.g.:
```yaml
status:
  ...
  conditions:
  ...
  - lastTransitionTime: "2024-08-30T13:36:27Z"
    message: |-
      Drain not completed yet:
      * Pods with deletionTimestamp that still exist: cert-manager/cert-manager-756d54fb98-hcb6k
      * Pods with eviction failed:
        * Cannot evict pod as it would violate the pod's disruption budget. The disruption budget nginx needs 10 healthy pods and has 10 currently: test-namespace/nginx-deployment-6886c85ff7-2jtqm, test-namespace/nginx-deployment-6886c85ff7-7ggsd, test-namespace/nginx-deployment-6886c85ff7-f6z4s, ... (7 more)
    reason: Draining
    severity: Info
    status: "False"
    type: DrainingSucceeded
```

**Example logs**

When cordoning the Node:
```text
I0830 12:50:13.961156      17 machine_controller.go:716] "Cordoning Node" ... Node="my-cluster-md-0-wxtcg-mtg57-k9qvz"
```

When starting the drain:
```text
I0830 12:50:13.961156      17 machine_controller.go:716] "Draining Node" ... Node="my-cluster-md-0-wxtcg-mtg57-k9qvz"
```

Immediately before Pods are evicted:
```text
I0830 12:52:58.739093      17 drain.go:172] "Drain not completed yet, there are still Pods on the Node that have to be drained" ... Node="my-cluster-md-0-wxtcg-mtg57-ssfg8" podsToTriggerEviction="test-namespace/nginx-deployment-6886c85ff7-4r297, test-namespace/nginx-deployment-6886c85ff7-5gl2h, test-namespace/nginx-deployment-6886c85ff7-64tf9, test-namespace/nginx-deployment-6886c85ff7-9k5gp, test-namespace/nginx-deployment-6886c85ff7-9mdjw, ... (5 more)" podsWithDeletionTimestamp="kube-system/calico-kube-controllers-7dc5458bc6-rdjj4, kube-system/coredns-7db6d8ff4d-9cbhn"
```

On log level 4 it is possible to observe details of the Pod evictions, e.g.:
```text
I0830 13:29:56.211951      17 drain.go:224] "Evicting Pod" ... Node="my-cluster-2-md-0-wxtcg-mtg57-24lvh" Pod="test-namespace/nginx-deployment-6886c85ff7-77fpw"
I0830 13:29:56.211951      17 drain.go:229] "Pod eviction successfully triggered" ... Node="my-cluster-2-md-0-wxtcg-mtg57-24lvh" Pod="test-namespace/nginx-deployment-6886c85ff7-77fpw"
```

After Pods have been evicted, either the drain is directly completed:
```text
I0830 13:29:56.235398      17 machine_controller.go:727] "Drain completed, remaining Pods on the Node have been evicted" ... Node="my-cluster-2-md-0-wxtcg-mtg57-24lvh"
```

or we are requeuing:
```text
I0830 13:29:56.235398      17 machine_controller.go:736] "Drain not completed yet, requeuing in 20s" ... Node="my-cluster-2-md-0-wxtcg-mtg57-24lvh" podsFailedEviction="test-namespace/nginx-deployment-6886c85ff7-77fpw, test-namespace/nginx-deployment-6886c85ff7-8dq4q, test-namespace/nginx-deployment-6886c85ff7-8gjhf, test-namespace/nginx-deployment-6886c85ff7-jznjw, test-namespace/nginx-deployment-6886c85ff7-l5nj8, ... (5 more)" podsWithDeletionTimestamp="kube-system/calico-kube-controllers-7dc5458bc6-rdjj4, kube-system/coredns-7db6d8ff4d-9cbhn"
```

Eventually the Machine controller should log
```text
I0830 13:29:56.235398      17 machine_controller.go:702] "Drain completed" ... Node="my-cluster-2-md-0-wxtcg-mtg57-24lvh"
```

If this doesn't happen, please take a closer at the logs to determine which Pods still have to be evicted or haven't gone away yet
(i.e. deletionTimestamp is set but the Pod objects still exist).

### Related documentation

For more information, please see:
* [Disruptions: Pod disruption budgets](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/#pod-disruption-budgets)
* [Specifying a Disruption Budget for your Application](https://kubernetes.io/docs/tasks/run-application/configure-pdb/)
* [API-initiated eviction](https://kubernetes.io/docs/concepts/scheduling-eviction/api-eviction/)
