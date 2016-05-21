### Get the template file

First of all, download the dns template

[skydns template](skydns.yaml.in)

### Set environment variables

Then you need to set `ARCH` `DNS_REPLICAS`, `DNS_DOMAIN` and `DNS_SERVER_IP` envs

```shell
# Supported: amd64, arm, arm64 and ppc64le
export ARCH=amd64
export DNS_REPLICAS=1
export DNS_DOMAIN=cluster.local
export DNS_SERVER_IP=10.0.0.10
```

### Replace the corresponding value in the template and create the pod

```shell
# If the kube-system namespace isn't already created, create it
$ kubectl get ns
$ kubectl create namespace kube-system

$ sed -e "s/ARCH/${ARCH}/g;s/DNS_REPLICAS/${DNS_REPLICAS}/g;s/DNS_DOMAIN/${DNS_DOMAIN}/g;s/DNS_SERVER_IP/${DNS_SERVER_IP}/g" skydns.yaml.in | kubectl create -f -
```

### Test if DNS works

Follow [this link](https://releases.k8s.io/release-1.2/cluster/addons/dns#how-do-i-test-if-it-is-working) to check it out.
