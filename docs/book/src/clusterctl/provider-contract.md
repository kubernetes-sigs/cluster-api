# clusterctl Provider Contract

The `clusterctl` command is designed to work with all the providers compliant with the following rules.

## Provider Repositories

Each provider MUST define a **provider repository**, that is a well-known place where the release assets for
a provider are published.

The provider repository MUST contain the following files:

* The metadata YAML
* The components YAML

Additionally, the provider repository SHOULD contain the following files:

* Workload cluster templates

<aside class="note">

<h1> Pre-defined list of providers </h1>

The `clusterctl` command ships with a pre-defined list of provider repositories that allows a simpler "out-of-the-box" user experience.
As a provider implementer, if you are interested in being added to this list, please create an issue to the [Cluster API repository](https://sigs.k8s.io/cluster-api).

</aside>

<aside class="note">

<h1>Customizing the list of providers</h1>

It is possible to customize the list of providers for `clusterctl` by changing the [clusterctl configuration](configuration.md).

</aside>

#### Creating a provider repository on GitHub

You can use a GitHub release to package your provider artifacts for other people to use.

A GitHub release can be used as a provider repository if:

* The release tag is a valid semantic version number
* The components YAML, the metadata YAML and eventually the workload cluster templates are include into the release assets.

See the [GitHub help](https://help.github.com/en/github/administering-a-repository/creating-releases) for more information
about how to create a release.

#### Creating a local provider repository

clusterctl supports reading from a repository defined on the local file system.

A local repository can be defined by creating a `<provider-label>` folder with a `<version>` sub-folder for each hosted release;
the sub-folder name MUST be a valid semantic version number. e.g.

```
~/local-repository/infrastructure-aws/v0.5.2
```

Each version sub-folder MUST contain the corresponding components YAML, the metadata YAML and eventually the workload cluster templates.

### Metadata YAML

The provider is required to generate a **metadata YAML** file and publish it to the provider's repository.

The metadata YAML file documents the release series of each provider and maps each release series to an API Version of Cluster API (contract).

For example, for Cluster API:

```yaml
apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
kind: Metadata
releaseSeries:
- major: 0
  minor: 3
  contract: v1alpha3
- major: 0
  minor: 2
  contract: v1alpha2
```

<aside class="note">

<h1> Note on user experience</h1>

For clusterctl versions pre-v1alpha4, if provider implementers only update the clusterctl's built-in metadata and don't provide a `metadata.yaml` in a new release, users are forced to update `clusterctl`
to the latest released version in order to properly install the provider.

As a related example, see the details in [issue 3418].

To address the above explained issue, the embedded metadata within clusterctl has been removed (as of v1alpha4) to prevent the reliance on using the latest version of clusterctl in order to pull newer provider releases.

For more information see the details in [issue 3515].
</aside>

### Components YAML

The provider is required to generate a **components YAML** file and publish it to the provider's repository.
This file is a single YAML with _all_ the components required for installing the provider itself (CRDs, Controller, RBAC etc.).

The following rules apply:

#### Naming conventions

It is strongly recommended that:
* Core providers release a file called `core-components.yaml`
* Infrastructure providers release a file called `infrastructure-components.yaml`
* Bootstrap providers release a file called ` bootstrap-components.yaml`
* Control plane providers release a file called `control-plane-components.yaml`

#### Target namespace

The instance components should contain one Namespace object, which will be used as the default target namespace
when creating the provider components.

All the objects in the components YAML MUST belong to the target namespace, with the exception of objects that
are not namespaced, like ClusterRoles/ClusterRoleBinding and CRD objects.

<aside class="note warning">

<h1>Warning</h1>

If the generated component YAML does't contain a Namespace object, the user will be required to provide one to `clusterctl init`
using the `--target-namespace` flag.

In case there is more than one Namespace object in the components YAML, `clusterctl` will generate an error and abort
the provider installation.

</aside>

#### Controllers & Watching namespace

Each provider is expected to deploy controllers using a Deployment.

While defining the Deployment Spec, the container that executes the controller binary MUST be called `manager`.

The manager MUST support a `--namespace` flag for specifying the namespace where the controller
will look for objects to reconcile; however, clusterctl will always install providers watching for all namespaces 
(`--namespace=""`); for more details see [support for multiple instances](../../developer/architecture/controllers/support-multiple-instances.md)
for more context.

#### Variables

The components YAML can contain environment variables matching the format ${VAR}; it is highly
recommended to prefix the variable name with the provider name e.g. `${AWS_CREDENTIALS}`

<aside class="note warning">

<h1>Warning</h1>

`clusterctl` currently supports variables with leading/trailing spaces such
as: `${ VAR }`, `${ VAR}`,`${VAR }`. However, these formats will be deprecated
in the near future. e.g. v1alpha4.

Formats such as `${VAR$FOO}` are not supported.
</aside>

`clusterctl` uses the library [drone/envsubst][drone-envsubst] to perform
variable substitution.

```bash
# If `VAR` is not set or empty, the default value is used. This is true for
# all the following formats.
${VAR:=default}
${VAR=default}
${VAR:-default}
```
Other functions such as substring replacement are also supported by the
library. See [drone/envsubst][drone-envsubst] for more information.

Additionally, each provider should create user facing documentation with the list of required variables and with all the additional
notes that are required to assist the user in defining the value for each variable.

#### Labels
The components YAML components should be labeled with
`cluster.x-k8s.io/provider` and the name of the provider. This will enable an
easier transition from `kubectl apply` to `clusterctl`.

As a reference you can consider the labels applied to the following
providers.

| Provider Name| Label                                                  |
|--------------|--------------------------------------------------------|
|CAPI          | cluster.x-k8s.io/provider=cluster-api                  |
|CABPK         | cluster.x-k8s.io/provider=bootstrap-kubeadm            |
|CACPK         | cluster.x-k8s.io/provider=control-plane-kubeadm        |
|CACPN         | cluster.x-k8s.io/provider=control-plane-nested        |
|CAPA          | cluster.x-k8s.io/provider=infrastructure-aws           |
|CAPV          | cluster.x-k8s.io/provider=infrastructure-vsphere       |
|CAPD          | cluster.x-k8s.io/provider=infrastructure-docker        |
|CAPM3         | cluster.x-k8s.io/provider=infrastructure-metal3        |
|CAPN          | cluster.x-k8s.io/provider=infrastructure-nested        |
|CAPP          | cluster.x-k8s.io/provider=infrastructure-packet        |
|CAPZ          | cluster.x-k8s.io/provider=infrastructure-azure         |
|CAPO          | cluster.x-k8s.io/provider=infrastructure-openstack     |
|CAPDO         | cluster.x-k8s.io/provider=infrastructure-digitalocean  |

### Workload cluster templates

An infrastructure provider could publish a **cluster templates** file to be used by `clusterctl generate cluster`.
This is single YAML with _all_ the objects required to create a new workload cluster.

The following rules apply:

#### Naming conventions

Cluster templates MUST be stored in the same folder as the component YAML and follow this naming convention:
1. The default cluster template should be named `cluster-template.yaml`.
2. Additional cluster template should be named `cluster-template-{flavor}.yaml`. e.g `cluster-template-prod.yaml`

`{flavor}` is the name the user can pass to the `clusterctl generate cluster --flavor` flag to identify the specific template to use.

Each provider SHOULD create user facing documentation with the list of available cluster templates.

#### Target namespace

The cluster template YAML MUST assume the target namespace already exists.

All the objects in the cluster template YAML MUST be deployed in the same namespace.

#### Variables

The cluster templates YAML can also contain environment variables (as can the components YAML).

Additionally, each provider should create user facing documentation with the list of required variables and with all the additional
notes that are required to assist the user in defining the value for each variable.

##### Common variables

The `clusterctl generate cluster` command allows user to set a small set of common variables via CLI flags or command arguments.

Templates writers should use the common variables to ensure consistency across providers and a simpler user experience
(if compared to the usage of OS environment variables or the `clusterctl` config file).

| CLI flag                | Variable name     | Note                                        |
| ---------------------- | ----------------- | ------------------------------------------- |
|`--target-namespace`| `${NAMESPACE}` | The namespace where the workload cluster should be deployed |
|`--kubernetes-version`| `${KUBERNETES_VERSION}` | The Kubernetes version to use for the workload cluster |
|`--controlplane-machine-count`| `${CONTROL_PLANE_MACHINE_COUNT}` | The number of control plane machines to be added to the workload cluster |
|`--worker-machine-count`| `${WORKER_MACHINE_COUNT}` | The number of worker machines to be added to the workload cluster |

Additionally, the value of the command argument to `clusterctl generate cluster <cluster-name>` (`<cluster-name>` in this case), will
be applied to every occurrence of the `${ CLUSTER_NAME }` variable.

## OwnerReferences chain

Each provider is responsible to ensure that all the providers resources (like e.g. `VSphereCluster`, `VSphereMachine`, `VSphereVM` etc.
for the `vsphere` provider) MUST have a `Metadata.OwnerReferences` entry that links directly or indirectly to a `Cluster` object.

Please note that all the provider specific resources that are referenced by the Cluster API core objects will get the `OwnerReference`
set by the Cluster API core controllers, e.g.:

* The Cluster controller ensures that all the objects referenced in `Cluster.Spec.InfrastructureRef` get an `OwnerReference`
  that links directly to the corresponding `Cluster`.
* The Machine controller ensures that all the objects referenced in `Machine.Spec.InfrastructureRef` get an `OwnerReference`
  that links to the corresponding `Machine`, and the `Machine` is linked to the `Cluster` through its own `OwnerReference` chain.

That means that, practically speaking, provider implementers are responsible for ensuring that the `OwnerReference`s
are set only for objects that are not directly referenced by Cluster API core objects, e.g.:

* All the `VSphereVM` instances should get an `OwnerReference` that links to the corresponding `VSphereMachine`, and the `VSphereMachine`
  is linked to the `Cluster` through its own `OwnerReference` chain.

## Additional notes

### Components YAML transformations

Provider authors should be aware of the following transformations that `clusterctl` applies during component installation:

* Variable substitution;
* Enforcement of target namespace:
  * The name of the namespace object is set;
  * The namespace field of all the objects is set (with exception of cluster wide objects like e.g. ClusterRoles);
* All components are labeled;

### Cluster template transformations

Provider authors should be aware of the following transformations that `clusterctl` applies during components installation:

* Variable substitution;
* Enforcement of target namespace:
  * The namespace field of all the objects are set;

### Links to external objects

The `clusterctl` command requires that both the components YAML and the cluster templates contain _all_ the required
objects.

If, for any reason, the provider authors/YAML designers decide not to comply with this recommendation and e.g. to

* implement links to external objects from a component YAML (e.g. secrets, aggregated ClusterRoles NOT included in the component YAML)
* implement link to external objects from a cluster template (e.g. secrets, configMaps NOT included in the cluster template)

The provider authors/YAML designers should be aware that it is their responsibility to ensure the proper
functioning of `clusterctl` when using non-compliant component YAML or cluster templates.

### Move

Provider authors should be aware that `clusterctl move` command implements a discovery mechanism that considers:

* All the Kind defined in one of the CRDs installed by clusterctl using `clusterctl init` (identified via the `clusterctl.cluster.x-k8s.io label`); 
  For each CRD, discovery collects:
  * All the objects from the namespace being moved only if the CRD scope is `Namespaced`.
  * All the objects if the CRD scope is `Cluster`.
* All the `ConfigMap` objects from the namespace being moved.
* All the `Secret` objects from the namespace being moved and from the namespaces where infrastructure providers are installed.

After completing discovery, `clusterctl move` moves to the target cluster only the objects discovered in the previous phase
that are compliant with one of the following rules:
  * The object is directly or indirectly linked to a `Cluster` object (linked through the `OwnerReference` chain).
  * The object is a secret containing a user provided certificate (linked to a `Cluster` object via a naming convention).
  * The object is directly or indirectly linked to a `ClusterResourceSet` object (through the `OwnerReference` chain).
  * The object is directly or indirectly linked to another object with the `clusterctl.cluster.x-k8s.io/move-hierarchy` 
    label, e.g. the infrastructure Provider ClusterIdentity objects (linked through the `OwnerReference` chain).
  * The object hase the `clusterctl.cluster.x-k8s.io/move` label or the `clusterctl.cluster.x-k8s.io/move-hierarchy` label,
    e.g. the CPI config secret. 
    
Note. `clusterctl.cluster.x-k8s.io/move` and `clusterctl.cluster.x-k8s.io/move-hierarchy` labels could be applied 
to single objects or at the CRD level (the label applies to all the objects).
    
Please note that during move:
  * Namespaced objects, if not existing in the target cluster, are created.
  * Namespaced objects, if already existing in the target cluster, are updated.  
  * Namespaced objects are removed from the source cluster.
  * Global objects, if not existing in the target cluster, are created.
  * Global objects, if already existing in the target cluster, are not updated.
  * Global objects are not removed from the source cluster.
  * Namespaced objects which are part of an owner chain that starts with a global object (e.g. a secret containing 
    credentials for an infrastructure Provider ClusterIdentity) are treated as Global objects.

<aside class="note warning">

<h1>Warning</h1>

When using the "move" label, if the CRD is a global resource, the object is copied to the target cluster but not removed from the source cluster. It is up to the user to remove the source object as necessary.

</aside>

If moving some of excluded object is required, the provider authors should create documentation describing the
exact move sequence to be executed by the user.

Additionally, provider authors should be aware that `clusterctl move` assumes all the provider's Controllers respect the
`Cluster.Spec.Paused` field introduced in the v1alpha3 Cluster API specification.

<!--LINKS-->
[drone-envsubst]: https://github.com/drone/envsubst
[issue 3418]: https://github.com/kubernetes-sigs/cluster-api/issues/3418
[issue 3515]: https://github.com/kubernetes-sigs/cluster-api/issues/3515
