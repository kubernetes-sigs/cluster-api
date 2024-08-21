# Control Plane Controller

![](../../../images/control-plane-controller.png)

The Control Plane controller's main responsibilities are:

* Managing a set of machines that represent a Kubernetes control plane.
* Provide information about the state of the control plane to downstream
  consumers.
* Create/manage a secret with the kubeconfig file for accessing the workload cluster.

A reference implementation is managed within the core Cluster API project as the
Kubeadm control plane controller (`KubeadmControlPlane`). In this document,
we refer to an example `ImplementationControlPlane` where not otherwise specified.

## Contracts

### Control Plane Provider

The general expectation of a control plane controller is to instantiate a
Kubernetes control plane consisting of the following services:

#### Required Control Plane Services

* etcd
* Kubernetes API Server
* Kubernetes Controller Manager
* Kubernetes Scheduler

#### Optional Control Plane Services

* Cloud controller manager
* Cluster DNS (e.g. CoreDNS)
* Service proxy (e.g. kube-proxy)

#### Prohibited Services

* CNI - should be left to user to apply once control plane is instantiated.

### Relationship to other Cluster API types

The Cluster controller will set an OwnerReference on the Control Plane. The Control Plane controller should normally take no action during reconciliation until it sees the ownerReference.

A Control Plane controller implementation must either supply a controlPlaneEndpoint (via its own `spec.controlPlaneEndpoint` field),
or rely on `spec.controlPlaneEndpoint` in its parent [Cluster](./cluster.md) object.

If an endpoint is not provided, the implementer should exit reconciliation until it sees `cluster.spec.controlPlaneEndpoint` populated.

A Control Plane controller can optionally provide a `controlPlaneEndpoint`

The Cluster controller bubbles up `status.ready` into `status.controlPlaneReady`  and `status.initialized` into a `controlPlaneInitialized` condition from the Control Plane CR.

### CRD contracts

The CRD name must have the format produced by `sigs.k8s.io/cluster-api/util/contract.CalculateCRDName(Group, Kind)`.
The same applies for the name of the corresponding ControlPlane template CRD.

#### Required `spec` fields for implementations using replicas

* `replicas` - is an integer representing the number of desired
  replicas. In the KubeadmControlPlane, this represents the desired
  number of control plane machines.

* `scale` subresource with the following signature:

``` yaml
scale:
  labelSelectorPath: .status.selector
  specReplicasPath: .spec.replicas
  statusReplicasPath: .status.replicas
status: {}
```

More information about the [scale subresource can be found in the Kubernetes
documentation][scale].

#### Required `spec` fields for implementations using version

* `version` - is a string representing the Kubernetes version to be used
  by the control plane machines. The value must be a valid semantic version;
  also if the value provided by the user does not start with the v prefix, it
  must be added.

#### Required `spec` fields for implementations using Machines

* `machineTemplate` - is a struct containing details of the control plane
  machine template.

* `machineTemplate.metadata` - is a struct containing info about metadata for control plane
  machines.

* `machineTemplate.metadata.labels` - is a map of string keys and values that can be used
  to organize and categorize control plane machines.

* `machineTemplate.metadata.annotations` - is a map of string keys and values containing
  arbitrary metadata to be applied to control plane machines.

* `machineTemplate.infrastructureRef` - is a corev1.ObjectReference to a custom resource
  offered by an infrastructure provider. The namespace in the ObjectReference must
  be in the same namespace of the control plane object.

* `machineTemplate.nodeDrainTimeout` - is a *metav1.Duration defining the total amount of time
  that the controller will spend on draining a control plane node.
  The default value is 0, meaning that the node can be drained without any time limitations.

* `machineTemplate.nodeVolumeDetachTimeout` - is a *metav1.Duration defining how long the controller
  will spend on waiting for all volumes to be detached.
  The default value is 0, meaning that the volume can be detached without any time limitations.

* `machineTemplate.nodeDeletionTimeout` - is a *metav1.Duration defining how long the controller
  will attempt to delete the Node that is hosted by a Machine after the Machine is marked for
  deletion. A duration of 0 will retry deletion indefinitely. It defaults to 10 seconds on the
  Machine.

#### Optional `spec` fields for implementations providing endpoints

The `ImplementationControlPlane` object may provide a `spec.controlPlaneEndpoint` field to inform the Cluster
controller where the endpoint is located.

Implementers might opt to choose the `APIEndpoint` struct exposed by Cluster API types, or the following:

<table>
  <tr>
    <th> Field </th>
    <th> Type </th>
    <th> Description </th>
  </tr>
  <tr>
    <td><code>host</code></td>
    <td>String</td>
    <td>
      The hostname on which the API server is serving.
    </td>
  </tr>
  <tr>
    <td><code>port</code></td>
    <td>Integer</td>
    <td>
      The port on which the API server is serving.
    </td>
  </tr>
</table>

#### Required `status` fields

The `ImplementationControlPlane` object **must** have a `status` object.

The `status` object **must** have the following fields defined:

<table>
  <tr>
    <th> Field </th>
    <th> Type </th>
    <th> Description </th>
    <th> Implementation in Kubeadm Control Plane Controller </th>
  </tr>
  <tr>
    <td><code>initialized</code></td>
    <td>Boolean</td>
    <td>
      a boolean field that is true when the target cluster has
      completed initialization such that at least once, the
      target's control plane has been contactable.
    </td>
    <td>
      Transitions to initialized when the controller detects that kubeadm has uploaded
      a kubeadm-config configmap, which occurs at the end of kubeadm provisioning.
    </td>
  </tr>
  <tr>
    <td><code>ready</code></td>
    <td>Boolean</td>
    <td>
      Ready denotes that the target API Server is ready to receive requests.
    </td>
    <td></td>
  </tr>
</table>

#### Required `status` fields for implementations using replicas

Where the `ImplementationControlPlane` has a concept of replicas, e.g. most
high availability control planes, then the `status` object **must** have the
following fields defined:

<table>
  <tr>
    <th> Field </th>
    <th> Type </th>
    <th> Description </th>
    <th> Implementation in Kubeadm Control Plane Controller </th>
  </tr>
  <tr>
    <td><code>readyReplicas</code></td>
    <td>Integer</td>
    <td>Total number of fully running and ready control plane instances.</td>
    <td>Is equal to the number of fully running and ready control plane machines</td>
  </tr>
  <tr>
    <td><code>replicas</code></td>
    <td>Integer</td>
    <td>Total number of non-terminated control plane instances,
      i.e. the state machine for this instance
      of the control plane is able to transition to ready.</td>
    <td>Is equal to the number of non-terminated control plane machines</td>
  </tr>
  <tr>
    <td><code>selector</code></td>
    <td>String</td>
    <td>`selector` is the label selector in string format to avoid
      introspection by clients, and is used to provide the CRD-based integration
      for the scale subresource and additional integrations for things like
      kubectl describe. The string will be in the same format as the query-param
      syntax. More info about label selectors: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
    </td>
    <td></td>
  </tr>
  <tr>
    <td><code>unavailableReplicas</code></td>
    <td>Integer</td>
    <td>
      Total number of unavailable control plane instances targeted by this control plane, equal
      to the desired number of control plane instances - ready instances.
    </td>
    <td>
      Total number of unavailable machines targeted by this control
      plane. This is the total number of machines that are still required
      for the deployment to have 100% available capacity. They may either
      be machines that are running but not yet ready or machines that still
      have not been created.
    </td>
  </tr>
  <tr>
    <td>
      <code>updatedReplicas</code>
    </td>
    <td>integer</td>
    <td>
      Total number of non-terminated machines targeted by this
      control plane that have the desired template spec.
    </td>
    <td>
      Total number of non-terminated machines targeted by this
      control plane that have the desired template spec.
    </td>
  </tr>
</table>

#### Required `status` fields for implementations using version

* `version` - is a string representing the minimum Kubernetes version for the
  control plane machines in the cluster.
  NOTE: The minimum Kubernetes version, and more specifically the API server
  version, will be used to determine when a control plane is fully upgraded
  (`spec.version == status.version`) and for enforcing [Kubernetes version
  skew policies](https://kubernetes.io/releases/version-skew-policy/) in managed topologies.

#### Optional `status` fields

The `status` object **may** define several fields:

* `failureReason` - is a string that explains why an error has occurred, if possible.
* `failureMessage` - is a string that holds the message contained by the error.
* `externalManagedControlPlane` - is a bool that should be set to true if the Node objects do not
  exist in the cluster. For example, managed control plane providers for AKS, EKS, GKE, etc, should
  set this to `true`. Leaving the field undefined is equivalent to setting the value to `false`.

Note: once any of `failureReason` or `failureMessage` surface on the cluster who is referencing the control plane object,
they cannot be restored anymore (it is considered a terminal error; the only way to recover is to delete and recreate the cluster).

## Example usage

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: KubeadmControlPlane
metadata:
  name: kcp-1
  namespace: default
spec:
  machineTemplate:
    infrastructureRef:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
      kind: DockerMachineTemplate
      name: docker-machine-template-1
      namespace: default
  replicas: 3
  version: v1.21.2
```

[scale]: https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definitions/#scale-subresource

## Kubeconfig management

Control Plane providers are expected to create and maintain a Kubeconfig
secret for operators to gain initial access to the cluster.
The given secret must be labelled with the key-pair `cluster.x-k8s.io/cluster-name=${CLUSTER_NAME}`
to make it stored and retrievable in the cache used by CAPI managers. If a provider uses
client certificates for authentication in these Kubeconfigs, the client
certificate should be kept with a reasonably short expiration period and
periodically regenerated to keep a valid set of credentials available. As an
example, the Kubeadm Control Plane provider uses a year of validity and
refreshes the certificate after 6 months.
