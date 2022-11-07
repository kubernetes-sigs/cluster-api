## Automatically rotating certificates using Kubeadm Control Plane provider

When using Kubeadm Control Plane provider (KCP) it is possible to configure automatic certificate rotations. KCP does this by triggering a rollout when the certificates on the control plane machines are about to expire.

If configured, the certificate rollout feature is available for all new and existing control plane machines.

### Configuring Machine Rollout

To configure a rollout on the KCP machines you need to set `.rolloutBefore.certificatesExpiryDays` (minimum of 7 days).  

Example: 
```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: KubeadmControlPlane
metadata:
  name: example-control-plane
spec:
  rolloutBefore:
    certificatesExpiryDays: 21 # trigger a rollout if certificates expire within 21 days
  kubeadmConfigSpec:
    clusterConfiguration:
      ...
    initConfiguration:
      ...
    joinConfiguration:
      ...
  machineTemplate:
    infrastructureRef:
      ...
  replicas: 1
  version: v1.23.3
``` 

It is strongly recommended to set the `certificatesExpiryDays` to a large enough value so that all the machines will have time to complete rollout well in advance before the certificates expire.

### Triggering Machine Rollout for Certificate Expiry

KCP uses the value in the corresponding Control Plane machine's `Machine.Status.CertificatesExpiryDate` to check if a machine's certificates are going to expire and if it needs to be rolled out.  

`Machine.Status.CertificatesExpiryDate` gets its value from one of the following 2 places:

* `machine.cluster.x-k8s.io/certificates-expiry` annotation value on the Machine object. This annotation is not applied by default and it can be set by users to manually override the certificate expiry information.
* `machine.cluster.x-k8s.io/certificates-expiry` annotation value on the Bootstrap Config object referenced by the machine. This value is automatically set for machines bootstrapped with CABPK that are owned by the KCP resource.

The annotation value is a [RFC3339] format timestamp. The annotation value on the machine object, if provided, will take precedence.  

<aside class="note warning">

<h1>Certificate Expiry Time</h1>

It is assumed that all certificates on a control plane node have roughly the same expiration time (+/- a few minutes). KCP decides when a rotation is needed based on the expiry of the kube-apiserver certificate.

</aside>

<aside class="note warning">

<h1>Manual certificate rotation</h1>

If certificates on control plane nodes are rotated manually (e.g. via `kubeadm certs renew`), please be aware that the rotation is only
complete after all components including the kube-apiserver are using the new certificates. Thus, kube-apiserver, kube-controller-manager, kube-scheduler and etcd have to be restarted after certificate renewal.
To allow KCP to re-discover the expiry date please remove the `machine.cluster.x-k8s.io/certificates-expiry` annotation from the
KubeadmConfig corresponding to the current machine.

</aside>

<!-- links -->
[RFC3339]: https://www.ietf.org/rfc/rfc3339.txt