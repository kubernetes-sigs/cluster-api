# MicroK8s control plane provider
## What is the Cluster API MicroK8s control plane provider ?

Cluster API MicroK8s control plane provider (CACPM) is a component responsible for managing the control plane of the provisioned clusters. This implementation uses [MicroK8s](https://github.com/canonical/microk8s) for cluster provisioning and management.

Currently the CACPM does not expose any functionality. It serves however the following purposes:

  * Sets the ProviderID on the provisioned nodes. MicroK8s will not set the provider ID automatically so the control plane provider identifies the VMs' provider IDs and updates the respective machine objects.
  * Updates the machine state.
  * Generates and provisions the kubeconfig file used for accessing the cluster. The kubeconfig file is stored as a secret and the user can retrieve via `clusterctl`.
