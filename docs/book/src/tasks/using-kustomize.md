# Using Kustomize with Workload Cluster Manifests

Although the `clusterctl generate cluster` command exposes a number of different configuration values
for customizing workload cluster YAML manifests, some users may need additional flexibility above
and beyond what `clusterctl generate cluster` or the example "flavor" templates that some CAPI providers
supply (as an example, see [these flavor templates](https://github.com/kubernetes-sigs/cluster-api-provider-azure/tree/main/templates/flavors)
for the Cluster API Provider for Azure). In the future, a [templating solution](https://github.com/kubernetes-sigs/cluster-api/issues/3252)
may be integrated into `clusterctl` to help address this need, but in the meantime users can use
`kustomize` as a solution to this need.

This document provides a few examples of using `kustomize` with Cluster API. All of these examples
assume that you are using a directory structure that looks something like this:

```
.
├── base
│   ├── base.yaml
│   └── kustomization.yaml
└── overlays
    ├── custom-ami
    │   ├── custom-ami.json
    │   └── kustomization.yaml
    └── mhc
        ├── kustomization.yaml
        └── workload-mhc.yaml
```

In the overlay directories, the "base" (unmodified) Cluster API configuration (perhaps generated using
`clusterctl generate cluster`) would be referenced as a resource in `kustomization.yaml` using `../../base`.

## Example: Using Kustomize to Specify Custom Images

Users can use `kustomize` to specify custom OS images for Cluster API nodes. Using the Cluster API
Provider for AWS (CAPA) as an example, the following `kustomization.yaml` would leverage a JSON 6902 patch
to modify the AMI for nodes in a workload cluster:

```yaml
---
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
patchesJson6902:
  - path: custom-ami.json
    target:
      group: infrastructure.cluster.x-k8s.io
      kind: AWSMachineTemplate
      name: ".*"
      version: v1alpha3
```

The referenced JSON 6902 patch in `custom-ami.json` would look something like this:

```json
[
    { "op": "add", "path": "/spec/template/spec/ami", "value": "ami-042db61632f72f145"}
]
```

This configuration assumes that the workload cluster _only_ uses MachineDeployments. Since
MachineDeployments and the KubeadmControlPlane both leverage AWSMachineTemplates, this `kustomize`
configuration would catch all nodes in the workload cluster.

## Example: Adding a MachineHealthCheck for a Workload Cluster

Users could also use `kustomize` to combine additional resources, like a MachineHealthCheck (MHC), with the
base Cluster API manifest. In an overlay directory, specify the following in `kustomization.yaml`:

```yaml
---
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
  - workload-mhc.yaml
```

The content of the `workload-mhc.yaml` file would be the definition of a standard MHC:

```yaml
apiVersion: cluster.x-k8s.io/v1alpha3
kind: MachineHealthCheck
metadata:
  name: md-0-mhc
spec:
  clusterName: test
  # maxUnhealthy: 40%
  nodeStartupTimeout: 10m
  selector:
    matchLabels:
      cluster.x-k8s.io/deployment-name: md-0
  unhealthyConditions:
  - type: Ready
    status: Unknown
    timeout: 300s
  - type: Ready
    status: "False"
    timeout: 300s
```

You would want to ensure the `clusterName` field in the MachineHealthCheck manifest appropriately
matches the name of the workload cluster, taking into account any transformations you may have specified
in `kustomization.yaml` (like the use of "namePrefix" or "nameSuffix").

Running `kustomize build .` with this configuration would append the MHC to the base
Cluster API manifest, thus creating the MHC at the same time as the workload cluster.

## Modifying Names

The `kustomize` "namePrefix" and "nameSuffix" transformers are not currently "Cluster API aware."
Although it is possible to use these transformers with Cluster API manifests, doing so requires separate
patches for Clusters versus infrastructure-specific equivalents (like an AzureCluster or a vSphereCluster).
This can significantly increase the complexity of using `kustomize` for this use case.

Modifying the transformer configurations for `kustomize` can make it more effective with Cluster API.
For example, changes to the `nameReference` transformer in `kustomize` will enable `kustomize` to know
about the references between Cluster API objects in a manifest. See
[here](https://github.com/kubernetes-sigs/kustomize/tree/master/examples/transformerconfigs) for more
information on transformer configurations.

Add the following content to the `namereference.yaml` transformer configuration:

```yaml
- kind: Cluster
  group: cluster.x-k8s.io
  version: v1alpha3
  fieldSpecs:
  - path: spec/clusterName
    kind: MachineDeployment
  - path: spec/template/spec/clusterName
    kind: MachineDeployment

- kind: AWSCluster
  group: infrastructure.cluster.x-k8s.io
  version: v1alpha3
  fieldSpecs:
  - path: spec/infrastructureRef/name
    kind: Cluster

- kind: KubeadmControlPlane
  group: controlplane.cluster.x-k8s.io
  version: v1alpha3
  fieldSpecs:
  - path: spec/controlPlaneRef/name
    kind: Cluster

- kind: AWSMachine
  group: infrastructure.cluster.x-k8s.io
  version: v1alpha3
  fieldSpecs:
  - path: spec/infrastructureRef/name
    kind: Machine

- kind: KubeadmConfig
  group: bootstrap.cluster.x-k8s.io
  version: v1alpha3
  fieldSpecs:
  - path: spec/bootstrap/configRef/name
    kind: Machine

- kind: AWSMachineTemplate
  group: infrastructure.cluster.x-k8s.io
  version: v1alpha3
  fieldSpecs:
  - path: spec/template/spec/infrastructureRef/name
    kind: MachineDeployment
  - path: spec/infrastructureTemplate/name
    kind: KubeadmControlPlane

- kind: KubeadmConfigTemplate
  group: bootstrap.cluster.x-k8s.io
  version: v1alpha3
  fieldSpecs:
  - path: spec/template/spec/bootstrap/configRef/name
    kind: MachineDeployment
```

Including this custom configuration in a `kustomization.yaml` would then enable the use of simple
"namePrefix" and/or "nameSuffix" directives, like this:

```yaml
---
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
configurations:
  - namereference.yaml
namePrefix: "blue-"
nameSuffix: "-dev"
```

Running `kustomize build. ` with this configuration would modify the name of all the Cluster API
objects _and_ the associated referenced objects, adding "blue-" at the beginning and appending "-dev"
at the end.
