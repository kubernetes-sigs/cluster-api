# Rollout Changes to Cluster Configuration

When something is changed in the Cluster Configuration, Cluster API triggers a rollout process.

The rollout process happens in an incremental way, ensuring that the Cluster respects availability
requirements for both control plane and workers. In case of problems, the rollout slows down or stops.

A rollout can happen using two different approaches:
- By deleting old machines and creating new ones, similar to how changes are rolled out for `Deployment` in Kubernetes (rolling update)
- By changing existing machines in-place, see proposal [In place update].

When in-place update is possible, it should be preferred to a rolling update.

> [!IMPORTANT]
> While users have full control over availability rules governing rollouts as well as over changes applied to the Cluster,
> the responsibility to determine how a change is performed is deferred to Cluster API.
>
> This approach allows reducing the risks related to rolling out changes, as well as the risks derived from
> the complexity of the limitations and constraints that are specific to each type of change,
> and which might also differ across providers.

Please also note that Cluster API implements an additional mechanism to entirely avoid a rollout (either rolling update or in-place)
when you are changing fields that have no direct impact on machines.

E.g. `spec.template.spec.deletion.nodeDeletionTimeoutSeconds` in a `MachineDeployment` is a field that is used by Cluster API
controllers during the deletion process, but it doesn't have impact on the Machine at all, and thus the change can be
propagated to the controlled machines without a rollout.

See proposal [In place propagation of changes affecting Kubernetes objects only].

## Control Plane

Changes to the control plane object might include changes to the `ControlPlane` resource and/or changes
to the corresponding `InfrastructureTemplate`.

> [!NOTE]
> The control plane resource embeds the `BootstrapConfig`, e.g. the `KubeadmControlPlane` embeds a `kubeadmConfigSpec`.

### InfrastructureTemplate Rotation

If it is required to change the image used by a machine, or to perform some other infrastructure configuration change,
 the `InfrastructureTemplate` resource referenced by the `spec.machineTemplate.spec.infrastructureRef`
field in the `ControlPlane` resource must be changed.

Since `InfrastructureTemplate` resources are immutable, the recommended approach is to:

1. Duplicate an existing template.
   Users can use `kubectl get <MachineTemplateType> <name> -o yaml > file.yaml`
   to retrieve a template configuration from a running cluster to serve as a starting
   point.
2. Update the desired fields.
   Fields that might need to be modified could include the SSH key, the AWS instance
   type, or the Azure VM size. Refer to the provider-specific documentation
   for more details on the specific fields that each provider requires or accepts.
3. Give the newly-modified template a new name by modifying the `metadata.name` field
   (or by using `metadata.generateName`).

e.g. if using the VSphere infrastructure provider

```diff
---
# old template
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: VSphereMachineTemplate
metadata:
  name: my-cluster-control-plane-machine-infrastructure-v1
spec:
  template:
    spec:
      ...
      storagePolicyName: ssd
---
# new template
apiVersion: infrastructure.cluster.x-k8s.io/v1beta2
kind: VSphereMachineTemplate
metadata:
+  name: my-cluster-control-plane-machine-infrastructure-v2
-  name: my-cluster-control-plane-machine-infrastructure-v1
spec:
  template:
    spec:
      ...
+     storagePolicyName: ultra-fast-ssd
-     storagePolicyName: ssd
```

The next paragraph will describe how to edit the `ControlPlane` resource for using the new `InfrastructureTemplate`.

### Control Plane Changes

In order to apply control plane changes you should perform the following changes _in a single operation_:

- Edit the `ControlPlane` resource, apply desired changes.
- If required, change the bootstrap configuration setting embedded in the `ControlPlane` resource.
  e.g. in the `KubeadmControlPlane` you should change fields under `kubeadmConfigSpec`.
- If required, modify the existing `ControlPlane` resource to reference the new `InfrastructureTemplate` by
  changing the `spec.machineTemplate.spec.infrastructureRef` field.

e.g.

```diff
---
apiVersion: controlplane.cluster.x-k8s.io/v1beta2
kind: KubeadmControlPlane
metadata:
  name: my-cluster-controlplane
spec:
  ...
  replicas: 3
  version: v1.34.0
  kubeadmConfigSpec:
    clusterConfiguration:
      dns:
+       imageTag: v1.14.6
-       imageTag: v1.14.0
  machineTemplate:
    spec:
      infrastructureRef:
        apiGroup: infrastructure.cluster.x-k8s.io
        kind: VSphereMachineTemplate
+       name: my-cluster-control-plane-machine-infrastructure-v2
-       name: my-cluster-control-plane-machine-infrastructure-v1
```

Once all the changes above are submitted, Cluster API will trigger a rollout of the machines controlled by the control plane.

> [!TIP]
> If you are using [ClusterClass], you can change the control plane and the corresponding infrastructure configuration and
> bootstrap configuration with a single change to the Cluster resource.

### Rollout Strategy

Each control plane provider might support different rollout strategy.

In case of the kubeadm control plane provider, only the `RollingUpdate` strategy is supported and
the `spec.rollout.strategy.rollingUpdate.maxSurge` field defines how it should be performed:
- If `maxSurge` is 1 (default) the kubeadm control plane provider is allowed to create an additional machine when performed rollout;
  as a consequence, if in-place update is not possible, the rollout will start by scaling up.
- If `maxSurge` is 0 the kubeadm control plane provider is not allowed to create an additional machine when performed rollout;
  as a consequence, if in-place update is not possible, the rollout will start by scaling down.

## Workers

Changes to workers might include changes to the `MachineDeployment` resource and/or changes
to the corresponding `InfrastructureTemplate` or `BootstrapConfigTemplate`.

The `MachinePool` resource has a similar behavior.

### InfrastructureTemplate Rotation

Changes to the `InfrastructureTemplate` resource referenced by MachineDeployment's `spec.template.spec.infrastructureRef`
field are performed by rotating the template.

1. Duplicate an existing template.
2. Update the desired fields.
3. Give the newly-modified template a new name.

The next paragraphs will describe how to edit the `MachineDeployment` resource for using the new `InfrastructureTemplate`.

### BootstrapConfigTemplate Rotation

Changes to the `BootstrapConfigTemplate` resource referenced by MachineDeployment's `spec.template.spec.bootstrap.configRef`
field are performed by rotating the template.

1. Duplicate an existing template.
2. Update the desired fields.
3. Give the newly-modified template a new name.

e.g. if using the Kubeadm bootstrap provider

```diff
---
# old template
apiVersion: bootstrap.cluster.x-k8s.io/v1beta2
kind: KubeadmConfigTemplate
metadata:
  name: my-cluster-worker-machine-config-v1
spec:
  template:
    spec:
      joinConfiguration:
        nodeRegistration:
          kubeletExtraArgs:
          - name: cloud-provider
            value: external
  ...
---
# new template
apiVersion: bootstrap.cluster.x-k8s.io/v1beta2
kind: KubeadmConfigTemplate
metadata:
+ name: my-cluster-worker-machine-config-v2
- name: my-cluster-worker-machine-config-v1
spec:
  template:
    spec:
      joinConfiguration:
        nodeRegistration:
          kubeletExtraArgs:
          - name: cloud-provider
            value: external
+         - name: max-pods
+           value: "150"
  ...
```

The next paragraph will describe how to edit the `MachineDeployment` resource for using the new `BootstrapConfigTemplate`.

### MachineDeployment Changes

In order to apply changes you should perform the following changes _in a single operation_:

- Edit the `MachineDeployment` resource. e.g. to change the failure domain where the Machines are placed,
  you should change the `spec.template.spec.failureDomain` field.
- If required, modify the existing `MachineDeployment` resource to reference the new `BootstrapConfigTemplate` by
  changing the `spec.template.spec.bootstrap.configRef` field.
- If required, modify the existing `MachineDeployment` resource to reference the new `InfrastructureTemplate` by
  changing the `spec.template.spec.infrastructureRef` field.

e.g.

```diff
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: MachineDeployment
metadata:
  name: my-cluster-md-0
spec:
  ...
  template:
    spec:
+     failureDomain: us-east
-     failureDomain: us-west
      bootstrap:
        configRef:
          apiGroup: bootstrap.cluster.x-k8s.io
          kind: KubeadmConfigTemplate
+         name: my-cluster-worker-machine-config-v2
-         name: my-cluster-worker-machine-config-v1
      infrastructureRef:
        apiGroup: infrastructure.cluster.x-k8s.io
        kind: VSphereMachineTemplate
+       name: my-cluster-worker-machine-infrastructure-v2
-       name: my-cluster-worker-machine-infrastructure-v1
```

Once all the changes above are submitted, Cluster API will trigger a rollout of the machines controlled by the `MachineDeployment`.

> [!TIP]
> If you are using [ClusterClass], you can change the `MachineDeployment` and the corresponding infrastructure configuration and
> bootstrap configuration with a single change to the Cluster resource.

### Rollout Strategy

`MachineDeployment`s support different strategies for rolling out changes to Machines:

- RollingUpdate

  Changes are rolled out by honouring the `spec.rollout.strategy.rollingUpdate.maxUnavailable` and `maxSurge` values.
  Only values allowed are of type Int or Strings with an integer and percentage symbol e.g "5%".

- OnDelete

  Changes are rolled out driven by the user or any entity deleting the old `Machines`. Only when a `Machine` is fully deleted a new one will come up.

For a more in-depth look at how `MachineDeployments` manage scaling events, take a look at the [`MachineDeployment`
controller documentation](../../../developer/core/controllers/machine-deployment.md) and the [`MachineSet` controller
documentation](../../../developer/core/controllers/machine-set.md).

## Schedule a Rollout

The `KubeadmControlPlane` and `MachineDeployment` resources have a `spec.rollout.after` field that can be
set to a timestamp (RFC-3339) after which a rollout should be triggered regardless of whether there
were any changes to `KubeadmControlPlane.spec`/`MachineDeployment.spec.template` or not. This would
roll out replacement nodes which can be useful e.g. to perform certificate rotation, reflect changes
to machine templates, move to new machines, etc.

Note that this field can only be used for triggering a rollout, not for delaying one. Specifically,
a rollout can also happen before the time specified in `spec.rollout.after` if any changes are made to
the spec before that time.

Alternatively, a rollout can be triggered immediately by running the following commands:

```shell
# Trigger a KubeadmControlPlane rollout.
clusterctl alpha rollout restart kubeadmcontrolplane/my-kcp

# Trigger a MachineDeployment rollout.
clusterctl alpha rollout restart machinedeployment/my-md-0
```

## Using ClusterClass

If you are using [ClusterClass], you can change both control plane, workers, and the corresponding infrastructure
configuration and bootstrap configuration with a single change to the Cluster resource.

e.g. In order to change the failure domain for a MachineDeployment all you have to do is:

```diff
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-docker-cluster
spec:
  ...
  topology:
    classRef:
      name: vsphere-clusterclass-v0.1.0
    controlPlane:
      replicas: 5
    workers:
      machineDeployments:
      - class: default-worker
        name: md-0
        replicas: 10
+       failureDomain: us-east
-       failureDomain: us-west
```

> [!NOTE]
> When using [ClusterClass] infrastructure settings or machine bootstrap settings are exposed through
> variables defined by the [ClusterClass] author.

Cluster API will take care of propagating changes to the underlying `ControlPlane`, `MachineDeployment` or `MachinePool` and
corresponding `BootstrapConfigTemplate` and `InfrastructureTemplate` automatically.

Most notably, Cluster API will also take care of template rotation for both `BootstrapConfigTemplate` and `InfrastructureTemplate` automatically.

> [!NOTE]
> If a cluster is using a [ClusterClass], direct changes to `ControlPlane`, `MachineDeployment` or `MachinePool`
> resources will be overridden with the corresponding settings from the `Cluster` resource.

[ClusterClass]: ../configuration/cluster-class/index.md
[In place update]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240807-in-place-updates.md
[In place propagation of changes affecting Kubernetes objects only]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20221003-In-place-propagation-of-Kubernetes-objects-only-changes.md
