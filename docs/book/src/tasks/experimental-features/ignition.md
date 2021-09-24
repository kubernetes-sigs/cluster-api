# Experimental Feature: Ignition Bootstrap Config (alpha)

The default configuration engine for bootstrapping workload cluster machines is [cloud-init](https://cloudinit.readthedocs.io/). **Ignition** is an alternative engine used by Linux distributions such as [Flatcar Container Linux](https://www.flatcar-linux.org/docs/latest/provisioning/ignition/) and [Fedora CoreOS](https://docs.fedoraproject.org/en-US/fedora-coreos/producing-ign/) and therefore should be used when choosing an Ignition-based distribution as the underlying OS for workload clusters.

<aside class="note warning">

<h1>Note</h1>

This initial implementation uses Ignition **v2** and was tested with **Flatcar Container Linux** only. Future releases are expected to add Ignition **v3** support and cover more Linux distributions.

</aside>

This guide explains how to deploy an AWS workload cluster using Ignition.

## Prerequisites

- [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl) installed locally
- [clusterawsadm](https://cluster-api-aws.sigs.k8s.io/introduction.html#clusterawsadm) installed locally - download from the [releases](https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases) page of the AWS provider
- [Kind](https://kind.sigs.k8s.io/) and [Docker](https://www.docker.com/) installed locally (when using Kind to create a management cluster)

## Configure a management cluster

Follow [this](../../user/quick-start.md#install-andor-configure-a-kubernetes-cluster) section of the quick start guide to deploy a Kubernetes cluster or connect to an existing one.

Follow [this](../../user/quick-start.md#install-clusterctl) section of the quick start guide to install `clusterctl`.

## Initialize the management cluster

Before workload clusters can be deployed, Cluster API components must be deployed to the management cluster.

Initialize the management cluster:

```bash
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

# Workload clusters need to call the AWS API as part of their normal operation.
# The following command creates a CloudFormation stack which provisions the
# necessary IAM resources to be used by workload clusters.
clusterawsadm bootstrap iam create-cloudformation-stack

# The management cluster needs to call the AWS API in order to manage cloud
# resources for workload clusters. The following command tells clusterctl to
# store the AWS credentials provided before in a Kubernetes secret where they
# can be retrieved by the AWS provider running on the management cluster.
export AWS_B64ENCODED_CREDENTIALS=$(clusterawsadm bootstrap credentials encode-as-profile)

# Enable the feature gates controlling Ignition bootstrap.
export EXP_KUBEADM_BOOTSTRAP_FORMAT_IGNITION=true # Used by the kubeadm bootstrap provider
export BOOTSTRAP_FORMAT_IGNITION=true # Used by the AWS provider

# Initialize the management cluster.
clusterctl init --infrastructure aws
```

## Generate a workload cluster configuration

```bash
# Deploy the workload cluster in the following AWS region.
export AWS_REGION=us-east-1

# Authorize the following SSH public key on cluster nodes.
export AWS_SSH_KEY_NAME=my-key

# Ignition bootstrap data needs to be stored in an S3 bucket so that nodes can
# read them at boot time. Store Ignition bootstrap data in the following bucket.
export AWS_S3_BUCKET_NAME=my-bucket

# Set the EC2 machine size for controllers and workers.
export AWS_CONTROL_PLANE_MACHINE_TYPE=t3a.small
export AWS_NODE_MACHINE_TYPE=t3a.small

# TODO: Update --from URL once https://github.com/kubernetes-sigs/cluster-api-provider-aws/pull/2271 is merged.
clusterctl generate cluster ignition-cluster \
    --from https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/e7c89c9add92a4b233b26a1712518d9616d99e7a/templates/cluster-template-flatcar.yaml \
    --kubernetes-version v1.22.2 \
    --worker-machine-count 2 \
    > ignition-cluster.yaml
```

## Apply the workload cluster

```bash
kubectl apply -f ignition-cluster.yaml
```

Wait for the control plane of the workload cluster to become initialized:

```bash
kubectl get kubeadmcontrolplane ignition-cluster-control-plane
```

This could take a while. When the control plane is initialized, the `INITIALIZED` field should be `true`:

```
NAME                             CLUSTER            INITIALIZED   API SERVER AVAILABLE   REPLICAS   READY   UPDATED   UNAVAILABLE   AGE    VERSION
ignition-cluster-control-plane   ignition-cluster   true                                 1                  1         1             7m7s   v1.22.2
```

## Connect to the workload cluster

Generate a kubeconfig for the workload cluster:

```bash
clusterctl get kubeconfig ignition-cluster > ./kubeconfig
```

Set `kubectl` to use the generated kubeconfig:

```bash
export KUBECONFIG=$(pwd)/kubeconfig
```

Verify connectivity with the workload cluster's API server:

```bash
kubectl cluster-info
```

Sample output:

```
Kubernetes control plane is running at https://ignition-cluster-apiserver-284992524.us-east-1.elb.amazonaws.com:6443
CoreDNS is running at https://ignition-cluster-apiserver-284992524.us-east-1.elb.amazonaws.com:6443/api/v1/namespaces/kube-system/services/kube-dns:dns/proxy

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```

## Deploy a CNI plugin

A [CNI plugin](https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/network-plugins/) must be deployed to the workload cluster for the cluster to become ready. We use [Calico](https://www.tigera.io/project-calico/) here, however other CNI plugins could be used, too.

```bash
kubectl apply -f https://docs.projectcalico.org/v3.20/manifests/calico.yaml
```

Ensure all cluster nodes become ready:

```bash
kubectl get nodes
```

Sample output:

```
NAME                                            STATUS   ROLES                  AGE   VERSION
ip-10-0-122-154.us-east-1.compute.internal   Ready    control-plane,master   14m   v1.22.2
ip-10-0-127-59.us-east-1.compute.internal    Ready    <none>                 13m   v1.22.2
ip-10-0-89-169.us-east-1.compute.internal    Ready    <none>                 13m   v1.22.2
```

## Clean up

Delete the workload cluster (from a shell connected to the *management* cluster):

```bash
kubectl delete cluster ignition-cluster
```

## Caveats

### Supported infrastructure providers

Cluster API has multiple [infrastructure providers](../../user/concepts.md#infrastructure-provider) which can be used to deploy workload clusters.

The following infrastructure providers already have Ignition support:

- [AWS](https://cluster-api-aws.sigs.k8s.io/)

Ignition support will be added to more providers in the future.
