## Experimental CA rotation procedure

<aside class="note warning">

<h1>Cluster API CA rotation support</h1>

CA rotation is not supported (nor automated) by Cluster API, please use this procedure at your own risk!
Rotating CA is a tough Kubernetes topic on its own, it's even harder when everything is wrapped around Cluster API automation and immutability concepts.

An issue is currently tracking discussions around this feature request : [Automatic CA rotation in CAPI #7721](https://github.com/kubernetes-sigs/cluster-api/issues/7721). Reading the issue is strongly advised to really be sure of what you are going to do.

The recommended strategy in CAPI is to destroy and recreate clusters ahead of the expirty date to avoid CA expiry.

</aside>

## CA manual rotation step-by-step guide

### Rotating `cluster-ca`, `cluster-proxy` & `cluster-etcd` CA Secrets

During this step we're going to use the names `CA1`, `CA2`, `pubCA` and `CAkey`. These are:

* `CA1` = old CA that needs to be replaced
* `CA2` = new CA that will replace the old one and signs all new kubernetes certificates
* `pubCA1+2` = means that you should concatenate CA cert1 and CA cert2 (in this order)
* `CAkey2` = this is the private key for the new CA

<h1>PREFLIGHT CA ROTATION OPERATIONS</h1>

Workload cluster and management cluster needs to recognize the incoming new CA before switching all control-plane nodes all at once.
By injecting the new CAcert as bundle in all kubernetes components and rolling everything, we ensure that kubernetes api, kubelet and CAPI controller will still work immediately after the control-plane cert rotation.

- [ ] a working & stable workload cluster should have 3 secrets provisionned with "old" CAs
- [ ] backup/copy existing Secret as `[cluster name]-**-backup` to keep old state of the workload cluster & allow recovery
- [ ] in management cluster : update CA Secret with `pubCA1+2` and `CAkey1`
- [ ] in management cluster : update PROXY Secret with `pubPROXY1+2` and `PROXYkey1`
- [ ] in management cluster : update ETCD Secret with `pubETCD1+2` and `ETCDkey1`
- [ ] ensure kube-controller-manager still signs using old pubCA only with `--cluster-signing-cert-file` (It doesn't support any CA bundle, as explained in [Kubernetes manual CA rotation - official](https://kubernetes.io/docs/tasks/tls/manual-rotation-of-ca-certificates/#rotate-the-ca-certificates-manually) )
- [ ] edit workload cluster's cluster-info ConfigMap to match `pubCA1+2`
- [ ] in management cluster : rollout all `MachineDepoyment` worker machines used by the workload cluster (e.g set `RolloutAfter` field or run `clusterctl alpha rollout restart` command)
- [ ] in management cluster : rollout `KubeadmControlPlane` machines used by the workload cluster (e.g set `RolloutAfter` field or run `clusterctl alpha rollout restart` command)
- [ ] rollout restart all pods running in workload cluster

<h1>LIVE OPERATIONS</h1>

<aside class="note warning">

<h1>Machine remote access is required</h1>

In order to manually replace CA and renew all control-plane certificates at once, this procedure requires logging into systems using SSH. So it breaks CAPI *"no direct access to the machines"* concept and (by nature) will never be generalized nor automated "as it is" by any CAPI controller.

This is only a manual workaround and may not work as expected on all systems. 

</aside>

- [ ] **pause CAPI cluster reconciliation**
- [ ] replace key & crt files on all control planes & renew cert **on all control-plane nodes at the same time**:

```bash
# as root
# kube CA
echo -n "<new CA private key content>" > /etc/kubernetes/pki/ca.key
chmod 0600 /etc/kubernetes/pki/ca.key

echo -n "<new CA pubcert + old CA pubcert content>" > /etc/kubernetes/pki/ca.crt
chmod 0644 /etc/kubernetes/pki/ca.crt

echo -n "<new CA public key content>" > /etc/kubernetes/pki/cluster-signing-ca.crt
chmod 0644 /etc/kubernetes/pki/cluster-signing-ca.crt

# kube PROXY
echo -n "<new PROXY private key content>" > /etc/kubernetes/pki/front-proxy-ca.key
chmod 0600 /etc/kubernetes/pki/front-proxy-ca.key

echo -n "<new PROXY pubcert + old PROXY pubcert content>" > /etc/kubernetes/pki/front-proxy-ca.crt
chmod 0644 /etc/kubernetes/pki/front-proxy-ca.crt

# kube ETCD
echo -n "<new ETCD private key content>" > /etc/kubernetes/pki/etcd/ca.key
chmod 0600 /etc/kubernetes/pki/etcd/ca.key

echo -n "<new ETCD pubcert + old ETCD pubcert content>" > /etc/kubernetes/pki/etcd/ca.crt
chmod 0644 /etc/kubernetes/pki/etcd/ca.crt

# regen certs on all VM at the same time !
kubeadm certs renew all

crictl stop $(crictl ps |grep kube-apiserver |cut -d' ' -f1)
crictl stop $(crictl ps |grep kube-controller-manager |cut -d' ' -f1)
crictl stop $(crictl ps |grep kube-scheduler |cut -d' ' -f1)

# fetch admin token to fill it back in CAPI cluster...
cat /etc/kubernetes/admin.conf
```

- [ ] in management cluster : deploy workload cluster's CA Secret `pubCA2` / `CAkey2`
- [ ] in management cluster : deploy workload cluster's PROXY Secret `pubPROXY2` / `PROXYkey2`
- [ ] in management cluster : deploy workload cluster's ETCD Secret `pubETCD2` / `ETCDkey2`
- [ ] in management cluster : update kubeconfig cluster admin Secret with new admin.conf kubeconfig content from any control-plane node local file
- [ ] edit workload cluster's cluster-info ConfigMap to only store `pubCA2`
- [ ] **resume CAPI cluster reconciliation**
- [ ] in management cluster : rollout restart all Worker machines
- [ ] in management cluster : rollout restart all CP machines
- [ ] (optional) rollout restart all workload cluster's pods
