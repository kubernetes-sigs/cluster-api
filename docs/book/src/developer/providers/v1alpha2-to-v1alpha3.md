# Cluster API v1alpha2 compared to v1alpha3

## Minimum Go version

- The Go version used by Cluster API is now Go 1.13+

## In-Tree bootstrap provider

- Cluster API now ships with the Kubeadm Bootstrap provider (CABPK).
- Update import paths from `sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm` to `sigs.k8s.io/cluster-api/bootstrap/kubeadm`.

## Machine `spec.metadata` field has been removed

- The field has been unused for quite some time and didn't have any function.
- If you have been using this field to setup MachineSet or MachineDeployment, switch to MachineTemplate's metadata instead.

## Set `spec.clusterName` on Machine, MachineSet, MachineDeployments

- The field is now required on all Cluster dependant objects.
- The `cluster.x-k8s.io/cluster-name` label is created automatically by each respective controller.

## Context is now required for `external.CloneTemplate` function.

- Pass a context as the first argument to calls to `external.CloneTemplate`.

## Context is now required for `external.Get` function.

- Pass a context as the first argument to calls to `external.Get`.

## Cluster and Machine `Status.Phase` field values now start with an uppercase letter

- To be consistent with Pod phases in k/k.
- More details in https://github.com/kubernetes-sigs/cluster-api/pull/1532/files.

## `MachineClusterLabelName` is renamed to `ClusterLabelName`

- The variable name is renamed as this label isn't applied only to machines anymore.
- This label is also applied to external objects(bootstrap provider, infrastructure provider)

## Cluster and Machine controllers now set `cluster.x-k8s.io/cluster-name` to external objects.

- In addition to the OwnerReference back to the Cluster, a label is now added as well to any external objects, for example objects such as KubeadmConfig (bootstrap provider), AWSCluster (infrastructure provider), AWSMachine (infrastructure provider), etc.

## The `util/restmapper` package has been removed

- Controller runtime has native support for a [DynamicRESTMapper](https://github.com/kubernetes-sigs/controller-runtime/pull/554/files), which is used by default when creating a new Manager.

## Generated kubeconfig admin username changed from `kubernetes-admin` to `<cluster-name>-admin`

- The kubeconfig secret shipped with Cluster API now uses the cluster name as prefix to the `username` field.

## Changes to `sigs.k8s.io/cluster-api/controllers/remote`

-  The `ClusterClient` interface has been removed.
- `remote.NewClusterClient` now returns a `sigs.k8s.io/controller-runtime/pkg/client` Client. The signature changed from

    `func NewClusterClient(c client.Client, cluster *clusterv1.Cluster) (ClusterClient, error)`

    to

    `func NewClusterClient(c client.Client, cluster *clusterv1.Cluster, scheme runtime.Scheme) (client.Client, error)`

## A Machine is now considered a control plane if it has `cluster.x-k8s.io/control-plane` set, regardless of value

- Previously examples and tests were setting/checking for the label to be set to `true`.
- The function `util.IsControlPlaneMachine` was previously checking for any value other than empty string, while now we only check if the associated label exists.

## Machine `Status.Phase` field set to `Provisioned` if a NodeRef is set but infrastructure is not ready

 - The machine Status.Phase is set back to `Provisioned` if the infrastructure is not ready. This is only applicable if the infrastructure node status does not have any errors set.

## Metrics

- The cluster and machine controllers expose the following prometheus metrics.
  - `capi_cluster_control_plane_ready`: Cluster control plane is ready if set to 1 and not if 0.
  - `capi_cluster_infrastructure_ready`: Cluster infrastructure is ready if set to 1 and not if 0.
  - `capi_cluster_kubeconfig_ready`: Cluster kubeconfig is ready if set to 1 and not if 0.
  - `capi_cluster_failure_set`: Cluster FailureMesssage or FailureReason is set if metric is 1.
  - `capi_machine_bootstrap_ready`: Machine Boostrap is ready if set to 1 and not if 0.
  - `capi_machine_infrastructure_ready`: Machine InfrastructureRef is ready if set to 1 and not if 0.
  - `capi_machine_node_ready`: Machine NodeRef is ready if set to 1 and not if 0.

  They can be accessed by default via the `8080` metrics port on the cluster
  api controller manager.

## Cluster `Status.Phase` transition to `Provisioned` additionally needs at least one APIEndpoint to be available.

- Previously, the sole requirement to transition a Cluster's `Status.Phase` to `Provisioned` was a `true` value of `Status.InfrastructureReady`. Now, there are two requirements: a `true` value of `Status.InfrastructureReady` and at least one entry in `Status.APIEndpoints`.
- See https://github.com/kubernetes-sigs/cluster-api/pull/1721/files.

## `Status.ErrorReason` and `Status.ErrorMessage` fields, populated to signal a fatal error has occurred, have been renamed in Cluster, Machine and MachineSet

-  `Status.ErrorReason` has been renamed to `Status.FailureReason`
-  `Status.ErrorMessage` has been renamed to `Status.FailureMessage`

## The `external.ErrorsFrom` function has been renamed to `external.FailuresFrom`

- The function has been modified to reflect the rename of `Status.ErrorReason` to `Status.FailureReason` and `Status.ErrorMessage` to `Status.FailureMessage`.

## External objects will need to rename `Status.ErrorReason` and `Status.ErrorMessage`

- As a follow up to the changes mentioned above - for the `external.FailuresFrom` function to retain its functionality, external objects
(e.g., AWSCluster, AWSMachine, etc.) will need to rename the fields as well.
-  `Status.ErrorReason` should be renamed to `Status.FailureReason`
-  `Status.ErrorMessage` should be renamed to `Status.FailureMessage`

## The field `Cluster.Status.APIEndpoints` is removed in favor of `Cluster.Spec.ControlPlaneEndpoint`

- The slice in Cluster.Status has been removed and replaced by a single APIEndpoint field under Spec.
- Infrastructure providers MUST expose a ControlPlaneEndpoint field in their cluster infrastructure resource at `Spec.ControlPlaneEndpoint`. They may optionally remove the `Status.APIEndpoints` field (Cluster API no longer uses it).

## Data generated from a bootstrap provider is now stored in a secret.

- The Cluster API Machine Controller no longer reconciles the bootstrap provider `status.bootstrapData` field, but instead looks at `status.dataSecretName`.
- The `Machine.Spec.Bootstrap.Data` field is deprecated and will be removed in a future version.
- Bootstrap providers must create a Secret in the bootstrap resource's namespace and store the name in the bootstrap resource's `status.dataSecretName` field.
    - On reconciliation, we suggest to migrate from the deprecated field to a secret reference.
- Infrastructure providers must look for the bootstrap data secret name in `Machine.Spec.Bootstrap.DataSecretName` and fallback to `Machine.Spec.Bootstrap.Data`.

## The `cloudinit` module under the Kubeadm bootstrap provider has been made private

The `cloudinit` module has been moved to an `internal` directory as it is not designed to be a public interface consumed
outside of the existing module.

## Interface for Bootstrap Provider Consumers

- Consumers of bootstrap configuration, Machine and eventually MachinePool, must adhere to a
  contract that defines a set of required fields used for coordination with the kubeadm bootstrap
  provider.
  - `apiVersion` to check for supported version/kind.
  - `kind` to check for supported version/kind.
  - `metadata.labels["cluster.x-k8s.io/control-plane"]` only present in the case of a control plane
    `Machine`.
  - `spec.clusterName` to retrieve the owning Cluster status.
  - `spec.bootstrap.dataSecretName` to know where to put bootstrap data with sensitive information.
  - `status.infrastuctureReady` to understand the state of the configuration consumer so the
    bootstrap provider can take appropriate action (e.g. renew bootstrap token).

## Support the `cluster.x-k8s.io/paused` annotation and `Cluster.Spec.Paused` field.

- A new annotation `cluster.x-k8s.io/paused` provides the ability to pause reconciliation on specific objects.
- A new field `Cluster.Spec.Paused` provides the ability to pause reconciliation on a Cluster and all associated objects.
- A helper function `util.IsPaused` can be used on any Kubernetes object associated with a Cluster.

## Optional support for failure domains.

An infrastructure provider may or may not implement the failure domains feature. Failure domains gives Cluster API
just enough information to spread machines out reducing the risk of a target cluster failing due to a domain outage.
This is particularly useful for Control Plane providers. They are now able to put control plane nodes in different
domains.

An infrastructure provider can implement this by setting the `InfraCluster.Spec.FailureDomains` field with a map of
unique keys to `failureDomainSpec`s as well as respecting a set `Machine.Spec.FailureDomain` field when creating
instances.

Please see the cluster and machine infrastructure provider specifications for more detail.
