# Cluster API bootstrap provider MicroK8s
## What is the Cluster API bootstrap provider MicroK8s?

Cluster API bootstrap provider MicroK8s (CABPM) is a component responsible for generating a cloud-init script to turn a Machine into a Kubernetes Node. This implementation uses [MicroK8s](https://github.com/canonical/microk8s) for Kubernetes bootstrap.

### Resources

* [CABPM Repository](https://github.com/canonical/cluster-api-bootstrap-provider-microk8s)
* [Official MicroK8s site](https://microk8s.io)

## CABPM configuration options

MicroK8s defines a `MicroK8sControlPlane` definition as well as the `MachineDeployment` to configure the control plane and worker nodes respectively. The `MicroK8sControlPlane` is linked in the cluster definition as shown in the following example:

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
spec:
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1beta1
    kind: MicroK8sControlPlane
    name: capi-aws-control-plane
```

A control plane manifest section includes the Kubernetes version, the replica number as well as the `MicroK8sConfig`:

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: MicroK8sControlPlane
spec:
  controlPlaneConfig:
    initConfiguration:
      addons:
      - dns
      - ingress
  replicas: 3
  version: v1.23.0
  ......
``` 

The worker nodes are configured through the `MachineDeployment` object:

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineDeployment
metadata:
  name: capi-aws-md-0
  namespace: default
spec:
  clusterName: capi-aws
  replicas: 2
  selector:
    matchLabels: null
  template:
    spec:
      clusterName: capi-aws
      version: v1.23.0     
      bootstrap:
        configRef:
          apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
          kind: MicroK8sConfigTemplate
          name: capi-aws-md-0
......
---
apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
kind: MicroK8sConfigTemplate
metadata:
  name: capi-aws-md-0
  namespace: default
spec:
  template:
    spec: {}
```

In both the `MicroK8sControlPlane` and `MicroK8sConfigTemplate` you can set a `MicroK8sConfig` object. In the `MicroK8sControlPlane` case `MicroK8sConfig` is under `MicroK8sConfig.spec.controlPlaneConfig` whereas in `MicroK8sConfigTemplate` it is under `MicroK8sConfigTemplate.spec.template.spec`.

Some of the configuration options available via `MicroK8sConfig` are:

  * `MicroK8sConfig.spec.initConfiguration.joinTokenTTLInSecs`: the time-to-live (TTL) of the token used to join nodes, defaults to 10 years.
  * `MicroK8sConfig.spec.initConfiguration.httpsProxy`: the https proxy to be used, defaults to none.
  * `MicroK8sConfig.spec.initConfiguration.httpProxy`: the http proxy to be used, defaults to none.
  * `MicroK8sConfig.spec.initConfiguration.noProxy`: the no-proxy to be used, defaults to none.
  * `MicroK8sConfig.spec.initConfiguration.addons`: the list of addons to be enabled, defaults to dns.
  * `MicroK8sConfig.spec.clusterConfiguration.portCompatibilityRemap`: option to reuse the security group ports set for kubeadm, defaults to true.

### How does CABPM work?

The main purpose of the MicroK8s bootstrap provider is to translate the users needs to the a number of cloud-init files applicable for each type of cluster nodes. There are three types of cloud-inits:

  - The first node cloud-init. That node will be a control plane node and will be the one where the addons are enabled.
  - The control plane node cloud-init. The control plane nodes need to join a cluster and contribute to its HA.
  - The worker node cloud-init. These nodes join the cluster as workers.

The cloud-init scripts are saved as secrets that then the infrastructure provider uses during the machine creation. For more information on cloud-init options, see [cloud config examples](https://cloudinit.readthedocs.io/en/latest/topics/examples.html).
