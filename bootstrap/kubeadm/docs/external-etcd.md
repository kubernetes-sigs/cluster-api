# Support for external etcd

Cluster API Bootstrap Provider Kubeadm  supports using an external etcd cluster for your workload Kubernetes clusters.

### ⚠️ Warnings ⚠️

Before getting started you should be aware of the expectations that come with using an external etcd cluster.

* Cluster API is unable to manage any aspect of the external etcd cluster.
* Depending on how you configure your etcd nodes you may incur additional cloud costs in data transfer.
    * As an example, cross availability zone traffic can cost money on cloud providers. You don't have to deploy etcd
    across availability zones, but if you do please be aware of the costs.

### Getting started

To use this, you will need to create an etcd cluster and generate an apiserver-etcd-client key/pair.
[`etcdadm`](https://github.com/kubernetes-sigs/etcdadm) is a good way to get started if you'd like to test this
behavior.

Once you create an etcd cluster, you will want to base64 encode the `/etc/etcd/pki/apiserver-etcd-client.crt`,
`/etc/etcd/pki/apiserver-etcd-client.key`, and `/etc/etcd/pki/server.crt` files and put them in two secrets. The secrets
must be formatted as follows and the cert material must be base64 encoded:

```yaml
# Kubernetes APIServer etcd client certificate
kind: Secret
apiVersion: v1
metadata:
  name: $CLUSTER_NAME-apiserver-etcd-client
  namespace: $CLUSTER_NAMESPACE
data:
  tls.crt: |
    LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURCRENDQWV5Z0F3SUJBZ0lJZFlkclZUMzV0
    NW93RFFZSktvWklodmNOQVFFTEJRQXdEekVOTUFzR0ExVUUKQXhNRVpYUmpaREFlRncweE9UQTVN
    ...
  tls.key: |
    LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFb3dJQkFBS0NBUUVBdlFlTzVKOE5j
    VCtDeGRubFR3alpuQ3YwRzByY0tETklhZzlSdFdrZ1p4MEcxVm1yClA4Zy9BRkhXVHdxSTUrNi81
    ...
```

```yaml
# Etcd's CA crt file to validate the generated client certificates
kind: Secret
apiVersion: v1
metadata:
  name: $CLUSTER_NAME-etcd
  namespace: $CLUSTER_NAMESPACE
data:
  tls.crt: |
    LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURBRENDQWVpZ0F3SUJBZ0lJRDNrVVczaDIy
    K013RFFZSktvWklodmNOQVFFTEJRQXdEekVOTUFzR0ExVUUKQXhNRVpYUmpaREFlRncweE9UQTVN
    ...
```

After that the rest is standard Kubeadm. Config your ClusterConfiguration as follows:

```yaml
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha2
kind: KubeadmConfig
metadata:
  name: CLUSTER_NAME-controlplane-0
  namespace: CLUSTER_NAMESPACE
spec:
  ... # initConfiguration goes here
  clusterConfiguration:
    etcd:
      external:
        endpoints:
          - https://10.0.0.230:2379
        caFile: /etc/kubernetes/pki/etcd/ca.crt
        certFile: /etc/kubernetes/pki/apiserver-etcd-client.crt
        keyFile: /etc/kubernetes/pki/apiserver-etcd-client.key
    ... # other clusterConfiguration goes here
```

Create your cluster as normal!
