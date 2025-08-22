# clusterctl get kubeconfig

This command prints the kubeconfig of an existing workload cluster into stdout.
This functionality is available in clusterctl v0.3.9 or newer.

## Examples

Get the kubeconfig of a workload cluster named foo.

```bash
clusterctl get kubeconfig foo
```

Get the kubeconfig of a workload cluster named foo in the namespace bar

```bash
clusterctl get kubeconfig foo --namespace bar
```

Get the kubeconfig of a workload cluster named foo using a specific context bar

```bash
clusterctl get kubeconfig foo --kubeconfig-context bar
```
