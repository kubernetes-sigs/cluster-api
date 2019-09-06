# Cluster API v1alpha1 compared to v1alpha2

## Providers

### v1alpha1

Providers in v1alpha1 wrap behavior specific to an environment to create the infrastructure and bootstrap instances into
Kubernetes nodes. Examples of environments that have integrated with Cluster API v1alpha1 include, AWS, GCP, OpenStack,
Azure, vSphere and others. The provider vendors in Cluster API's controllers, registers its own actuators with the
Cluster API controllers and runs a single manager to complete a Cluster API management cluster.

### v1alpha2

v1alpha2 introduces two new providers and changes how the Cluster API is consumed. This means that in order to have a
complete management cluster that is ready to build clusters you will need three managers.

* Core (Cluster API)
* Bootstrap (kubeadm)
* Infrastructure (aws, gcp, azure, vsphere, etc)

Cluster API's controllers are no longer vendored by providers. Instead, Cluster API offers its own independent
controllers that are responsible for the core types:

* Cluster
* Machine
* MachineSet
* MachineDeployment

Bootstrap providers are an entirely new concept aimed at reducing the amount of kubeadm boilerplate that every provider
reimplemented in v1alpha1. The Bootstrap provider is responsible for running a controller that generates data necessary
to bootstrap an instance into a Kubernetes node (cloud-init, bash, etc).

v1alpha1 "providers" have become Infrastructure providers. The Infrastructure provider is responsible for generating
actual infrastructure (networking, load balancers, instances, etc) for a particular environment that can consume
bootstrap data to turn the infrastructure into a Kubernetes cluster.

## Actuators

### v1alpha1

Actuators are interfaces that the Cluster API controller calls. A provider pulls in the generic Cluster API controller
and then registers actuators to run specific infrastructure logic (calls to the provider cloud).

### v1alpha2

Actuators are not used in this version. Cluster API's controllers are no longer shared across providers and therefore
they do not need to know about the actuator interface. Instead, providers communicate to each other via Cluster API's
central objects, namely Machine and Cluster. When a user modifies a Machine with a reference, each provider will notice
that update and respond in some way. For example, the Bootstrap provider may attach BootstrapData to a BootstrapConfig
which will then be attached to the Machine object via Cluster API's controllers or the Infrastructure provider may
create a cloud instance for Kubernetes to run on.

## `clusterctl`

### v1alpha1

`clusterctl` was a command line tool packaged with v1alpha1 providers. The goal of this tool was to go from nothing to a
running management cluster in whatever environment the provider was built for. For example, Cluster-API-Provider-AWS
packaged a `clusterctl` that created a Kubernetes cluster in EC2 and installed the necessary controllers to respond to
Cluster API's APIs.

### v1alpha2

`clusterctl` is likely becoming provider-agnostic meaning one clusterctl is bundled with Cluster API and can be reused
across providers. Work here is still being figured out but providers will not be packaging their own `clusterctl`
anymore.
