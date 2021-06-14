# Cluster API v1alpha3 compared to v1alpha4

## Minimum Go version

- The Go version used by Cluster API is now Go 1.16+
  - In case cloudbuild is used to push images, please upgrade to `gcr.io/k8s-testimages/gcb-docker-gcloud:v20210331-c732583`
    in the cloudbuild YAML files.

## Controller Runtime version

- The Controller Runtime version is now v0.9.+

## Controller Tools version (if used)

- The Controller Tools version is now v0.6.+

## Kind version

- The KIND version used for this release is v0.11.x

## :warning: Go Module changes :warning:

- The `test` folder now ships with its own Go module `sigs.k8s.io/cluster-api/test`.
- The module is going to be tagged and versioned as part of the release.
- Folks importing the test e2e framework or the docker infrastructure provider need to import the new module.
  - When imported, the test module version should always match the Cluster API one.
  - Add the following line in `go.mod` to replace the cluster-api dependency in the test module (change the version to your current Cluster API version):
  ```
  replace sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v0.4.x
  ```
- The CAPD go module in test/infrastructure/docker has been removed.

## Klog version

- The klog package used has been upgraded to v2.5.x. It is recommended that
  all providers also switch to using v2.

  - Change `import k8s.io/klog` to `import k8s.io/klog/v2`
  - Change `import k8s.io/klog/klogr` to `import k8s.io/klog/v2/klogr`
  - Update `go.mod` to `k8s.io/klog/v2 v2.5.0`
  - Run `go mod tidy` to ensure all dependencies are updated.

## The controllers.DeleteNodeAnnotation constant has been removed

- This annotation `cluster.k8s.io/delete-machine` was originally deprecated a while ago when we moved our types under the `x-k8s.io` domain.

## The controllers.DeleteMachineAnnotation has been moved to v1alpha4.DeleteMachineAnnotation

- This annotation was previously exported as part of the controllers package, instead this should be a versioned annotation under the api packages.

## Align manager flag names with upstream Kubernetes components

- Rename `--metrics-addr` to `--metrics-bind-addr`
- Rename `--leader-election` to `--leader-elect`

## util.ManagerDelegatingClientFunc has been removed

This function was originally used to generate a delegating client when creating a new manager.

Controller Runtime v0.9.x now uses a `ClientBuilder` in its Options struct and it uses
the delegating client by default under the hood, so this can be now removed.

## Use to Controller Runtime's new fake client builder

- The functions `fake.NewFakeClientWithScheme` and `fake.NewFakeClient` have been deprecated.
- Switch to `fake.NewClientBuilder().WithObjects().Build()` instead, which provides a cleaner interface
  to create a new fake client with objects, lists, or a scheme.

## Multi tenancy

Up until v1alpha3, the need of supporting multiple credentials was addressed by running multiple
instances of the same provider, each one with its own set of credentials while watching different namespaces.

Starting from v1alpha4 instead we are going require that an infrastructure provider should manage different credentials,
each one of them corresponding to an infrastructure tenant.

see [Multi-tenancy](../architecture/controllers/multi-tenancy.md) and [Support multiple instances](../architecture/controllers/support-multiple-instances.md) for
more details.

Specific changes related to this topic will be detailed in this document.

## Change types with arrays of pointers to custom objects

The conversion-gen code from the `1.20.x` release onward generates incorrect conversion functions for types having arrays of pointers to custom objects. Change the existing types to contain objects instead of pointer references.

## Optional flag for specifying webhook certificates dir
Add optional flag `--webhook-cert-dir={string-value}` which allows user to specify directory where webhooks will get tls certificates.
If flag has not provided, default value from `controller-runtime` should be used.

## Required kustomize changes to have a single manager watching all namespaces and answer to webhook calls

In an effort to simplify the management of Cluster API components, and realign with Kubebuilder configuration,
we're requiring some changes to move all webhooks back into a single deployment manager, and to allow Cluster
API watch all namespaces it manages.
For a `/config` folder reference, please use the testdata in the Kubebuilder project: https://github.com/kubernetes-sigs/kubebuilder/tree/master/testdata/project-v3/config

**Pre-requisites**

Provider's `/config` folder has the same structure of  `/config` folder in CAPI controllers.

**Changes in the `/config/webhook` folder:**

1. Edit the `/config/webhook/kustomization.yaml` file:
    - Remove the `namespace:` configuration
    - In the `resources:` list, remove the following items:
      ```
      - ../certmanager
      - ../manager
      ```
    - Remove the `patchesStrategicMerge` list
    - Copy the `vars` list into a temporary file to be used later in the process
    - Remove the `vars` list
1. Edit the `config/webhook/kustomizeconfig.yaml` file:
    - In the `varReference:` list, remove the item with `kind: Deployment`
1. Edit the `/config/webhook/manager_webhook_patch.yaml` file and remove
   the `args` list from the `manager` container.
1. Move the following files to the `/config/default` folder
    - `/config/webhook/manager_webhook_patch.yaml`
    - `/config/webhook/webhookcainjection_patch.yaml`

**Changes in the `/config/manager` folder:**

1. Edit the `/config/manager/kustomization.yaml` file:
    - Remove the `patchesStrategicMerge` list
1. Edit the `/config/manager/manager.yaml` file:
    - Add the following items to the `args` list for the `manager` container list
    ```
    - "--metrics-bind-addr=127.0.0.1:8080"
    ```
    - Verify that feature flags required by your container are properly set
      (as it was in `/config/webhook/manager_webhook_patch.yaml`).
1. Edit the `/config/manager/manager_auth_proxy_patch.yaml` file:
    - Remove the patch for the container with name `manager`
1. Move the following files to the `/config/default` folder
    - `/config/manager/manager_auth_proxy_patch.yaml`
    - `/config/manager/manager_image_patch.yaml`
    - `/config/manager/manager_pull_policy.yaml`

**Changes in the `/config/default` folder:**
1. Create a file named `/config/default/kustomizeconfig.yaml` with the following content:
   ```
   # This configuration is for teaching kustomize how to update name ref and var substitution
   varReference:
   - kind: Deployment
     path: spec/template/spec/volumes/secret/secretName
   ```
1. Edit the `/config/default/kustomization.yaml` file:
    - Add the `namePrefix` and the `commonLabels` configuration values copying values from the `/config/kustomization.yaml` file
    - In the `bases:` list, add the following items:
      ```
      - ../crd
      - ../certmanager
      - ../webhook
      ```
    - Add the `patchesStrategicMerge:` list, with the following items:
      ```
      - manager_auth_proxy_patch.yaml
      - manager_image_patch.yaml
      - manager_pull_policy.yaml
      - manager_webhook_patch.yaml
      - webhookcainjection_patch.yaml
      ```
    - Add a `vars:` configuration using the value from the temporary file created while modifying `/config/webhook/kustomization.yaml`
    - Add the `configurations:` list with the following items:
      ```
      - kustomizeconfig.yaml
      ```

**Changes in the `/config` folder:**

1. Remove the `/config/kustomization.yaml` file
1. Remove the `/config/patch_crd_webhook_namespace.yaml` file

**Changes in the `main.go` file:**

1. Change default value for the flags `webhook-port` flag to `9443`
1. Change your code so all the controllers and the webhooks are started no matter if the webhooks port selected.

**Other changes:**

- makefile
    - update all the references for `/config/manager/manager_image_patch.yaml` to `/config/default/manager_image_patch.yaml`
    - update all the references for `/config/manager/manager_pull_policy.yaml` to `/config/default/manager_pull_policy.yaml`
    - update all the call to `kustomize` targeting `/config` to target `/config/default` instead.
- E2E config files
    - update provider sources reading from `/config` to read from `/config/default` instead.
- clusterctl-settings.json file
    - if the `configFolder` value is defined, update from `/config` to `/config/default`.

## Upgrade cert-manager to v1.1.0

NB. instructions assumes "Required kustomize changes to have a single manager watching all namespaces and answer to webhook calls"
should be executed before this changes.

**Changes in the `/config/certmanager` folder:**

1. Edit the `/config/certmanager/certificate.yaml` file and replace all the occurrences of `cert-manager.io/v1alpha2`
   with `cert-manager.io/v1`

**Changes in the `/config/default` folder:**

1. Edit the `/config/default/kustomization.yaml` file and replace all the occurencies of
   ```
         kind: Certificate
         group: cert-manager.io
         version: v1alpha2
   ```
   with
   ```
         kind: Certificate
         group: cert-manager.io
         version: v1
   ```
## Support the cluster.x-k8s.io/watch-filter label and watch-filter flag.

- A new label `cluster.x-k8s.io/watch-filter` provides the ability to filter the controllers to only reconcile objects with a specific label.
- A new flag `watch-filter` enables users to specify the label value for the `cluster.x-k8s.io/watch-filter` label on controller boot.
- The flag which enables users to set the flag value can be structured like this:
  ```go
  	fs.StringVar(&watchFilterValue, "watch-filter", "", fmt.Sprintf("Label value that the controller watches to reconcile cluster-api objects. Label key is always %s. If unspecified, the controller watches for all cluster-api objects.", clusterv1.WatchLabel))
  ```
- The `ResourceNotPausedAndHasFilterLabel` predicate is a useful helper to check for the pause annotation and the filter label easily:
  ```go
  c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineSet{}).
		Owns(&clusterv1.Machine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			handler.EnqueueRequestsFromMapFunc(r.MachineToMachineSets),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
  ```

## Required changes to have individual service accounts for controllers.

1. Create a new service account such as:
  ```yaml
  apiVersion: v1
  kind: ServiceAccount
  metadata:
    name: manager
  namespace: system
  ```
2. Change the `subject` of the managers `ClusterRoleBinding` to match:
  ```yaml
  subjects:
  - kind: ServiceAccount
    name: manager
    namespace: system
  ```
3. Add the correct `serviceAccountName` to the manager deployment:
  ```yaml
  serviceAccountName: manager
  ```

## Percentage String or Int API input will fail with a string different from an integer with % appended.

`MachineDeployment.Spec.Strategy.RollingUpdate.MaxSurge`, `MachineDeployment.Spec.Strategy.RollingUpdate.MaxUnavailable` and `MachineHealthCheck.Spec.MaxUnhealthy` would have previously taken a String value with an integer character in it e.g "3" as a valid input and process it as a percentage value.
Only String values like "3%" or Int values e.g 3 are valid input values now. A string not matching the percentage format will fail, e.g "3".

## Required change to support externally managed infrastructure.

- A new annotation `cluster.x-k8s.io/managed-by` has been introduced that allows cluster infrastructure to be managed
  externally.
- When this annotation is added to an `InfraCluster` resource, the controller for these resources should not reconcile
  the resource.
- The `ResourceIsNotExternallyManaged` predicate is a useful helper to check for the annotation and the filter the resource easily:
  ```go
  c, err := ctrl.NewControllerManagedBy(mgr).
		For(&providerv1.InfraCluster{}).
		Watches(...).
		WithOptions(options).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(ctrl.LoggerFrom(ctx))).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
  ```
- Note: this annotation also has to be checked in other cases, e.g. when watching for the Cluster resource.

## MachinePool API group changed to `cluster.x-k8s.io`

MachinePool is today an experiment, and the API group we originally decided to pick was `exp.cluster.x-k8s.io`. Given that the intent is in the future to move MachinePool to the core API group, we changed the experiment to use `cluster.x-k8s.io` group to avoid future breaking changes.

All InfraMachinePool implementations should be moved to `infrastructure.cluster.x-k8s.io`. See `DockerMachinePool` for an example.

Note that MachinePools are still experimental after this change and should still be feature gated.

## Golangci-lint configuration

There were a lot of new useful linters added to `.golangci.yml`. Of course it's not mandatory to use `golangci-lint` or
a similar configuration, but it might make sense regardless. Please note there was previously an error in
the `exclude` configuration which has been fixed in [#4657](https://github.com/kubernetes-sigs/cluster-api/pull/4657). As
this configuration has been duplicated in a few other providers, it could be that you're also affected.

# test/helpers.NewFakeClientWithScheme has been removed

This function used to create a new fake client with the given scheme for testing,
and all the objects given as input were initialized with a resource version of "1".
The behavior of having a resource version in fake client has been fixed in controller-runtime,
and this function isn't needed anymore.

## Required kustomize changes to remove Kubeadm-rbac-proxy

NB. instructions assumes "Required kustomize changes to have a single manager watching all namespaces and answer to webhook calls"
should be executed before this changes.

**Changes in the `/config/default` folder:**
1. Edit `/config/default/kustomization.yaml` and remove the `manager_auth_proxy_patch.yaml` item from the `patchesStrategicMerge` list.
1. Delete the `/config/default/manager_auth_proxy_patch.yaml` file.

**Changes in the `/config/manager` folder:**
1. Edit `/config/manager/manager.yaml` and remove the `--metrics-bind-addr=127.0.0.1:8080` arg from the `args` list.

**Changes in the `/config/rbac` folder:**
1. Edit `/config/rbac/kustomization.yaml` and remove following items from the `resources` list.
   - `auth_proxy_service.yaml`
   - `auth_proxy_role.yaml`
   - `auth_proxy_role_binding.yaml`
1. Delete the `/config/rbac/auth_proxy_service.yaml` file.
1. Delete the `/config/rbac/auth_proxy_role.yaml` file.
1. Delete the `/config/rbac/auth_proxy_role_binding.yaml` file.

**Changes in the `main.go` file:**
1. Change the default value for the `metrics-bind-addr` from `:8080` to `localhost:8080`

## Required cluster template changes

- `spec.infrastructureTemplate` has been moved to `spec.machineTemplate.infrastructureRef`. Thus, cluster templates which include `KubeadmControlPlane`
have to be adjusted accordingly.
- `spec.nodeDrainTimeout` has been moved to `spec.machineTemplate.nodeDrainTimeout`.
