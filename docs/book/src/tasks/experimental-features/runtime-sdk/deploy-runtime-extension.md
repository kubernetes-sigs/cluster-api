# Deploy Runtime Extensions

<aside class="note warning">

<h1>Caution</h1>

Please note Runtime SDK is an advanced feature. If implemented incorrectly, a failing Runtime Extension can severely impact the Cluster API runtime.

</aside>

Cluster API requires that each Runtime Extension must be deployed using an endpoint accessible from the Cluster API
controllers. The recommended deployment model is to deploy a Runtime Extension in the management cluster by:

- Packing the Runtime Extension in a container image.
- Using a Kubernetes Deployment to run the above container inside the Management Cluster.
- Using a Cluster IP Service to make the Runtime Extension instances accessible via a stable DNS name.
- Using a cert-manager generated Certificate to protect the endpoint.

For an example, please see our [test extension](https://github.com/kubernetes-sigs/cluster-api/tree/main/test/extension)
which follows, as closely as possible, the kubebuilder setup used for controllers in Cluster API.

There are a set of important guidelines that must be considered while choosing the deployment method:

## Availability

It is recommended that Runtime Extensions should leverage some form of load-balancing, to provide high availability
and performance benefits. You can run multiple Runtime Extension servers behind a Kubernetes Service to leverage the
load-balancing that services support.

## Identity and access management

The security model for each Runtime Extension should be carefully defined, similar to any other application deployed
in the Cluster. If the Runtime Extension requires access to the apiserver the deployment must use a dedicated service 
account with limited RBAC permission. Otherwise no service account should be used.

On top of that, the container image for the Runtime Extension should be carefully designed in order to avoid
privilege escalation (e.g using [distroless](https://github.com/GoogleContainerTools/distroless) base images).
The Pod spec in the Deployment manifest should enforce security best practices (e.g. do not use privileged pods).

##  Alternative deployments methods

Alternative deployment methods can be used as long as the HTTPs endpoint is accessible, like e.g.:

- deploying the HTTPS Server as a part of another component, e.g. a controller.
- deploying the HTTPS Server outside the Management Cluster.

In those cases recommendations about availability and identity and access management still apply.

## Developing Runtime Extensions with Tilt

[The Cluster API Tilt environment](../../../developer/tilt.md) can be used to enable easier deployment and testing of Runtime Extensions in a dev workflow. The following steps for deploying an Extension can be automated with tilt:

1. Enabling both the `CLUSTER_TOPOLOGY` and `EXP_RUNTIME_SDK` flags for Cluster API.
2. Building a binary and Docker image for the Extension.
3. Deploying Kubernetes manifests including Service, RBAC, Deployment,
4. Deploying ExtensionConfig for registering a runtime extension.
5. Enabling live debugging.

The Test Extension is a Runtime Extension that's used for Cluster API end-to-end testing. It registers every available Hook - Patches and Lifecycle hooks and performs a number of patches on templates created for the Cluster.
The test extension is built into the CAPI Tilt workflow, and can be enabled with b

To deploy it with Tilt simply modify the tilt-settings.yaml to:

```yaml
default_registry: localhost:5000
deploy_kind_cluster: false
deploy_cert_manager: true
enable_providers:
  - docker
  - kubeadm-bootstrap
  - kubeadm-control-plane
kustomize_substitutions:
  EXP_CLUSTER_RESOURCE_SET: 'true'
  EXP_MACHINE_POOL: 'true'
  CLUSTER_TOPOLOGY: 'true'
  EXP_RUNTIME_SDK: 'true'
enable_addons:
  - "test-extension"
```
The test extension will now be deployed. The deployment includes the ExtensionConfig required for registering the extension with Cluster API.

The test extension implements Lifecycle hooks by reading the desired response directly from a configmap. This behaviour is part of the test extension implementation and is not shared by other runtime extensions.

To deploy a Runtime Extension that's external to the CAPI repo, for example a Runtime Extension called `sample-extension`, the following is needed:
1) `tilt-settings.yaml` must point to the `sample-extension` directory.
2) `tilt-settings.yaml` must enable the `sample-extension`, similar to how the `test-extension` is enabled above.
3) The `sample-extension` directory must contain a `tilt-addon.yaml` file.

After making these changes the `tilt-settings.yaml` will look like the below:

```yaml
default_registry: localhost:5000
deploy_kind_cluster: false
deploy_cert_manager: true
enable_providers:
  - docker
  - kubeadm-bootstrap
  - kubeadm-control-plane
kustomize_substitutions:
  EXP_CLUSTER_RESOURCE_SET: 'true'
  EXP_MACHINE_POOL: 'true'
  CLUSTER_TOPOLOGY: 'true'
  EXP_RUNTIME_SDK: 'true'
addon_repos:
  - "~/cluster-api-sample-runtime-extension"
enable_addons:
  - "test-extension"
  - "sample-extension" 
```

Inside the `cluster-api-sample-runtime-extension` directory we will have a `tilt-addon.yaml` that looks like:

```yaml
name: sample-extension
config:
  image: "gcr.io/k8s-staging-cluster-api/sample-extension"
  container_name: "extension"
  live_reload_deps: ["main.go", "handlers",]
  label: sample-extension
  resource_deps: ["capi_controller"]
  additional_resources: [
     "extensionconfig.yaml"
  ]
```