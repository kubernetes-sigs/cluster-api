## Using Custom Certificates

Cluster API expects certificates and keys used for bootstrapping to follow the below convention. CABPK generates new certificates using this convention if they do not already exist.

Each certificate must be stored in a single secret named one of:

| Name                   | Type     | Example                                               |
| ---------------------- | -------- | ------------------------------------------------------------ |
| *[cluster name]***-ca**  | CA       | openssl req -x509 -subj "/CN=Kubernetes API" -new -newkey rsa:2048 -nodes -keyout tls.key -sha256 -days 3650 -out tls.crt |
| *[cluster name]***-etcd** | CA       | openssl req -x509 -subj "/CN=ETCD CA" -new -newkey rsa:2048 -nodes -keyout tls.key -sha256 -days 3650 -out tls.crt                                                          |
| *[cluster name]***-proxy** | CA       | openssl req -x509 -subj "/CN=Front-End Proxy" -new -newkey rsa:2048 -nodes -keyout tls.key -sha256 -days 3650 -out tls.crt                                                           |
| *[cluster name]***-sa**  | Key Pair | openssl genrsa -out tls.key 2048 && openssl rsa -in tls.key -pubout -out tls.crt |


<aside class="note warn">

<h1>CA Key Age</h1>

Note that rotating CA certificates is non-trivial and it is recommended to create a long-lived CA or use a long-lived root/offline CA with a short lived intermediary CA

</aside>

**Example**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cluster1-ca
type: kubernetes.io/tls
data:
  tls.crt: <base 64 encoded PEM>
  tls.key: <base 64 encoded PEM>
```

