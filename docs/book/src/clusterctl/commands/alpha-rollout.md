# clusterctl alpha rollout

The `clusterctl alpha rollout` command manages the rollout of a Cluster API resource. It consists of several sub-commands which are documented below. 

<aside class="note">

<h1> Valid Resource Types </h1>

Currently, only the following Cluster API resources are supported by the rollout command:

- kubeadmcontrolplanes
- machinedeployments

Third party controlplane providers are supported as long as they:

- Implement rolloutAfter in their spec
- Are in the controlplane.cluster.x-k8s.io group

</aside>

### Restart 

Use the `restart` sub-command to force an immediate rollout. Note that rollout refers to the replacement of existing machines with new machines using the desired rollout strategy (default: rolling update). For example, here the MachineDeployment `my-md-0` will be immediately rolled out:

```bash
clusterctl alpha rollout restart machinedeployment/my-md-0
```

Or for a third party controlplane:

```bash
clusterctl alpha rollout restart my-controlplane-kind.v1beta1/my-kcp
```

### Undo

Use the `undo` sub-command to rollback to an earlier revision. For example, here the MachineDeployment `my-md-0` will be rolled back to revision number 3. If the `--to-revision` flag is omitted, the MachineDeployment will be rolled back to the revision immediately preceding the current one. If the desired revision does not exist, the undo will return an error.

```bash
clusterctl alpha rollout undo machinedeployment/my-md-0 --to-revision=3
```

### Pause/Resume

Use the `pause` sub-command to pause a Cluster API resource. The command is a NOP if the resource is already paused. Note that internally, this command sets the `Paused` field within the resource spec (e.g. MachineDeployment.Spec.Paused) to true. 

```bash
clusterctl alpha rollout pause machinedeployment/my-md-0
```

Use the `resume` sub-command to resume a currently paused Cluster API resource. The command is a NOP if the resource is currently not paused. 

```bash
clusterctl alpha rollout resume machinedeployment/my-md-0
```

<aside class="note warning">

<h1> Warning </h1>

Paused resources will not be reconciled by a controller. By resuming a resource, we allow it to be reconciled again. 

</aside>
