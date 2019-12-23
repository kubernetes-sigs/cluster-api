# Support for external etcd

Cluster API Bootstrap Provider Kubeadm  supports using an external etcd cluster for your workload Kubernetes clusters.

## ⚠️ Warnings ⚠️

Before getting started you should be aware of the expectations that come with using an external etcd cluster.

* Cluster API is unable to manage any aspect of the external etcd cluster.
* Depending on how you configure your etcd nodes you may incur additional cloud costs in data transfer.
    * As an example, cross availability zone traffic can cost money on cloud providers. You don't have to deploy etcd
    across availability zones, but if you do please be aware of the costs.

## Getting started

To use this, you will need to create an etcd cluster and generate an apiserver-etcd-client key/pair.
This behaviour can be tested using [`kubeadm`](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/setup-ha-etcd-with-kubeadm/) and [`etcdadm`](https://github.com/kubernetes-sigs/etcdadm).
 
### Setting up etcd with kubeadm 

CA certificates are required to setup etcd cluster.
If you already have a CA then the CA's `key` and `crt` must be copied to `/etc/kubernetes/pki/etcd/ca.crt` and `/etc/kubernetes/pki/etcd/ca.key`. 

If you do not already have a CA then run command `kubeadm init phase certs etcd-ca`. This creates two files

 * `/etc/kubernetes/pki/etcd/ca.crt`
 * `/etc/kubernetes/pki/etcd/ca.key`  

These key/pair are used to sign etcd server, peer certificates and eventually apiserver-etcd client. More information on how to setup external etcd with kubeadm can be found [`here`](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/setup-ha-etcd-with-kubeadm/#setting-up-the-cluster).

You would require files `/etc/kubernetes/pki/apiserver-etcd-client.key`, `/etc/kubernetes/pki/apiserver-etcd-client.crt` and `/etc/kubernetes/pki/etcd/server.crt` to setup etcd cluster. These are put in 2 secrets.

```
# Kubernetes APIServer etcd client certificate
$ kubectl create secret tls $CLUSTER_NAME-apiserver-etcd-client \
  --cert /etc/kubernetes/pki/apiserver-etcd-client.crt --key /etc/kubernetes/pki/apiserver-etcd-client.crt \
  --namespace $CLUSTER_NAMESPACE

# Etcd's CA crt file to validate the generated client certificates
$ kubectl create secret tls $CLUSTER_NAME-etcd --cert /etc/kubernetes/pki/etcd/server.crt \ 
  --namespace $CLUSTER_NAMESPACE
```
**Note:** Above command has key/pair base64 encoded by default. 

**Note:** Alternatively you can base64 encode the files and put them in two secrets. The secrets
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

### Setting up etcd with etcdadm (Alpha)
`etcdadm` creates the CA if one does not exist, uses it to sign it's server and peer certificates, and finally to sign the apiserver etcd client certificate.
CA's `key` and `crt` generated using `etcdadm` are stored in `/etc/etcd/pki/apiserver-etcd-client.crt`, `/etc/etcd/pki/apiserver-etcd-client.key` and `/etc/etcd/pki/server.crt` .


Just like kubeadm, it is required to create 2 [`secrets`](https://kubernetes.io/docs/concepts/configuration/secret/#creating-a-secret-using-kubectl-create-secret) using these server and etcd client key/pair.

## Configuring CABPK
After that the rest is standard Kubeadm. Config your ClusterConfiguration as follows:

```yaml
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha3
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
