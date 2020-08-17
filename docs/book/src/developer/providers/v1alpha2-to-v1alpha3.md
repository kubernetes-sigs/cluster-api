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
- `remote.NewClusterClient` now returns a `sigs.k8s.io/controller-runtime/pkg/client` Client. It also requires `client.ObjectKey` instead of a cluster reference. The signature changed:
  - From: `func NewClusterClient(c client.Client, cluster *clusterv1.Cluster) (ClusterClient, error)`
  - To: `func NewClusterClient(c client.Client, cluster client.ObjectKey, scheme runtime.Scheme) (client.Client, error)`
- Same for the remote client `ClusterClientGetter` interface:
  - From: `type ClusterClientGetter func(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, scheme *runtime.Scheme) (client.Client, error)`
  - To: `type ClusterClientGetter func(ctx context.Context, c client.Client, cluster client.ObjectKey, scheme *runtime.Scheme) (client.Client, error)`
- `remote.RESTConfig` now uses `client.ObjectKey` instead of a cluster reference. Signature change:
  - From: `func RESTConfig(ctx context.Context, c client.Client, cluster *clusterv1.Cluster) (*restclient.Config, error)`
  - To: `func RESTConfig(ctx context.Context, c client.Client, cluster client.ObjectKey) (*restclient.Config, error)`

### Related changes to `sigs.k8s.io/cluster-api/util`

- A helper function `util.ObjectKey` could be used to get `client.ObjectKey` for a Cluster, Machine etc.
- The returned client is no longer configured for lazy discovery. Any consumers that attempt to create
  a client prior to the server being available will now see an error.
- Getter for a kubeconfig secret, associated with a cluster requires `client.ObjectKey` instead of a cluster reference. Signature change:
  - From: `func Get(ctx context.Context, c client.Client, cluster client.ObjectKey, purpose Purpose) (*corev1.Secret, error)`
  - To: `func Get(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, purpose Purpose) (*corev1.Secret, error)`

## A Machine is now considered a control plane if it has `cluster.x-k8s.io/control-plane` set, regardless of value

- Previously examples and tests were setting/checking for the label to be set to `true`.
- The function `util.IsControlPlaneMachine` was previously checking for any value other than empty string, while now we only check if the associated label exists.

## Machine `Status.Phase` field set to `Provisioned` if a NodeRef is set but infrastructure is not ready

 - The machine Status.Phase is set back to `Provisioned` if the infrastructure is not ready. This is only applicable if the infrastructure node status does not have any errors set.

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
    - The secret created by the bootstrap provider is of type `cluster.x-k8s.io/secret`.
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
    Consumers must also verify the secret type matches `cluster.x-k8s.io/secret`.
  - `status.infrastuctureReady` to understand the state of the configuration consumer so the
    bootstrap provider can take appropriate action (e.g. renew bootstrap token).

## Support the `cluster.x-k8s.io/paused` annotation and `Cluster.Spec.Paused` field.

- A new annotation `cluster.x-k8s.io/paused` provides the ability to pause reconciliation on specific objects.
- A new field `Cluster.Spec.Paused` provides the ability to pause reconciliation on a Cluster and all associated objects.
- A helper function `util.IsPaused` can be used on any Kubernetes object associated with a Cluster and can be used during a Reconcile loop:
  ```go
  // Return early if the object or Cluster is paused.
  if util.IsPaused(cluster, <object>) {
    logger.Info("Reconciliation is paused for this object")
    return ctrl.Result{}, nil
  }
  ```
- Unless your controller is already watching Clusters, add a Watch to get notifications when Cluster.Spec.Paused field changes.
  In most cases, `predicates.ClusterUnpaused` and `util.ClusterToObjectsMapper` can be used like in the example below:
  ```go
  // Add a watch on clusterv1.Cluster object for paused notifications.
  clusterToObjectFunc, err := util.ClusterToObjectsMapper(mgr.GetClient(), <List object here>, mgr.GetScheme())
  if err != nil {
    return err
  }
  err = controller.Watch(
      &source.Kind{Type: &cluserv1.Cluster{}},
      &handler.EnqueueRequestsFromMapFunc{
          ToRequests: clusterToObjectFunc,
      },
      predicates.ClusterUnpaused(r.Log),
  )
  if err != nil {
    return err
  }
  ```
  NB: You need to have `cluster.x-k8s.io/cluster-name` applied to all your objects for the mapper to function.
- In some cases, you'll want to not just watch on Cluster.Spec.Paused changes, but also on
  Cluster.Status.InfrastructureReady. For those cases `predicates.ClusterUnpausedAndInfrastructureReady` should be used
  instead.
  ```go
  // Add a watch on clusterv1.Cluster object for paused and infrastructureReady notifications.
  clusterToObjectFunc, err := util.ClusterToObjectsMapper(mgr.GetClient(), <List object here>, mgr.GetScheme())
  if err != nil {
    return err
  }
  err = controller.Watch(
        &source.Kind{Type: &cluserv1.Cluster{}},
        &handler.EnqueueRequestsFromMapFunc{
            ToRequests: clusterToObjectFunc,
        },
        predicates.ClusterUnpausedAndInfrastructureReady(r.Log),
    )
    if err != nil {
      return err
    }
    ```

## [OPTIONAL] Support failure domains.

An infrastructure provider may or may not implement the failure domains feature. Failure domains gives Cluster API
just enough information to spread machines out reducing the risk of a target cluster failing due to a domain outage.
This is particularly useful for Control Plane providers. They are now able to put control plane nodes in different
domains.

An infrastructure provider can implement this by setting the `InfraCluster.Status.FailureDomains` field with a map of
unique keys to `failureDomainSpec`s as well as respecting a set `Machine.Spec.FailureDomain` field when creating
instances.

To support migration from failure domains that were previously specified through provider-specific resources, the
Machine controller will support updating `Machine.Spec.FailureDomain` field if `Spec.FailureDomain` is present and
defined on the provider-defined infrastructure resource.

Please see the cluster and machine infrastructure provider specifications for more detail.

## Refactor kustomize `config/` folder to support multi-tenancy when using webhooks.

> Pre-Requisites: Upgrade to CRD v1.

More details and background can be found in [Issue #2275](https://github.com/kubernetes-sigs/cluster-api/issues/2275) and [PR #2279](https://github.com/kubernetes-sigs/cluster-api/pull/2279).

Goals:
- Have all webhook related components in the `capi-webhook-system` namespace.
  - Achieves multi-tenancy and guarantees that both CRD and webhook resources can live globally and can be patched in future iterations.
- Run a new manager instance that ONLY runs webhooks and doesn't install any reconcilers.

Steps:
- In `config/certmanager/`
  - **Patch**
    - **certificate.yaml**: The `secretName` value MUST be set to `$(SERVICE_NAME)-cert`.
    - **kustomization.yaml**: Add the following to `varReference`
      ```yaml
      - kind: Certificate
        group: cert-manager.io
        path: spec/secretName
      ```

- In `config/`
  - **Create**
    - **kustomization.yaml**: This file is going to function as the new entrypoint to run `kustomize build`.
      `PROVIDER_NAME` is the name of your provider, e.g. `aws`.
      `PROVIDER_TYPE` is the type of your provider, e.g. `control-plane`, `bootstrap`, `infrastructure`.
      ```yaml
      namePrefix: {{e.g. capa-, capi-, etc.}}

      commonLabels:
        cluster.x-k8s.io/provider: "{{PROVIDER_TYPE}}-{{PROVIDER_NAME}}"

      bases:
      - crd
      - webhook # Disable this if you're not using the webhook functionality.
      - default

      patchesJson6902:
      - target: # NOTE: This patch needs to be repeatd for EACH CustomResourceDefinition you have under crd/bases.
          group: apiextensions.k8s.io
          version: v1
          kind: CustomResourceDefinition
          name: {{CRD_NAME_HERE}}
        path: patch_crd_webhook_namespace.yaml
      ```
    - **patch_crd_webhook_namespace.yaml**: This patch sets the conversion webhook namespace to `capi-webhook-system`.
        ```yaml
        - op: replace
          path: "/spec/conversion/webhook/clientConfig/service/namespace"
          value: capi-webhook-system
        ```

- In `config/default`
  - **Create**
    - **namespace.yaml**
      ```yaml
      apiVersion: v1
      kind: Namespace
      metadata:
        name: system
      ```
  - **Move**
    - `manager_image_patch.yaml` to `config/manager`
    - `manager_label_patch.yaml` to `config/manager`
    - `manager_pull_policy.yaml` to `config/manager`
    - `manager_auth_proxy_patch.yaml` to `config/manager`
    - `manager_webhook_patch.yaml` to `config/webhook`
    - `webhookcainjection_patch.yaml` to `config/webhook`
    - `manager_label_patch.yaml` to trash.
  - **Patch**
    - **kustomization.yaml**
      - Add under `resources`:
        ```yaml
        resources:
        - namespace.yaml
        ```
      - Replace `bases` with:
        ```yaml
        bases:
      	- ../rbac
        - ../manager
        ```
      - Add under `patchesStrategicMerge`:
        ```yaml
        patchesStrategicMerge:
        - manager_role_aggregation_patch.yaml
        ```
      - Remove `../crd` from `bases` (now in `config/kustomization.yaml`).
      - Remove `namePrefix` (now in `config/kustomization.yaml`).
      - Remove `commonLabels` (now in `config/kustomization.yaml`).
      - Remove from `patchesStrategicMerge`:
        - manager_image_patch.yaml
        - manager_pull_policy.yaml
        - manager_auth_proxy_patch.yaml
        - manager_webhook_patch.yaml
        - webhookcainjection_patch.yaml
        - manager_label_patch.yaml
      - Remove from `vars`:
        - CERTIFICATE_NAMESPACE
        - CERTIFICATE_NAME
        - SERVICE_NAMESPACE
        - SERVICE_NAME

- In `config/manager`
  - **Patch**
    - **manager.yaml**: Remove the `Namespace` object.
    - **kustomization.yaml**:
      - Add under `patchesStrategicMerge`:
        ```yaml
        patchesStrategicMerge:
        - manager_image_patch.yaml
        - manager_pull_policy.yaml
        - manager_auth_proxy_patch.yaml
        ```

- In `config/webhook`
  - **Patch**
    - **kustomizeconfig.yaml**
      - Add the following to `varReference`
        ```yaml
        - kind: Deployment
          path: spec/template/spec/volumes/secret/secretName
        ```
    - **kustomization.yaml**
      - Add `namespace: capi-webhook-system` at the top of the file.
      - Under `resources`, add `../certmanager` and `../manager`.
      - Add at the bottom of the file:
        ```yaml
        patchesStrategicMerge:
        - manager_webhook_patch.yaml
        - webhookcainjection_patch.yaml # Disable this value if you don't have any defaulting or validation webhook. If you don't know, you can check if the manifests.yaml file in the same directory has any contents.

        vars:
        - name: CERTIFICATE_NAMESPACE # namespace of the certificate CR
          objref:
            kind: Certificate
            group: cert-manager.io
            version: v1alpha2
            name: serving-cert # this name should match the one in certificate.yaml
          fieldref:
            fieldpath: metadata.namespace
        - name: CERTIFICATE_NAME
          objref:
            kind: Certificate
            group: cert-manager.io
            version: v1alpha2
            name: serving-cert # this name should match the one in certificate.yaml
        - name: SERVICE_NAMESPACE # namespace of the service
          objref:
            kind: Service
            version: v1
            name: webhook-service
          fieldref:
            fieldpath: metadata.namespace
        - name: SERVICE_NAME
          objref:
            kind: Service
            version: v1
            name: webhook-service
        ```
    - **manager_webhook_patch.yaml**
      - Under `containers` find `manager` and add after `name`
        ```yaml
        - "--metrics-addr=127.0.0.1:8080"
        - "--webhook-port=9443"
        ```
      - Under `volumes` find `cert` and replace `secretName`'s value with `$(SERVICE_NAME)-cert`.
    - **service.yaml**
      - Remove the `selector` map, if any. The `control-plane` label is not needed anymore, a unique label is applied using `commonLabels` under `config/kustomization.yaml`.

In `main.go`
  - Default the `webhook-port` flag to `0`
    ```go
    flag.IntVar(&webhookPort, "webhook-port", 0,
		"Webhook Server port, disabled by default. When enabled, the manager will only work as webhook server, no reconcilers are installed.")
    ```
  - The controller MUST register reconcilers if and only if `webhookPort == 0`.
  - The controller MUST register webhooks if and only if `webhookPort != 0`.

After all the changes above are performed, `kustomize build` MUST target `config/`, rather than `config/default`. Using your favorite editor, search for `config/default` in your repository and change the paths accordingly.

In addition, often the `Makefile` contains a sed-replacement for `manager_image_patch.yaml`, this file has been moved from `config/default` to `config/manager`. Using your favorite editor, search for `manager_image_patch` in your repository and change the paths accordingly.

# Apply the contract version label `cluster.x-k8s.io/<version>: version1_version2_version3` to your CRDs

- Providers MUST set `cluster.x-k8s.io/<version>` labels on all Custom Resource Definitions related to Cluster API starting with v1alpha3.
- The label is a map from an API Version of Cluster API (contract) to your Custom Resource Definition versions.
  - The value is a underscore-delimited (`_`) list of versions.
  - Each value MUST point to an available version in your CRD Spec.
- The label allows Cluster API controllers to perform automatic conversions for object references, the controllers will
  pick the last available version in the list if multiple versions are found.
- To apply the label to CRDs it's possible to use `commonLabels` in your `kustomize.yaml` file, usually in `config/crd`.

In this example we show how to map a particular Cluster API contract version to your own CRD using Kustomize's `commonLabels` feature:

```yaml
commonLabels:
  cluster.x-k8s.io/v1alpha2: v1alpha1
  cluster.x-k8s.io/v1alpha3: v1alpha2
  cluster.x-k8s.io/v1beta1: v1alphaX_v1beta1
```

# Upgrade to CRD v1

- Providers should upgrade their CRDs to v1
- Minimum Kubernetes version supporting CRDv1 is `v1.16`
- In `Makefile` target `generate-manifests:`, add the following property to the crd `crdVersions=v1`

```yaml
generate-manifests: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc.
  $(CONTROLLER_GEN) \
    paths=./api/... \
    crd:crdVersions=v1 \
    output:crd:dir=$(CRD_ROOT) \
    output:webhook:dir=$(WEBHOOK_ROOT) \
    webhook
  $(CONTROLLER_GEN) \
    paths=./controllers/... \
    output:rbac:dir=$(RBAC_ROOT) \
    rbac:roleName=manager-role
```

- For all the CRDs in the `config/crd/bases` change the version of `CustomResourceDefinition` to `v1`

```yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
```

to

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
```

- In the `config/crd/kustomizeconfig.yaml` file, change the path of the webhook

```yaml
path: spec/conversion/webhookClientConfig/service/name
```

to

```yaml
spec/conversion/webhook/clientConfig/service/name
```

- Make the same change of changing `v1beta` to `v1` version in the `config/crd/patches`
- In the `config/crd/patches/webhook_in_******.yaml` file, add the `conversionReviewVersions` property to the CRD

```yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  ...
spec:
  conversion:
    strategy: Webhook
    webhookClientConfig:
    ...
```

to

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  ...
spec:
  strategy: Webhook
  webhook:
  conversionReviewVersions: ["v1", "v1beta1"]
  clientConfig:
  ...
```


# Add `matchPolicy=Equivalent` kubebuilder marker in webhooks
- All providers should set "matchPolicy=Equivalent" kubebuilder marker for webhooks on all Custom Resource Definitions related to Cluster API starting with v1alpha3.
- Specifying `Equivalent` ensures that webhooks continue to intercept the resources they expect when upgrades enable new versions of the resource in the API server.
- E.g., `matchPolicy` is added to `AWSMachine` (/api/v1alpha3/awsmachine_webhook.go)
    ```
    // +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha3-awsmachine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=awsmachines,versions=v1alpha3,name=validation.awsmachine.infrastructure.cluster.x-k8s.io
   ```
- Support for `matchPolicy` marker has been added in [kubernetes-sigs/controller-tools](https://github.com/kubernetes-sigs/controller-tools/commit/d6efdcdd90e2a95ae7aea0dbec3252b705a9314d). Providers needs to update controller-tools dependency to make use of it, usually in `hack/tools/go.mod`.

# [OPTIONAL] Implement `--feature-gates` flag in main.go

- Cluster API now ships with a new experimental package that lives under `exp/` containing both API types and controllers.
- Controller and types should always live behind a gate defined under the `feature/` package.
- If you're planning to support experimental features or API types in your provider or codebase, you can add
  `feature.MutableGates.AddFlag(fs)` to your main.go when initializing command line flags.
  For a full example, you can refer to the `main.go` in Cluster API or under `bootstrap/kubeadm/`.

> NOTE: To enable experimental features users are required to set the same `--feature-gates` flag across providers.
> For example, if you want to enable MachinePool, you'll have to enable in both Cluster API deployment and the Kubeadm Bootstrap Provider.
> In the future, we'll revisit this user experience and provide a centralized way to configure gates across all Cluster API (inc. providers) controllers.

## clusterctl

`clusterctl` is now bundled with Cluster API, provider-agnostic and can be reused across providers.
It is the recommended way to setup a management cluster and it implements best practices to avoid common mis-configurations
and for managing the life-cycle of deployed providers, e.g. upgrades.

see [clusterctl provider contract](../../clusterctl/provider-contract.md) for more details.
