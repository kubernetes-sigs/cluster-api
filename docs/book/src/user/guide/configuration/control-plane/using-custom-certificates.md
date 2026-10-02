# Using Custom Certificates

Cluster API expects certificates and keys used for bootstrapping to follow the below convention.
If they do not already exist,, the Kubeadm bootstrap provider generates new certificates using this convention.

Each certificate must be stored in a single secret named one of:

| Name                   | Type     | Example                                               |
| ---------------------- | -------- | ------------------------------------------------------------ |
| *[cluster name]***-ca**  | CA       | openssl req -x509 -subj "/CN=Kubernetes API" -new -newkey rsa:2048 -nodes -keyout tls.key -sha256 -days 3650 -out tls.crt |
| *[cluster name]***-etcd** | CA       | openssl req -x509 -subj "/CN=ETCD CA" -new -newkey rsa:2048 -nodes -keyout tls.key -sha256 -days 3650 -out tls.crt                                                          |
| *[cluster name]***-proxy** | CA       | openssl req -x509 -subj "/CN=Front-End Proxy" -new -newkey rsa:2048 -nodes -keyout tls.key -sha256 -days 3650 -out tls.crt                                                           |
| *[cluster name]***-sa**  | Key Pair | openssl genrsa -out tls.key 2048 && openssl rsa -in tls.key -pubout -out tls.crt |

The certificates *must* also be labeled with the key-value pair `cluster.x-k8s.io/cluster-name=[cluster name]` (where `[cluster name]` is the name of the cluster it should be used with).

> [!TIP]
> **CA Key Age**
>
> Note that rotating CA certificates is non-trivial, so it is recommended to create a long-lived CA, or to use a long-lived root/offline CA with a short-lived intermediate CA.

**Example**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cluster1-ca
  labels:
    cluster.x-k8s.io/cluster-name: cluster1
type: kubernetes.io/tls
data:
  tls.crt: <base64 encoded PEM>
  tls.key: <base64 encoded PEM>
```

## Generating a Kubeconfig with Your Own CA

This guide applies when you are using custom certificates for a
Cluster API workload cluster, rather than relying on automatically generated certificates.

1. Extract the CA certificate and key from the `[cluster-name]-ca` secret:

   ```bash
   kubectl get secret [cluster-name]-ca -o jsonpath='{.data.tls\.crt}' | base64 -d > tls.crt
   kubectl get secret [cluster-name]-ca -o jsonpath='{.data.tls\.key}' | base64 -d > tls.key
   ```

2. Create a new Certificate Signing Request (CSR) for the `admin` user with the `system:masters` Kubernetes role, or specify any other group in the `O` field of the subject.

   ```bash
   openssl req -subj "/CN=admin/O=system:masters" -new -newkey rsa:2048 -nodes -keyout admin.key -out admin.csr
   ```

3. Sign the CSR using the *[cluster-name]-ca* key:

   ```bash
   openssl x509 -req -in admin.csr -CA tls.crt -CAkey tls.key -CAcreateserial -out admin.crt -days 365 -sha256
   ```

4. Add the signed client certificate and key to your kubeconfig:

   ```bash
   kubectl config set-credentials cluster-admin --client-certificate=admin.crt --client-key=admin.key --embed-certs=true
   ```
