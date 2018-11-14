# Cluster-API Provisioning Mechanism Consolidation Proposal

## Abstract

Cluster-API requires implementers to implement provisioning mechanisms (bootstrap scripts and logic for running those scripts) themselves. However, the mechanism, including bootstrap scripts, is same or very similar for all cloud providers. As this is a time consuming task it could be very useful to provide a reusable provisioning mechanism in the upstream Cluster-API.

## Proposal

The upstream Cluster-API doesn’t have any bootstrap scripts neither any wrapper around kubeadm for handling cluster operations (initialization and join). This means implementers have to manually write bootstrap scripts used by specific Cluster-API provider and manually handle cluster initialization and node joining. For most cloud providers both bootstrap scripts and steps for cluster initialization/joining are the same, except for provider specifics that could be provided via template variables.

The bootstrap scripts usually do the following tasks:

* Install dependencies required by Kubernetes
    * Includes dependencies such as socat, ebtables, which are same for all providers.
* Install container runtime
    * Steps for installing container runtime such as Docker or containerd are same for all providers.
* Install Kubernetes packages—kubeadm, kubelet, kubectl, kubernetes-cni
    * The required packages and steps to install them are same for all providers.
* Configure kubelet
    * There are several differences based on provider, however differences could be templated.
    * kubeadm configuration file can handle some configuration tasks for kubelet.
* Initialize cluster or join node to a cluster
    * Should be done using kubeadm configuration files, written by implementer or Cluster-API provider user.

Implementing the provisioning scripts in the upstream Cluster-API ensures:

* Cluster-API implementers can easily get started, as they don’t need to implement complex provisioning logic and scripts for many various setups and operating systems
* The bootstrap scripts are located in one place, which makes it easier to maintain, distribute and test them

### Example use case

Han wants to write a new Cluster-API provider for his employers cloud solution. Han knows the only difference is the cloud providers’ API, hence Han only wants to implement those API calls. Because the Cluster-API provides templates for the actual provisioning, Han can easily import and re-use them and safely assume they will work, as they are well tested.

## Goals

* Cluster-API provides re-usable provisioning scripts for bootstrapping a cluster for the most popular operating systems (Ubuntu, CentOS and CoreOS)
* Provisioning scripts are well tested

## Non-Goals

* Implement mechanism for executing the bootstrap scripts, usable in the all scenarios. This is not subject of this proposal, but a follow-up proposal will be written soon

## Implementation

### Scope of Implementation

The provisioning scripts are supposed to do actions described in the Proposal section: install dependencies, container runtime, Kubernetes, and configure Kubelet and Cloud Provider. Initializing Kubernetes cluster and joining nodes a cluster may or may not be part of those scripts.

Cluster initialization and node joining are going to be configured using Kubeadm configuration files, which provide a big set of options for configuring cluster and all relevant cluster components (such as kube-controller-manager, kube-proxy, etc.). Cluster-API users would write kubeadm configuration files themselves and pass them as a ConfigMap/Secret or pass them to the function for generating userdata.

Installing any other packages or manifests, including CNI, are not scope of those scripts, neither scope of Cluster-API. Cluster-API is not supposed to be a package manager, so everything needed should be installed as an add-on.

### Implementation Prerequisites

1. clusterctl should be able to install CNI. This was discussed on Cluster-API Breakout meeting held on 10/10. [Relevant issue #534](https://github.com/kubernetes-sigs/cluster-api/issues/534)

### Implementation details

The implementation will be based on [Go Templates](https://golang.org/pkg/text/template/). That allows implementers to source environment variables, pass data such as IP addresses, host names, and kubeadm tokens, configure Kubelet and Cloud Provider, without any change to the Cluster-API source code.

Similar approach is used by many projects and providers, including:

* [Cluster-API provider for DigitalOcean](https://github.com/kubermatic/cluster-api-provider-digitalocean/blob/master/cloud/digitalocean/actuators/machine/userdata.go)
* [Cluster-API provider for GCP](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/master/pkg/cloud/google/metadata.go)
* [Cluster-API provider for AWS](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/4553a80b6337b4adcc378c07db943772d30fbc78/pkg/cloud/aws/services/ec2/bastion.go)
* [Cluster-API provider for VSphere](https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/blob/master/cloud/vsphere/provisioner/common/templates.go)

The implementation will be realized using interfaces, meaning that for each supported operating system interface will be implemented. The interface has two functions, for getting user data for Master instances and for workers.

Both functions should take the following arguments:

* Environment variables, provided as a string map and then parsed and appended on the top of script.
* Kubernetes version, including Kubelet, kubeadm, kubernetes-cni versions.
* Kubelet and Cloud Provider configuration files
* To be discussed how we want to provide those:
* Kubeadm configuration file. Do we want to be able to pass it clusterctl, as a Secret/ConfigMap, or as a function parameter?

```go
type Userdata interface {
    MasterUserData(envVars map[string]string, kubeVersion string, cniVersion string) (string, error)
    NodeUserData(envVars map[string]string, kubeVersion, cniVersion string) (string, error)
}
```

### Decisions to be made

* What Kubernetes versions should be supported?
    * Kubernetes supports 3 version at the same time.
    * It should be possible to provide scripts for all 3 versions. Usually there are no changes in the bootstrap scripts between older and newer versions.
* What operating systems should be supported? The following three are supported by most of cloud providers:
    * Ubuntu 16.04 and Ubuntu 18.04. Very popular and very widely used
    * Container Linux
    * CentOS 7
* What container engine should be set up by the scripts?
    * Docker - very popular and very widely used in production.
    * containerd - newer solution, seems to work well. Used by AWS provider.
    * Opinion: we should choose one and stay with it.

## Alternatives Considered

* Custom images
    * Positive sides:
        * Cluster-API implementers can build images and redistribute them to  the end users.
        * Images have all components preinstalled and preconfigured. Usually no further interaction is needed from the user side.
        * It is possible to use various frameworks and utilities for building, versioning and testing images.
        * This approach is used by Cluster-API provider for AWS.
    * Negative sides:
        * Heavily depends on the provider’s ability to host and use custom images. This feature is still not available or not fully implemented for many providers.
        * Building and releasing images is time consuming. If you use images built by implementer you must fully trust the image.
        * It is not possible to modify an image without rebuilding and hosting it yourself.
        * Using Custom images can introduce additional costs depending on the cloud provider.
        * The build and publish process is unique for each cloud provider
