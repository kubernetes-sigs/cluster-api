# Troubleshooting

## Labeling nodes with reserved labels such as `node-role.kubernetes.io` fails with `kubeadm` error during bootstrap.

Reserved labels cannot be set during bootstrapping (for example, by using `node-labels` in `kubeletExtraArgs` when bootstrapping with [CAPBK](https://github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm)) due to a [restriction](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#noderestriction) on the kubelet self-assigning reserved labels such as `node-roles`.  

In order to achieve this, assinging of such labels to nodes needs to be done after the bootstrap process is over. One way to do that is to introduce a separate step that does the job of applying reserved labels after the bootstrap is over using kubectl as shown below.

```
kubectl label nodes <name> node-role.kubernetes.io/worker=""
```
