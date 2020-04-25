# Support for external etcd

Cluster API Bootstrap Provider Kubeadm supports using an external etcd cluster for your workload Kubernetes clusters.

## ⚠️ Warnings ⚠️

Before getting started you should be aware of the expectations that come with using an external etcd cluster.

* Cluster API is unable to manage any aspect of the external etcd cluster.
* Depending on how you configure your etcd nodes you may incur additional cloud costs in data transfer.
    * As an example, cross availability zone traffic can cost money on cloud providers. You don't have to deploy etcd across availability zones, but if you do please be aware of the costs.

## Getting started

To use this, you will need to create an etcd cluster and generate an apiserver-etcd-client certificate and private key. This behaviour can be tested using [`kubeadm`](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/setup-ha-etcd-with-kubeadm/) and [`etcdadm`](https://github.com/kubernetes-sigs/etcdadm).
 
### Setting up etcd with kubeadm 

CA certificates are required to setup etcd cluster. If you already have a CA then the CA's `crt` and `key` must be copied to `/etc/kubernetes/pki/etcd/ca.crt` and `/etc/kubernetes/pki/etcd/ca.key`. 

If you do not already have a CA then run command `kubeadm init phase certs etcd-ca`. This creates two files:

* `/etc/kubernetes/pki/etcd/ca.crt`
* `/etc/kubernetes/pki/etcd/ca.key`  

This certificate and private key are used to sign etcd server and peer certificates as well as other client certificates (like the apiserver-etcd-client certificate or the etcd-healthcheck-client certificate). More information on how to setup external etcd with kubeadm can be found [here](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/setup-ha-etcd-with-kubeadm/#setting-up-the-cluster).

Once the etcd cluster is setup, you will need the following files from the etcd cluster:

1. `/etc/kubernetes/pki/apiserver-etcd-client.crt` and `/etc/kubernetes/pki/apiserver-etcd-client.key`
2. `/etc/kubernetes/pki/etcd/ca.crt`

You'll use these files to create the necessary Secrets on the management cluster (see the "Creating the required Secrets" section).

### Setting up etcd with etcdadm (Alpha)

`etcdadm` creates the CA if one does not exist, uses it to sign its server and peer certificates, and finally to sign the API server etcd client certificate. The CA's `crt` and `key` generated using `etcdadm` are stored in `/etc/etcd/pki/ca.crt` and `/etc/etcd/pki/ca.key`. `etcdadm` also generates a certificate for the API server etcd client; the certificate and private key are found at `/etc/etcd/pki/apiserver-etcd-client.crt` and `/etc/etcd/pki/apiserver-etcd-client.key`, respectively.

Once the etcd cluster has been bootstrapped using `etcdadm`, you will need the following files from the etcd cluster:

1. `/etc/etcd/pki/apiserver-etcd-client.crt` and `/etc/etcd/pki/apiserver-etcd-client.key`
2. `/etc/etcd/pki/etcd/ca.crt`

You'll use these files in the next section to create the necessary Secrets on the management cluster.

## Creating the required Secrets

Regardless of the method used to bootstrap the etcd cluster, you will need to use the certificates copied from the etcd cluster to create some [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/#creating-a-secret-using-kubectl-create-secret) on the management cluster.

In the commands below to create the Secrets, substitute `$CLUSTER_NAME` with the name of the workload cluster to be created by CAPI, and substitute `$CLUSTER_NAMESPACE` with the name of the namespace where the workload cluster will be created. The namespace can be omitted if the workload cluster will be created in the default namespace.

First, you will need to create a Secret containing the API server etcd client certificate and key. This command assumes the certificate and private key are in the current directory; adjust your command accordingly if they are not:

```bash
# Kubernetes API server etcd client certificate and key
$ kubectl create secret tls $CLUSTER_NAME-apiserver-etcd-client \
  --cert apiserver-etcd-client.crt \
  --key apiserver-etcd-client.key \
  --namespace $CLUSTER_NAMESPACE
```

Next, create a Secret for the etcd cluster's CA certificate. The `kubectl create secret tls` command requires _both_ a certificate and a key, but the key isn't needed by CAPI. Instead, use the `kubectl create secret generic` command, and note that the file containing the CA certificate **must** be named `tls.crt`:

```bash
# Etcd's CA crt file to validate the generated client certificates
$ kubectl create secret generic $CLUSTER_NAME-etcd \
  --from-file tls.crt \
  --namespace $CLUSTER_NAMESPACE
```

**Note:** The above commands will base64 encode the certificate/key files by default.

Alternatively you can base64 encode the files and put them in two secrets. The secrets must be formatted as follows and the cert material must be base64 encoded:

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

The Secrets must be created _before_ creating the workload cluster.

## Configuring CABPK

Once the Secrets are in place on the management cluster, the rest of the process leverages standard `kubeadm` configuration. Configure your ClusterConfiguration for the workload cluster as follows:

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

Create your workload cluster as normal. The new workload cluster should use the configured external etcd nodes instead of creating co-located etcd Pods on the control plane nodes.

## Additional Notes/Caveats

* Depending on the provider, additional changes to the workload cluster's manifest may be necessary to ensure the new CAPI-managed nodes have connectivity to the existing etcd nodes. For example, on AWS you will need to leverage the `additionalSecurityGroups` field on the AWSMachine and/or AWSMachineTemplate objects to add the CAPI-managed nodes to a security group that has connectivity to the existing etcd cluster. Other mechanisms exist for other providers.
