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
