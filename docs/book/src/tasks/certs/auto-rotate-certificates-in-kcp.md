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

<h1> Approximate Certificate Expiry Time </h1>

The time captured in the Bootstrap Config annotation is an approximate time at which the machine certificates will expire (1 year from creation). The time captured in the annotation will be a little earlier than the actual certificate expiry time.

It is assumed that all the certificates has the same expiration time. If not, it is assumed that the kube-apiserver certificate expires before other certificates.

</aside>

<aside class="note warning">

<h1> Deleting the machine.cluster.x-k8s.io/certificates-expiry annotation </h1>

If the annotation is delete from the object, the certificate expiry information will be cleared form the Machine's status leading to certificate renewal being effectively disabled. It is recommended to be highly cautions when deleting this annotation from the object.

</aside>

<!-- links -->
[RFC3339]: https://www.ietf.org/rfc/rfc3339.txt