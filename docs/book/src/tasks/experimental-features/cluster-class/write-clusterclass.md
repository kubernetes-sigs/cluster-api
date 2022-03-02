
# Writing a ClusterClass

A ClusterClass becomes more useful and valuable when it can be used to create many Cluster of a similar 
shape. The goal of this document is to explain how ClusterClasses can be written in a way that they are 
flexible enough to be used in as many Cluster as possible by supporting variants of the same base Cluster shape.

**Table of Contents**

* [Basic ClusterClass](#basic-clusterclass)
* [ClusterClass with MachineHealthChecks](#clusterclass-with-machinehealthchecks)
* [ClusterClass with patches](#clusterclass-with-patches)
* [Advanced features of ClusterClass with patches](#advanced-features-of-clusterclass-with-patches)
    * [MachineDeployment variable overrides](#machinedeployment-variable-overrides)
    * [Builtin variables](#builtin-variables)
    * [Complex variable types](#complex-variable-types)
    * [Using variable values in JSON patches](#using-variable-values-in-json-patches)
    * [Optional patches](#optional-patches)

## Basic ClusterClass

The following example shows a basic ClusterClass. It contains templates to shape the control plane, 
infrastructure and workers of a Cluster. When a Cluster is using this ClusterClass, the templates 
are used to generate the objects of the managed topology of the Cluster.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: docker-clusterclass-v0.1.0
spec:
  controlPlane:
    ref:
      apiVersion: controlplane.cluster.x-k8s.io/v1beta1
      kind: KubeadmControlPlaneTemplate
      name: docker-clusterclass-v0.1.0
      namespace: default
    machineInfrastructure:
      ref:
        kind: DockerMachineTemplate
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        name: docker-clusterclass-v0.1.0
        namespace: default
  infrastructure:
    ref:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
      kind: DockerClusterTemplate
      name: docker-clusterclass-v0.1.0-control-plane
      namespace: default
  workers:
    machineDeployments:
    - class: default-worker
      template:
        bootstrap:
          ref:
            apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
            kind: KubeadmConfigTemplate
            name: docker-clusterclass-v0.1.0-default-worker
            namespace: default
        infrastructure:
          ref:
            apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
            kind: DockerMachineTemplate
            name: docker-clusterclass-v0.1.0-default-worker
            namespace: default
```

The following example shows a Cluster using this ClusterClass. In this case a `KubeadmControlPlane` 
with the corresponding `DockerMachineTemplate`, a `DockerCluster` and a `MachineDeployment` with 
the corresponding `KubeadmConfigTemplate` and `DockerMachineTemplate` will be created. This basic 
ClusterClass is already very flexible. Via the topology on the Cluster the following can be configured:
* `.spec.topology.version`: the Kubernetes version of the Cluster
* `.spec.topology.controlPlane`: ControlPlane replicas and their metadata
* `.spec.topology.workers`: MachineDeployments and their replicas, metadata and failure domain

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: my-docker-cluster
spec:
  topology:
    class: docker-clusterclass
    version: v1.22.4
    controlPlane:
      replicas: 3
      metadata:
        labels:
          cpLabel: cpLabelValue 
        annotations:
          cpAnnotation: cpAnnotationValue
    workers:
      machineDeployments:
      - class: default-worker
        name: md-0
        replicas: 4
        metadata:
          labels:
            mdLabel: mdLabelValue
          annotations:
            mdAnnotation: mdAnnotationValue
        failureDomain: region
```

Best practices:
* The ClusterClass name should be generic enough to make sense across multiple clusters, i.e. a 
  name which corresponds to a single Cluster, e.g. "my-cluster", is not recommended.
* Try to keep the ClusterClass names short and consistent (if you publish multiple ClusterClasses).
* As a ClusterClass usually evolves over time and you might want to rebase Clusters from one version 
  of a ClusterClass to another, consider including a version suffix in the ClusterClass name.
  For more information about changing a ClusterClass please see: [Changing a ClusterClass].
* Prefix the templates used in a ClusterClass with the name of the ClusterClass.
* Don't reuse the same template in multiple ClusterClasses. This is automatically taken care
  of by prefixing the templates with the name of the ClusterClass.

<aside class="note">

For a full example ClusterClass for CAPD you can take a look at
[clusterclass-quickstart.yaml](https://github.com/kubernetes-sigs/cluster-api/blob/main/test/infrastructure/docker/templates/clusterclass-quick-start.yaml)
(which is also used in the CAPD quickstart with ClusterClass).

</aside>

<aside class="note">

<h1>Tip: clusterctl alpha topology plan</h1>

The `clusterctl alpha topology plan` command can be used to test ClusterClasses; the output will show
you how the resulting Cluster will look like, but without actually creating it.
For more details please see: [clusterctl alpha topology plan].

</aside>

## ClusterClass with MachineHealthChecks

`MachineHealthChecks` can be configured in the ClusterClass for the control plane and for a 
MachineDeployment class. The following configuration makes sure a `MachineHealthCheck` is 
created for the control plane and for every `MachineDeployment` using the `default-worker` class.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: docker-clusterclass-v0.1.0
spec:
  controlPlane:
    ...
    machineHealthCheck:
      maxUnhealthy: 33%
      nodeStartupTimeout: 15m
      unhealthyConditions:
      - type: Ready
        status: Unknown
        timeout: 300s
      - type: Ready
        status: "False"
        timeout: 300s
  workers:
    machineDeployments:
    - class: default-worker
      ...
      machineHealthCheck:
        unhealthyRange: "[0-2]"
        nodeStartupTimeout: 10m
        unhealthyConditions:
        - type: Ready
          status: Unknown
          timeout: 300s
        - type: Ready
          status: "False"
          timeout: 300s
```

## ClusterClass with patches

As shown above, basic ClusterClasses are already very powerful. But there are cases where 
more powerful mechanisms are required. Let's assume you want to manage multiple Clusters 
with the same ClusterClass, but they require different values for a field in one of the 
referenced templates of a ClusterClass.

A concrete example would be to deploy Clusters with different registries. In this case, 
every cluster needs a Cluster-specific value for `.spec.kubeadmConfigSpec.clusterConfiguration.imageRepository` 
in `KubeadmControlPlane`. Use cases like this can be implemented with ClusterClass patches.

**Defining variables in the ClusterClass**

The following example shows how variables can be defined in the ClusterClass.
A variable definition specifies the name and the schema of a variable and if it is 
required. The schema defines how a variable is defaulted and validated. It supports 
a subset of the schema of CRDs. For more information please see the [godoc](https://doc.crds.dev/github.com/kubernetes-sigs/cluster-api/cluster.x-k8s.io/ClusterClass/v1beta1#spec-variables-schema-openAPIV3Schema).

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: docker-clusterclass-v0.1.0
spec:
  ...
  variables:
  - name: imageRepository
    required: true
    schema:
      openAPIV3Schema:
        type: string
        description: ImageRepository is the container registry to pull images from.
        default: k8s.gcr.io
        example: k8s.gcr.io
```

<aside class="note">

<h1>Supported types</h1>

The following basic types are supported: `string`, `integer`, `number` and `boolean`. We are also 
supporting complex types, please see the [complex variables](#complex-variables) section.

</aside>

**Defining patches in the ClusterClass**

The variable can then be used in a patch to set a field on a template referenced in the ClusterClass.
The `selector` specifies on which template the patch should be applied. `jsonPatches` specifies which JSON 
patches should be applied to that template. In this case we set the `imageRepository` field of the 
`KubeadmControlPlaneTemplate` to the value of the variable `imageRepository`. For more information 
please see the [godoc](https://doc.crds.dev/github.com/kubernetes-sigs/cluster-api/cluster.x-k8s.io/ClusterClass/v1beta1#spec-patches-definitions).

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: docker-clusterclass-v0.1.0
spec:
  ...
  patches:
  - name: imageRepository
    definitions:
    - selector:
        apiVersion: controlplane.cluster.x-k8s.io/v1beta1
        kind: KubeadmControlPlaneTemplate
        matchResources:
          controlPlane: true
      jsonPatches:
      - op: add
        path: /spec/template/spec/kubeadmConfigSpec/clusterConfiguration/imageRepository
        valueFrom:
          variable: imageRepository
```

<aside class="note">

<h1>Writing JSON patches</h1>

* Only fields below `/spec` can be patched.
* Only `add`, `remove` and `replace` operations are supported.
* It's only possible to append and prepend to arrays. Insertions at a specific index are 
  not supported.
* Be careful, appending or prepending an array variable to an array leads to a nested array
  (for more details please see this [issue](https://github.com/kubernetes-sigs/cluster-api/issues/5944)).

</aside>

**Setting variable values in the Cluster**

After creating a ClusterClass with a variable definition, the user can now provide a value for 
the variable in the Cluster as in the example below.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: my-docker-cluster
spec:
  topology:
    ...
    variables:
    - name: imageRepository
      value: my.custom.registry
```

<aside class="note">

<h1>Variable defaulting</h1>

If the user does not set the value, but the corresponding variable definition in ClusterClass has
a default value, the value is automatically added to the variables list.

</aside>

## Advanced features of ClusterClass with patches

This section will explain more advanced features of ClusterClass patches.

### MachineDeployment variable overrides

If you want to use many variations of MachineDeployments in Clusters, you can either define
a MachineDeployment class for every variation or you can define patches and variables to
make a single MachineDeployment class more flexible.

In the following example we make the `instanceType` of a `AWSMachineTemplate` customizable.
First we define the `workerMachineType` variable and the corresponding patch:

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: aws-clusterclass-v0.1.0
spec:
  ...
  variables:
  - name: workerMachineType
    required: true
    schema:
      openAPIV3Schema:
        type: string
        default: t3.large
  patches:
  - name: workerMachineType
    definitions:
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: AWSMachineTemplate
        matchResources:
          machineDeploymentClass:
            names:
            - default-worker
      jsonPatches:
      - op: add
        path: /spec/template/spec/instanceType
        valueFrom:
          variable: workerMachineType
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: AWSMachineTemplate
metadata:
  name: aws-clusterclass-v0.1.0-default-worker
spec:
  template:
    spec:
      # instanceType: workerMachineType will be set by the patch.
      iamInstanceProfile: "nodes.cluster-api-provider-aws.sigs.k8s.io"
---
...
```

In the Cluster resource the `workerMachineType` variable can then be set cluster-wide and
it can also be overridden for an individual MachineDeployment.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: my-aws-cluster
spec:
  ...
  topology:
    class: aws-clusterclass-v0.1.0
    version: v1.22.0
    controlPlane:
      replicas: 3
    workers:
      machineDeployments:
      - class: "default-worker"
        name: "md-small-workers"
        replicas: 3
        variables:
          overrides:
          # Overrides the cluster-wide value with t3.small.
          - name: workerMachineType
            value: t3.small
      # Uses the cluster-wide value t3.large.
      - class: "default-worker"
        name: "md-large-workers"
        replicas: 3
    variables:
    - name: workerMachineType
      value: t3.large
```

### Builtin variables

In addition to variables specified in the ClusterClass, the following builtin variables can be 
referenced in patches:
- `builtin.cluster.{name,namespace}`
- `builtin.cluster.topology.{version,class}`
- `builtin.cluster.network.{serviceDomain,services,pods,ipFamily}`
- `builtin.controlPlane.{replicas,version,name}`
    - Please note, these variables are only available when patching control plane or control plane 
      machine templates.
- `builtin.controlPlane.machineTemplate.infrastructureRef.name`
    - Please note, these variables are only available when using a control plane with machines and 
      when patching control plane or control plane machine templates.
- `builtin.machineDeployment.{replicas,version,class,name,topologyName}`
    - Please note, these variables are only available when patching the templates of a MachineDeployment 
      and contain the values of the current `MachineDeployment` topology.
- `builtin.machineDeployment.{infrastructureRef.name,bootstrap.configRef.name}`
    - Please note, these variables are only available when patching the templates of a MachineDeployment
      and contain the values of the current `MachineDeployment` topology.

Builtin variables can be referenced just like regular variables, e.g.:
```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: docker-clusterclass-v0.1.0
spec:
  ...
  patches:
  - name: clusterName
    definitions:
    - selector:
      ...
      jsonPatches:
      - op: add
        path: /spec/template/spec/kubeadmConfigSpec/clusterConfiguration/controllerManager/extraArgs/cluster-name
        valueFrom:
          variable: builtin.cluster.name
```

**Tips & Tricks**

Builtin variables can be used to dynamically calculate image names. The version used in the patch 
will always be the same as the one we set in the corresponding MachineDeployment (works the same way 
with `.builtin.controlPlane.version`).

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: docker-clusterclass-v0.1.0
spec:
  ...
  patches:
  - name: customImage
    description: "Sets the container image that is used for running dockerMachines."
    definitions:
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: DockerMachineTemplate
        matchResources:
          machineDeploymentClass:
            names:
            - default-worker
      jsonPatches:
      - op: add
        path: /spec/template/spec/customImage
        valueFrom:
          template: |
            kindest/node:{{ .builtin.machineDeployment.version }}
```

### Complex variable types

Variables can also be objects and arrays. An object is specified with the type `object` and
by the schemas of the fields of the object. An array is specified via the type `array` and
the schema of the array items.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: docker-clusterclass-v0.1.0
spec:
  ...
  variables:
  - name: httpProxy
    schema:
      openAPIV3Schema:
        type: object
        properties: 
          # Schema of the url field.
          url: 
            type: string
          # Schema of the noProxy field.
          noProxy:
            type: string
  - name: dnsServers
    schema:
      openAPIV3Schema:
        type: array
        items:
          # Schema of the array items.
          type: string
```

Objects and arrays can be used in patches either directly by referencing the variable name,
or by accessing individual fields. For example:
```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: docker-clusterclass-v0.1.0
spec:
  ...
  jsonPatches:
  - op: add
    path: /spec/template/spec/httpProxy/url
    valueFrom:
      # Use the url field of the httpProxy variable.
      variable: httpProxy.url
  - op: add
    path: /spec/template/spec/dnsServers
    valueFrom:
      # Use the entire dnsServers array.
      variable: dnsServers
  - op: add
    path: /spec/template/spec/dnsServer
    valueFrom:
      # Use the first item of the dnsServers array.
      variable: dnsServers[0]
```

**Tips & Tricks**

Complex variables can be used to make references in templates configurable, e.g. the `identityRef` used in `AzureCluster`.
Of course it's also possible to only make the name of the reference configurable, including restricting the valid values 
to a pre-defined enum.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: azure-clusterclass-v0.1.0
spec:
  ...
  variables:
  - name: clusterIdentityRef
    schema:
      openAPIV3Schema:
        type: object
        properties:
          kind:
            type: string
          name:
            type: string
```

### Using variable values in JSON patches

We already saw above that it's possible to use variable values in JSON patches. It's also 
possible to calculate values via Go templating or to use hard-coded values.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: docker-clusterclass-v0.1.0
spec:
  ...
  patches:
  - name: etcdImageTag
    definitions:
    - selector:
      ...
      jsonPatches:
      - op: add
        path: /spec/template/spec/kubeadmConfigSpec/clusterConfiguration/etcd
        valueFrom:
          # This template is first rendered with Go templating, then parsed by 
          # a YAML/JSON parser and then used as value of the JSON patch.
          # For example, if the variable etcdImageTag is set to `3.5.1-0` the 
          # .../clusterConfiguration/etcd field will be set to:
          # {"local": {"imageTag": "3.5.1-0"}}
          template: |
            local:
              imageTag: {{ .etcdImageTag }}
  - name: imageRepository
    definitions:
    - selector:
      ...
      jsonPatches:
      - op: add
        path: /spec/template/spec/kubeadmConfigSpec/clusterConfiguration/imageRepository
        # This hard-coded value is used directly as value of the JSON patch.
        value: "my.custom.registry"
```

<aside class="note">

<h1>Variable paths</h1>

* Paths can be used in `.valueFrom.template` and `.valueFrom.variable` to access nested fields of arrays and objects.
* `.` is used to access a field of an object, e.g. `httpProxy.url`.
* `[i]` is used to access an array element, e.g. `dnsServers[0]`.
* Because of the way Go templates work, the paths in templates have to start with a dot.

</aside>

**Tips & Tricks**

Templates can be used to implement defaulting behavior during JSON patch value calculation. This can be used if the simple
constant default value which can be specified in the schema is not enough.
```yaml
        valueFrom:
          # If .vnetName is set, it is used. Otherwise, we will use `{{.builtin.cluster.name}}-vnet`.  
          template: "{{ if .vnetName }}{{.vnetName}}{{else}}{{.builtin.cluster.name}}-vnet{{end}}"
```
When writing templates, a subset of functions from [the sprig library](http://masterminds.github.io/sprig/) can be used to
write expressions like e.g. `{{ .name | upper }}`. Only functions that are guaranteed to evaluate to the same result
for a given input are allowed (e.g. `upper` or `max` can be used, while `now` or `randAlpha` can not be used).

### Optional patches

Patches can also be conditionally enabled. This can be done by configuring a Go template via `enabledIf`. 
The patch is then only applied if the Go template evaluates to `true`. In the following example the `httpProxy` 
patch is only applied if the `httpProxy` variable is set (and not empty).

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: docker-clusterclass-v0.1.0
spec:
  ...
  variables:
  - name: httpProxy
    schema:
      openAPIV3Schema:
        type: string
  patches:
  - name: httpProxy
    enabledIf: "{{ if .httpProxy }}true{{end}}"
    definitions:
    ...  
```

**Tips & Tricks**:

Hard-coded values can be used to test the impact of a patch during development, gradually roll out patches, etc. .
```yaml
    enabledIf: false
```

A boolean variable can be used to enable/disable a patch (or "feature"). This can have opt-in or opt-out behavior
depending on the default value of the variable.
```yaml
    enabledIf: "{{ .httpProxyEnabled }}"
```

Of course the same is possible by adding a boolean variable to a configuration object.
```yaml
    enabledIf: "{{ .httpProxy.enabled }}"
```

Builtin variables can be leveraged to apply a patch only for a specific Kubernetes version.
```yaml
    enabledIf: '{{ semverCompare "1.21.1" .builtin.controlPlane.version }}'
```

With `semverCompare` and `coalesce` a feature can be enabled in newer versions of Kubernetes for both KubeadmConfigTemplate and KubeadmControlPlane.
```yaml
    enabledIf: '{{ semverCompare "^1.22.0" (coalesce .builtin.controlPlane.version .builtin.machineDeployment.version )}}'
```

<!-- links -->
[Changing a ClusterClass]: ./change-clusterclass.md
[clusterctl alpha topology plan]: ../../../clusterctl/commands/alpha-topology-plan.md
