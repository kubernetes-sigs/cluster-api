# clusterctl alpha topology plan

The `clusterctl alpha topology plan` command can be used to get a plan of how a Cluster topology evolves given
file(s) containing resources to be applied to a Cluster.

The input file(s) could contain a new/modified Cluster, a new/modified ClusterClass and/or new/modified templates, 
depending on the use case you are going to plan for (see more details below).

The topology plan output would provide details about objects that will be created, updated and deleted of a target cluster; 
If instead the command detects that the change impacts many Clusters, the users will be required to select one to focus on (see flags below).

```shell
clusterctl alpha topology plan -f input.yaml -o output/
```

<aside class="note">

<h1>Running without a management cluster</h1>

This command can be used with or without a management cluster. In case the command is used without a management cluster 
the input should have all the objects needed.

</aside>

## Example use cases
 
### Designing a new ClusterClass

When designing a new ClusterClass users might want to preview the Cluster generated using such ClusterClass. 
The `clusterctl alpha topology plan command` can be used to do so:

```shell
clusterctl alpha topology plan -f example-cluster-class.yaml -f example-cluster.yaml -o output/
```

`example-cluster-class.yaml` holds the definitions of the ClusterClass and all the associated templates. 
<details>
<summary>View <code>example-cluster-class.yaml</code></summary>

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: example-cluster-class
  namespace: default
spec:
  controlPlane:
    ref:
      apiVersion: controlplane.cluster.x-k8s.io/v1beta1
      kind: KubeadmControlPlaneTemplate
      name: example-cluster-control-plane
      namespace: default
    machineInfrastructure:
      ref:
        kind: DockerMachineTemplate
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        name: "example-cluster-control-plane"
        namespace: default
  infrastructure:
    ref:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
      kind: DockerClusterTemplate
      name: example-cluster
      namespace: default
  workers:
    machineDeployments:
    - class: "default-worker"
      template:
        bootstrap:
          ref:
            apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
            kind: KubeadmConfigTemplate
            name: example-docker-worker-bootstraptemplate
        infrastructure:
          ref:
            apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
            kind: DockerMachineTemplate
            name: example-docker-worker-machinetemplate
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: DockerClusterTemplate
metadata:
  name: example-cluster
  namespace: default
spec:
  template:
    spec: {}
---
kind: KubeadmControlPlaneTemplate
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
metadata:
  name: "example-cluster-control-plane"
  namespace: default
spec:
  template:
    spec:
      machineTemplate:
        nodeDrainTimeout: 1s
      kubeadmConfigSpec:
        clusterConfiguration:
          controllerManager:
            extraArgs: { enable-hostpath-provisioner: 'true' }
          apiServer:
            certSANs: [ localhost, 127.0.0.1 ]
        initConfiguration:
          nodeRegistration:
            criSocket: unix:///var/run/containerd/containerd.sock
            kubeletExtraArgs:
              cgroup-driver: cgroupfs
              eviction-hard: 'nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%'
        joinConfiguration:
          nodeRegistration:
            criSocket: unix:///var/run/containerd/containerd.sock
            kubeletExtraArgs:
              cgroup-driver: cgroupfs
              eviction-hard: 'nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%'
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: DockerMachineTemplate
metadata:
  name: "example-cluster-control-plane"
  namespace: default
spec:
  template:
    spec:
      extraMounts:
      - containerPath: "/var/run/docker.sock"
        hostPath: "/var/run/docker.sock"
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: DockerMachineTemplate
metadata:
  name: "example-docker-worker-machinetemplate"
  namespace: default
spec:
  template:
    spec: {}
---
apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
kind: KubeadmConfigTemplate
metadata:
  name: "example-docker-worker-bootstraptemplate"
  namespace: default
spec:
  template:
    spec:
      joinConfiguration:
        nodeRegistration:
          kubeletExtraArgs:
            cgroup-driver: cgroupfs
            eviction-hard: 'nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%'
```

</details>

`example-cluster.yaml` holds the definition of `example-cluster` Cluster.
<details>
<summary>View <code>example-cluster.yaml</code></summary>

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: "example-cluster"
  namespace: "default"
  labels:
    cni: kindnet
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.128.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
    serviceDomain: "cluster.local"
  topology:
    class: example-cluster-class
    version: v1.21.2
    controlPlane:
      metadata: {}
      replicas: 1
    workers:
      machineDeployments:
      - class: "default-worker"
        name: "md-0"
        replicas: 1
```

</details>

Produces an output similar to this:
```shell
The following ClusterClasses will be affected by the changes:
 ＊ default/example-cluster-class

The following Clusters will be affected by the changes:
 ＊ default/example-cluster

Changes for Cluster "default/example-cluster": 

  NAMESPACE  KIND                   NAME                                  ACTION    
  default    DockerCluster          example-cluster-rnx2q                 created   
  default    DockerMachineTemplate  example-cluster-control-plane-dfnvz   created   
  default    DockerMachineTemplate  example-cluster-md-0-infra-qz9qk      created   
  default    KubeadmConfigTemplate  example-cluster-md-0-bootstrap-m29vz  created   
  default    KubeadmControlPlane    example-cluster-b2lhc                 created   
  default    MachineDeployment      example-cluster-md-0-pqscg            created   
  default    Secret                 example-cluster-shim                  created   
  default    Cluster                example-cluster                       modified  

Created objects are written to directory "output/created"
Modified objects are written to directory "output/modified"
```

The contents of the output directory are similar to this:
```
output
├── created
│   ├── DockerCluster_default_example-cluster-rnx2q.yaml
│   ├── DockerMachineTemplate_default_example-cluster-control-plane-dfnvz.yaml
│   ├── DockerMachineTemplate_default_example-cluster-md-0-infra-qz9qk.yaml
│   ├── KubeadmConfigTemplate_default_example-cluster-md-0-bootstrap-m29vz.yaml
│   ├── KubeadmControlPlane_default_example-cluster-b2lhc.yaml
│   ├── MachineDeployment_default_example-cluster-md-0-pqscg.yaml
│   └── Secret_default_example-cluster-shim.yaml
└── modified
    ├── Cluster_default_example-cluster.diff
    ├── Cluster_default_example-cluster.jsonpatch
    ├── Cluster_default_example-cluster.modified.yaml
    └── Cluster_default_example-cluster.original.yaml
```

### Plan changes to Cluster topology

When making changes to a Cluster topology the `clusterctl alpha topology plan` can be used to analyse how the underlying objects will be affected.

```shell
clusterctl alpha topology plan -f modified-example-cluster.yaml -o output/
```

The `modified-example-cluster.yaml` scales up the control plane to 3 replicas and adds additional labels to the machine deployment.
<details>
<summary>View <code>modified-example-cluster.yaml</code></summary>

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: "example-cluster"
  namespace: default
  labels:
    cni: kindnet
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.128.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
    serviceDomain: "cluster.local"
  topology:
    class: example-cluster-class
    version: v1.21.2
    controlPlane:
      metadata: {}
      # Scale up the control plane from 1 -> 3.
      replicas: 3
    workers:
      machineDeployments:
      - class: "default-worker"
        # Apply additional labels.
        metadata: 
          labels:
            test-label: md-0-label
        name: "md-0"
        replicas: 1
```
</details>

Produces an output similar to this:
```shell
Detected a cluster with Cluster API installed. Will use it to fetch missing objects.
No ClusterClasses will be affected by the changes.
The following Clusters will be affected by the changes:
 ＊ default/example-cluster

Changes for Cluster "default/example-cluster": 

  NAMESPACE  KIND                 NAME                        ACTION    
  default    KubeadmControlPlane  example-cluster-l7kx8       modified  
  default    MachineDeployment    example-cluster-md-0-j58ln  modified  

Modified objects are written to directory "output/modified"
```

### Rebase a Cluster to a different ClusterClass

The command can be used to plan if a Cluster can be successfully rebased to a different ClusterClass.

Rebasing a Cluster to a different ClusterClass:
```shell
# Rebasing from `example-cluster-class` to `another-cluster-class`.
clusterctl alpha topology plan -f rebase-example-cluster.yaml -o output/
```
The `example-cluster` Cluster is rebased from `example-cluster-class` to `another-cluster-class`. In this example `another-cluster-class` is assumed to be available in the management cluster.

<details>
<summary>View <code>rebase-example-cluster.yaml</code></summary>

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: "example-cluster"
  namespace: "default"
  labels:
    cni: kindnet
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.128.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
    serviceDomain: "cluster.local"
  topology:
    # ClusterClass changed from 'example-cluster-class' -> 'another-cluster-class'.
    class: another-cluster-class
    version: v1.21.2
    controlPlane:
      metadata: {}
      replicas: 1
    workers:
      machineDeployments:
      - class: "default-worker"
        name: "md-0"
        replicas: 1
```
</details>

If the target ClusterClass is compatible with the original ClusterClass the output be similar to:
```shell
Detected a cluster with Cluster API installed. Will use it to fetch missing objects.
No ClusterClasses will be affected by the changes.
The following Clusters will be affected by the changes:
 ＊ default/example-cluster

Changes for Cluster "default/example-cluster": 

  NAMESPACE  KIND                   NAME                                  ACTION    
  default    DockerCluster          example-cluster-7t7pl                 modified  
  default    DockerMachineTemplate  example-cluster-control-plane-lt6kw   modified  
  default    DockerMachineTemplate  example-cluster-md-0-infra-cjxs4      modified  
  default    KubeadmConfigTemplate  example-cluster-md-0-bootstrap-m9sg8  modified  
  default    KubeadmControlPlane    example-cluster-l7kx8                 modified  

Modified objects are written to directory "output/modified"
```

Instead, if the command detects that the rebase operation would lead to a non-functional cluster (ClusterClasses are incompatible), the output will be similar to:
```shell
Detected a cluster with Cluster API installed. Will use it to fetch missing objects.
Error: failed defaulting and validation on input objects: failed to run defaulting and validation on Clusters: failed validation of cluster.x-k8s.io/v1beta1, Kind=Cluster default/example-cluster: Cluster.cluster.x-k8s.io "example-cluster" is invalid: spec.topology.workers.machineDeployments[0].class: Invalid value: "default-worker": MachineDeploymentClass with name "default-worker" does not exist in ClusterClass "another-cluster-class"
```
In this example rebasing will lead to a non-functional Cluster because the ClusterClass is missing a worker class that is used by the Cluster.

### Testing the effects of changing a ClusterClass

When planning for a change on a ClusterClass you might want to understand what effects the change will have on existing clusters.

```shell
clusterctl alpha topology plan -f modified-first-cluster-class.yaml -o output/
```
When multiple clusters are affected, only the list of Clusters and ClusterClasses is presented.
```shell
Detected a cluster with Cluster API installed. Will use it to fetch missing objects.
The following ClusterClasses will be affected by the changes:
 ＊ default/first-cluster-class

The following Clusters will be affected by the changes:
 ＊ default/first-cluster
 ＊ default/second-cluster

No target cluster identified. Use --cluster to specify a target cluster to get detailed changes.
```

To get the full list of changes for the "first-cluster":
```shell
clusterctl alpha topology plan -f modified-first-cluster-class.yaml -o output/ -c "first-cluster"
```
Output will be similar to the full summary output provided in other examples.

## How does `topology plan` work?

The topology plan operation is composed of the following steps:
* Set the namespace on objects in the input with missing namespace.
* Run the Defaulting and Validation webhooks on the Cluster and ClusterClass objects in the input.
* Dry run the topology reconciler on the target cluster.
* Capture all changes observed during reconciliation.

## Reference

### `--file`, `-f` (REQUIRED)
 
The input file(s) with the target changes. Supports multiple input files. 

The objects in the input should follow these rules:
* All the objects in the input should belong to the same namespace.
* Should not have multiple Clusters.
* Should not have multiple ClusterClasses.

<aside class="note warning">

<h1>Object namespaces</h1>

If some of the objects have a defined namespace and some do not, the objects are considered as belonging to different namespaces
which is not allowed.

</aside>

<aside class="note warning">

<h1>Defaulting and Validation</h1>

All templates in the inputs should be fully valid and have all the default values set. `topology plan` will not run any defaulting 
or validation on these objects. Defaulting and validation is only run on Cluster and ClusterClass objects.

</aside>

### `--output-directory`, `-o` (REQUIRED)

Information about the objects that are created and updated is written to this directory.

For objects that are modified the following files are written to disk:
* Original object
* Final object
* JSON patch between the original and the final objects
* Diff of the original and final objects

### `--cluster`, `-c` (Optional)

When multiple clusters are affected by the input, `--cluster` can be used to specify a target cluster. 

If only one cluster is affected or if a Cluster is in the input it defaults as the target cluster. 

### `--namespace`, `-n` (Optional)

Namespace used for objects with missing namespaces in the input.

If not provided, the namespace defined in kubeconfig is used. If a kubeconfig is not available the value `default` is used.

