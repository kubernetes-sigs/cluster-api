# Cluster API v1alpha3 compared to v1alpha4

## Minimum Go version

- The Go version used by Cluster API is now Go 1.15+

## Controller Runtime version

- The Controller Runtime version is now v0.7.+

## Kind version

- The KIND version used for this release is v0.9.x

## Upgrade kube-rbac-proxy to v0.8.0

- Find and replace the `kube-rbac-proxy` version (usually the image is `gcr.io/kubebuilder/kube-rbac-proxy`) and update it to `v0.8.0`.

## The controllers.DeleteNodeAnnotation constant has been removed

- This annotation `cluster.k8s.io/delete-machine` was originally deprecated a while ago when we moved our types under the `x-k8s.io` domain.

## The controllers.DeleteMachineAnnotation has been moved to v1alpha4.DeleteMachineAnnotation

- This annotation was previously exported as part of the controllers package, instead this should be a versioned annotation under the api packages.

## Align manager flag names with upstream Kubernetes components

- Rename `--metrics-addr` to `--metrics-bind-addr`
- Rename `--leader-election` to `--leader-elect`

## util.ManagerDelegatingClientFunc has been removed

This function was originally used to generate a delegating client when creating a new manager.

Controller Runtime v0.7.x now uses a `ClientBuilder` in its Options struct and it uses
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
    - Verify that fetaure flags required by your container are properly set
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
1. Edit the `/config/manager/kustomization.yaml` file:
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

1. Edit the `/config/certmanager/certificates.yaml` file and replace all the occurencies of `cert-manager.io/v1alpha2`
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
