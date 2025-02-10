# Cluster API Provider In Memory backend

**NOTE:** The In memory backend is **not** designed for production use and is intended for development environments only.

This backend is an implementation of an infrastructure provider for the Cluster API project using in memory, fake objects.

The in memory backend is designed to test CAPI workflows using as few resources as possible on a single developer workstation; if 
the machine is powerful enough, it can be used to test scaling CAPI with thousands of clusters and machines.

It is also important to notice that the in memory backend doesn't provision a working Kubernetes cluster; 
you will get a minimal, fake K8s cluster capable to support only the interactions required by Cluster API, thus tricking
CAPI controllers in believing a real K8s cluster is there. But in memory K8s cluster cannot run any K8s workload.

## How Cluster API Provider In Memory backend differs from docker backend

While the in memory backend does not provision real Kubernetes clusters, the docker backend does, and in fact it can actually
provision a fully conformant K8s cluster.

However, with the docker backend you can create only clusters with few machines on a single developer workstation, in the order of tens, 
while instead with the in memory backend you can scale up to thousands.

## Architecture

The in memory backend does not reach out to any cloud provider / infrastructure provider; everything is created in memory.

The picture below explains how this works in detail:

![Architecture](architecture.drawio.svg)

- When you create a `DevCluster` with the in memory backend, it creates a VIP (or control plane endpoint) for your
  cluster. Such VIP is implemented as a listener on a port in the range 20000-24000 on the CAPD Pod.
- When the cluster API controllers (KCP, MD controllers) create an `DevMachine` with the in memory backend, it creates
  an "in-memory cloud machine". This cloud machine doesn't have any hardware infrastructure, it is just a record that a
  fake machine exists.

Given that there won't be cloud-init running on the machine, it is responsibility of the in memory backend to 
"mimic" the machine bootstrap process:
- If the machine is a control plane machine, fake static pods are created for API server, etcd and other control plane
  components (note: controller manager and scheduler are omitted from the picture for sake of simplicity).
- API server fake pods are registered as "backend" for the cluster VIP, as well as etcd fake pods are registered as
  "members" of a fake etcd cluster.

Cluster VIP, fake API server pods and fake Etcd pods provide a minimal fake Kubernetes control plane, just capable of the
operations required to trick Cluster API in believing a real K8s cluster is there.

More specifically the fake API server allows the in memory backend to complete the machine bootstrap process by: 
- Creating a fake Kubernetes Node, simulating Kubelet starting on the machine
- Assigning a fake provider ID to the Node, simulating a CPI performing this task
- Creating a fake kubeadm-config map, fake Core DNS deployment etc. simulating kubeadm init / join completion

The fake API server also serves requests from the Cluster API controllers checking the state of workload cluster:
- Get Nodes status
- Get control plane Pods status
- Get etcd member status (via port-forward)

## Working with the in memory backend

### Tilt

CAPD with the in memory backend can be used with Tilt for local development.

See [Developing Cluster API with Tilt](https://cluster-api.sigs.k8s.io/developer/tilt) for more details.

#### Accessing the workload cluster

Even if the workload cluster running in memory machines is fake, it could be interesting to use kubectl to
query the fake API server.

It is required to setup connectivity to the pod where the fake API server in running first;
this connection must be used when using kubectl.

From terminal window #1:

```shell
# NOTE: use namespace and name of the cluster you want to connect to
export NAMESPACE=default 
export CLUSTER_NAME=in-memory-development-11461

export CONTROL_PLANE_ENDPOINT_PORT=$(k get cluster -n $NAMESPACE $CLUSTER_NAME  -o=jsonpath='{.spec.controlPlaneEndpoint.port}')

kubectl port-forward -n capim-system deployments/capim-controller-manager $CONTROL_PLANE_ENDPOINT_PORT
```

From terminal window #2:

```shell
# NOTE: use namespace and name of the cluster you want to connect to
export NAMESPACE=default 
export CLUSTER_NAME=in-memory-development-11461

export CONTROL_PLANE_ENDPOINT_PORT=$(k get cluster -n $NAMESPACE $CLUSTER_NAME  -o=jsonpath='{.spec.controlPlaneEndpoint.port}')

bin/clusterctl get kubeconfig -n $NAMESPACE $CLUSTER_NAME > /tmp/kubeconfig
kubectl --kubeconfig=/tmp/kubeconfig --server=https://127.0.0.1:$CONTROL_PLANE_ENDPOINT_PORT get nodes
```

### E2E tests

CAPD with the in memory backend can be used to run a subset of CAPI E2E tests, but as of today we maintain only a smoke E2E scale test 
(10 clusters, 1 CP and 3 workers each) that can be executed by setting `GINKGO_LABEL_FILTER="in-memory"`.

See [Running the end-to-end tests locally](https://cluster-api.sigs.k8s.io/developer/testing#running-the-end-to-end-tests-locally) for more details.

### Clusterctl

You can look at the [CAPD quick-start](https://cluster-api.sigs.k8s.io/user/quick-start) as a reference about how to use CAPD with clusterctl.

See also [clusterctl for Developers](https://cluster-api.sigs.k8s.io/clusterctl/developers) for more details.
