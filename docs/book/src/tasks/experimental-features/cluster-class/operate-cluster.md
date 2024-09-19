# Operating a managed Cluster

The `spec.topology` field added to the Cluster object as part of ClusterClass allows changes made on the Cluster to be propagated across all relevant objects. This means the Cluster object can be used as a single point of control for making changes to objects that are part of the Cluster, including the ControlPlane and MachineDeployments. 

A managed Cluster can be used to:
* [Upgrade a Cluster](#upgrade-a-cluster)
* [Scale a ControlPlane](#scale-a-controlplane)
* [Scale a MachineDeployment](#scale-a-machinedeployment)
* [Add a MachineDeployment](#add-a-machinedeployment)
* [Use variables in a Cluster](#use-variables)
* [Rebase a Cluster to a different ClusterClass](#rebase-a-cluster)
* [Upgrading Cluster API](#upgrading-cluster-api)
* [Tips and tricks](#tips-and-tricks)

## Upgrade a Cluster
Using a managed topology the operation to upgrade a Kubernetes cluster is a one-touch operation.
Let's assume we have created a CAPD cluster with ClusterClass and specified Kubernetes v1.21.2 (as documented in the [Quick Start guide]). Specifying the version is done when running `clusterctl generate cluster`. Looking at the cluster, the version of the control plane and the MachineDeployments is v1.21.2.

```bash
> kubectl get kubeadmcontrolplane,machinedeployments
```
```bash
NAME                                                                              CLUSTER                   INITIALIZED   API SERVER AVAILABLE   REPLICAS   READY   UPDATED   UNAVAILABLE   AGE     VERSION
kubeadmcontrolplane.controlplane.cluster.x-k8s.io/clusterclass-quickstart-XXXX    clusterclass-quickstart   true          true                   1          1       1         0             2m21s   v1.21.2

NAME                                                                             CLUSTER                   REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE     AGE     VERSION
machinedeployment.cluster.x-k8s.io/clusterclass-quickstart-linux-workers-XXXX    clusterclass-quickstart   1          1       1         0             Running   2m21s   v1.21.2
```

To update the Cluster the only change needed is to the `version` field under `spec.topology` in the Cluster object.

Change `1.21.2` to `1.22.0` as below.

```bash
kubectl patch cluster clusterclass-quickstart --type json --patch '[{"op": "replace", "path": "/spec/topology/version", "value": "v1.22.0"}]'
```

The patch will make the following change to the Cluster yaml:
```diff 
   spec:
     topology:
      class: quick-start
+     version: v1.22.0
-     version: v1.21.2 
```

**Important Note**: A +2 minor Kubernetes version upgrade is not allowed in Cluster Topologies. This is to align with existing control plane providers, like KubeadmControlPlane provider, that limit a +2 minor version upgrade. Example: Upgrading from `1.21.2` to `1.23.0` is not allowed.

The upgrade will take some time to roll out as it will take place machine by machine with older versions of the machines only being removed after healthy newer versions come online.

To watch the update progress run:

```bash
watch kubectl get kubeadmcontrolplane,machinedeployments
```

After a few minutes the upgrade will be complete and the output will be similar to:

```bash
NAME                                                                              CLUSTER                   INITIALIZED   API SERVER AVAILABLE   REPLICAS   READY   UPDATED   UNAVAILABLE   AGE     VERSION
kubeadmcontrolplane.controlplane.cluster.x-k8s.io/clusterclass-quickstart-XXXX    clusterclass-quickstart   true          true                   1          1       1         0             7m29s   v1.22.0

NAME                                                                             CLUSTER                   REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE     AGE     VERSION
machinedeployment.cluster.x-k8s.io/clusterclass-quickstart-linux-workers-XXXX    clusterclass-quickstart   1          1       1         0             Running   7m29s   v1.22.0
```

## Scale a MachineDeployment
When using a managed topology scaling of MachineDeployments, both up and down, should be done through the Cluster topology.

Assume we have created a CAPD cluster with ClusterClass and Kubernetes v1.23.3 (as documented in the [Quick Start guide]). Initially we should have a MachineDeployment with 3 replicas. Running
```bash 
kubectl get machinedeployments
```
Will give us:
```bash
NAME                                                            CLUSTER           REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE     AGE   VERSION
machinedeployment.cluster.x-k8s.io/capi-quickstart-md-0-XXXX   capi-quickstart   3          3       3         0             Running   21m   v1.23.3
```
We can scale up or down this MachineDeployment through the Cluster object by changing the replicas field under `/spec/topology/workers/machineDeployments/0/replicas`
The `0` in the path refers to the position of the target MachineDeployment in the list of our Cluster topology. As we only have one MachineDeployment we're targeting the first item in the list under `/spec/topology/workers/machineDeployments/`.

To change this value with a patch:
```bash
kubectl  patch cluster capi-quickstart --type json --patch '[{"op": "replace", "path": "/spec/topology/workers/machineDeployments/0/replicas",  "value": 1}]'
```

This patch will make the following changes on the Cluster yaml:
```diff
   spec:
     topology:
       workers:
         machineDeployments:
         - class: default-worker
           name: md-0
           metadata: {}
+          replicas: 1
-          replicas: 3
```
After a minute the MachineDeployment will have scaled down to 1 replica:

```bash
NAME                         CLUSTER           REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE     AGE   VERSION
capi-quickstart-md-0-XXXXX  capi-quickstart   1          1       1         0             Running   25m   v1.23.3
```

As well as scaling a MachineDeployment, Cluster operators can edit the labels and annotations applied to a running MachineDeployment using the Cluster topology as a single point of control.

## Add a MachineDeployment
MachineDeployments in a managed Cluster are defined in the Cluster's topology. Cluster operators can add a MachineDeployment to a living Cluster by adding it to the `cluster.spec.topology.workers.machineDeployments` field.

Assume we have created a CAPD cluster with ClusterClass and Kubernetes v1.23.3 (as documented in the [Quick Start guide]). Initially we should have a single MachineDeployment with 3 replicas. Running
```bash 
kubectl get machinedeployments
```

Will give us:
```bash
NAME                                                            CLUSTER           REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE     AGE   VERSION
machinedeployment.cluster.x-k8s.io/capi-quickstart-md-0-XXXX   capi-quickstart   3          3       3         0             Running   21m   v1.23.3
```


A new MachineDeployment can be added to the Cluster by adding a new MachineDeployment spec under `/spec/topology/workers/machineDeployments/`. To do so we can patch our Cluster with:
```bash 
kubectl  patch cluster capi-quickstart --type json --patch '[{"op": "add", "path": "/spec/topology/workers/machineDeployments/-",  "value": {"name": "second-deployment", "replicas": 1, "class": "default-worker"} }]'
```
This patch will make the below changes on the Cluster yaml:
```diff
   spec:
     topology:
       workers:
         machineDeployments:
         - class: default-worker
           metadata: {}
           replicas: 3
           name: md-0
+        - class: default-worker
+          metadata: {}
+          replicas: 1
+          name: second-deployment
```

After a minute to scale the new MachineDeployment we get:
```bash
NAME                                      CLUSTER           REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE     AGE   VERSION
capi-quickstart-md-0-XXXX                 capi-quickstart   1          1       1         0             Running   39m   v1.23.3
capi-quickstart-second-deployment-XXXX    capi-quickstart   1          1       1         0             Running   99s   v1.23.3
```
Our second deployment uses the same underlying MachineDeployment class `default-worker` as our initial deployment. In this case they will both have exactly the same underlying machine templates. In order to modify the templates MachineDeployments are based on take a look at [Changing a ClusterClass].

A similar process as that described here - removing the MachineDeployment from `cluster.spec.topology.workers.machineDeployments` - can be used to delete a running MachineDeployment from an active Cluster.

## Scale a ControlPlane
When using a managed topology scaling of ControlPlane Machines, where the Cluster is using a topology that includes ControlPlane MachineInfrastructure, should be done through the Cluster topology.

This is done by changing the ControlPlane replicas field at `/spec/topology/controlPlane/replica` in the Cluster object. The command is:

```bash 
kubectl  patch cluster capi-quickstart --type json --patch '[{"op": "replace", "path": "/spec/topology/controlPlane/replicas",  "value": 1}]'
```

This patch will make the below changes on the Cluster yaml:
```diff
   spec:
      topology:
        controlPlane:
          metadata: {}
+         replicas: 1
-         replicas: 3
```

As well as scaling a ControlPlane, Cluster operators can edit the labels and annotations applied to a running ControlPlane using the Cluster topology as a single point of control.


## Use variables
A ClusterClass can use variables and patches in order to allow flexible customization of Clusters derived from a ClusterClass. Variable definition allows two or more Cluster topologies derived from the same ClusterClass to have different specs, with the differences controlled by variables in the Cluster topology.

Assume we have created a CAPD cluster with ClusterClass and Kubernetes v1.23.3 (as documented in the [Quick Start guide]). Our Cluster has a variable `etcdImageTag` as defined in the ClusterClass. The variable is not set on our Cluster. Some variables, depending on their definition in a ClusterClass, may need to be specified by the Cluster operator for every Cluster created using a given ClusterClass.

In order to specify the value of a variable all we have to do is set the value in the Cluster topology. 

We can see the current unset variable with:
```bash 
kubectl get cluster capi-quickstart -o jsonpath='{.spec.topology.variables[1]}'                                     
```
Which will return something like:
```bash
{"name":"etcdImageTag","value":""}
```

In order to run a different version of etcd in new ControlPlane machines - the part of the spec this variable sets - change the value using the below patch:
```bash 
kubectl  patch cluster capi-quickstart --type json --patch '[{"op": "replace", "path": "/spec/topology/variables/1/value",  "value": "3.5.0"}]'
```

Running the patch makes the following change to the Cluster yaml:
```diff
   spec:
     topology:
       variables:
       - name: imageRepository
         value: registry.k8s.io
       - name: etcdImageTag
         value: ""
       - name: coreDNSImageTag
+        value: "3.5.0"
-        value: ""

```
Retrieving the variable value from the Cluster object, with `kubectl get cluster capi-quickstart -o jsonpath='{.spec.topology.variables[1]}'` we can see:
```bash
{"name":"etcdImageTag","value":"3.5.0"}
```
Note: Changing the etcd version may have unintended impacts on a running Cluster. For safety the cluster should be reapplied after running the above variable patch.

## Rebase a Cluster
To perform more significant changes using a Cluster as a single point of control, it may be necessary to change the ClusterClass that the Cluster is based on. This is done by changing the class referenced in `/spec/topology/class`.

To read more about changing an underlying class please refer to [ClusterClass rebase].

## Tips and tricks

Users should always aim at ensuring the stability of the Cluster and of the applications hosted on it while
using `spec.topology` as a single point of control for making changes to the objects that are part of the Cluster.

Following recommendation apply:

- If possible, avoid concurrent changes to control-plane and/or MachineDeployments to prevent
  excessive turnover on the underlying infrastructure or bottlenecks in the Cluster trying to move workloads
  from one machine to the other.
- Keep machine labels and annotation stable, because changing those values requires machines rollouts;
  also, please note that machine labels and annotation are not propagated to Kubernetes nodes; see
  [metadata propagation](../../../reference/api/metadata-propagation.md).
- While upgrading a Cluster, if possible avoid any other concurrent change to the Cluster; please note
  that you can rely on [version-aware patches](write-clusterclass.md#version-aware-patches) to ensure
  the Cluster adapts to the new Kubernetes version in sync with the upgrade workflow.

For more details about how changes can affect a Cluster, please look at [reference](change-clusterclass.md#reference).

<aside class="note warning">

<h1>Effects of concurrent changes</h1>

When applying concurrent changes to a Cluster, the topology controller will immediately act in order to
reconcile to the desired state, and thus proxy all the required changes to the underlying objects which
in turn take action, and this might require rolling  out machines (create new, delete old).

As noted above, when executed at scale this might create excessive turnover on the underlying infrastructure
or bottlenecks in the Cluster trying to move workloads from one machine to the other.

Additionally, in case of change of the Kubernetes version and other concurrent changes for Machines deployments
this could lead to double rollout of the worker nodes:
- The first rollout triggered by the changes to the machine deployments immediately applied to the underlying objects
  (e.g change of labels). 
- The second rollout triggered by the upgrade workflow changing the MachineDeployment version only after the control 
  upgrade is completed (see [upgrade a cluster](#upgrade-a-cluster) above).

Please note that:
- Cluster API already implements strategies to ensure changes in a Cluster are executed in a safe way under
  most of the circumstances, including users occasionally not acting according to above best practices;
- The above-mentioned strategies are currently implemented on the abstraction controlling a single set of machines,
  the control-plane (KCP) or the MachineDeployment;
- In future Managed topologies could be improved by introducing strategies to ensure a higher safety across all
  abstraction controlling Machines in a Cluster, but this work is currently at its initial stage and user feedback
  could help in shaping out those improvements.
- Similarly, in future we might consider implementing strategies to controlling changes across many Clusters. 

</aside>

# Upgrading Cluster API

There are some special considerations for ClusterClass regarding Cluster API upgrades when the upgrade includes a bump
of the apiVersion of infrastructure, bootstrap or control plane provider CRDs.

The recommended approach is to first upgrade Cluster API and then update the apiVersions in the ClusterClass references afterwards.
By following above steps, there won't be any disruptions of the reconciliation as the Cluster topology controller is able to reconcile the Cluster 
even with the old apiVersions in the ClusterClass.

Note: The apiVersions in ClusterClass cannot be updated before Cluster API because the new apiVersions don't exist in 
the management cluster before the Cluster API upgrade.

In general the Cluster topology controller always uses exactly the versions of the CRDs referenced in the ClusterClass.
This means in the following example the Cluster topology controller will always use `v1beta1` when reconciling/applying 
patches for the infrastructure ref, even if the `DockerClusterTemplate` already has a `v1beta2` apiVersion.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: quick-start
  namespace: default
spec:
  infrastructure:
    ref:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
      kind: DockerClusterTemplate
...
```

<aside class="note warning">

<h1>Bumping apiVersions in ClusterClass</h1>

When upgrading the apiVersions in references in the ClusterClass the corresponding patches have to be changed accordingly.
This includes bumping the apiVersion in the patch selector and potentially updating the JSON patch to changes in the new 
apiVersion of the referenced CRD. The following example shows how to upgrade the ClusterClass in this case. 

ClusterClass with the old apiVersion:
```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: quick-start
spec:
  infrastructure:
    ref:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
      kind: DockerClusterTemplate
...
  patches:
  - name: lbImageRepository
    definitions:
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: DockerClusterTemplate
        matchResources:
          infrastructureCluster: true
      jsonPatches:
      - op: add
        path: "/spec/template/spec/loadBalancer/imageRepository"
        valueFrom:
          variable: lbImageRepository
```

ClusterClass with the new apiVersion:
```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
metadata:
  name: quick-start
spec:
  infrastructure:
    ref:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta2 # apiVersion updated
      kind: DockerClusterTemplate
...
  patches:
  - name: lbImageRepository
    definitions:
    - selector:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta2 # apiVersion updated
        kind: DockerClusterTemplate
        matchResources:
          infrastructureCluster: true
      jsonPatches:
      - op: add
        # Path has been updated, as in this example imageRepository has been renamed 
        # to imageRepo in v1beta2 of DockerClusterTemplate.
        path: "/spec/template/spec/loadBalancer/imageRepo"
        valueFrom:
          variable: lbImageRepository
```

If external patches are used in the ClusterClass, it has to be ensured that all external patches support the new apiVersion 
before bumping apiVersions.

</aside>

[Quick Start guide]: ../../../user/quick-start.md
[ClusterClass rebase]: ./change-clusterclass.md#rebase
[Changing a ClusterClass]: ./change-clusterclass.md
