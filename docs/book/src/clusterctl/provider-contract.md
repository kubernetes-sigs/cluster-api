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
As a provider implementer, if you are interested to be added to this list, please create an issue to the [Cluster API repository](https://sigs.k8s.io/cluster-api).

</aside>

<aside class="note">

<h1>Customizing the list of providers</h1>

It is possible to customize the list of providers for `clusterctl` by changing the [clusterctl configuration](configuration.md).

</aside>

#### Creating a provider repository on GitHub

You can use GitHub release to package your provider artifacts for other people to use.

A github release can be used as a provider repository if:

* The release tag is a valid semantic version number
* The components YAML, the metadata YAML and eventually the workload cluster templates are include into the release assets.

See the [GitHub help](https://help.github.com/en/github/administering-a-repository/creating-releases) for more information 
about how to create a release.

#### Creating a local provider repository

clusterctl supports reading from a repository defined on the local file system.

A local repository can be defined by creating a `<provider-name>` folder with a `<version>` sub-folder for each hosted release;
the sub-folder name MUST be a valid semantic version number.

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

<h1> Embedded metadata </h1>

The `clusterctl` command can ship with embedded metadata for pre-defined providers.
If, as a provider implementer, you are interested to this feature, please send a PR to the Cluster API repository.

</aside>

### Components YAML 

The provider is required to generate a **components YAML** file and publish it to the provider's repository.
This file is a single YAML with _all_ the components required for installing the provider itself (CRDs, Controller, RBAC etc.). 

The following rules apply:

#### Naming conventions

It is strongly recommended that:
* Core provider release a file called `core-components.yaml`
* Infrastructure providers release a file called `infrastructure-components.yaml`
* Bootstrap providers release a file called ` bootstrap-components.yaml`
* Control plane providers release a file called `control-plane-components.yaml`

#### Target namespace

The components YAML should contain one Namespace object, which will be used as the default target namespace
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
will look for objects to reconcile.

#### Variables

The components YAML can contain environment variables matching the regexp `\${\s*([A-Z0-9_]+)\s*}`; it is highly
recommended to prefix the variable name with the provider name e.g. `${ AWS_CREDENTIALS }`

Additionally, each provider should create user facing documentation with the list of required variables and with all the additional
notes that are required to assist the user in defining the value for each variable.

#### Labels
The components YAML components should be labeled with
`cluster.x-k8s.io/provider` and the name of the provider. This will enable an
easier transition from `kubectl apply` to `clusterctl`.

As a reference you can consider the labels applied to the following
providers.

| Provider Name| Label                                            |
|--------------|--------------------------------------------------|
|CAPI          | cluster.x-k8s.io/provider=cluster-api            |
|CABPK         | cluster.x-k8s.io/provider=bootstrap-kubeadm      |
|CACPK         | cluster.x-k8s.io/provider=control-plane-kubeadm  |
|CAPA          | cluster.x-k8s.io/provider=infrastructure-aws     |
|CAPV          | cluster.x-k8s.io/provider=infrastructure-vsphere |
|CAPD          | cluster.x-k8s.io/provider=infrastructure-docker  |

### Workload cluster templates

An infrastructure provider could publish a **cluster templates** file to be used by `clusterctl config cluster`.
This is single YAML with _all_ the objects required to create a new workload cluster.

The following rules apply:

#### Naming conventions

Cluster templates MUST be stored in the same folder as the component YAML and follow this naming convention:
1. The default cluster template should be named `cluster-template.yaml`.
2. Additional cluster template should be named `cluster-template-{flavor}.yaml`. e.g `cluster-template-prod.yaml`

`{flavor}` is the name the user can pass to the `clusterctl config cluster --flavor` flag to identify the specific template to use.
 
Each provider SHOULD create user facing documentation with the list of available cluster templates.

#### Target namespace

The cluster template YAML MUST assume the target namespace already exists.

All the objects in the cluster template YAML MUST be deployed in the same namespace. 

#### Variables

The cluster templates YAML can also contain environment variables (as can the components YAML).

Additionally, each provider should create user facing documentation with the list of required variables and with all the additional
notes that are required to assist the user in defining the value for each variable.

##### Common variables

The `clusterctl config cluster` command allows user to set a small set of common variables via CLI flags or command arguments.

Templates writers should use the common variables to ensure consistency across providers and a simpler user experience
(if compared to the usage of OS environment variables or the `clusterctl` config file).

| CLI flag                | Variable name     | Note                                        |
| ---------------------- | ----------------- | ------------------------------------------- |
|`--target-namespace`| `${ NAMESPACE }` | The namespace where the workload cluster should be deployed |
|`--kubernetes-version`| `${ KUBERNETES_VERSION }` | The Kubernetes version to use for the workload cluster |
|`--controlplane-machine-count`| `${ CONTROL_PLANE_MACHINE_COUNT }` | The number of control plane machines to be added to the workload cluster |
|`--worker-machine-count`| `${ WORKER_MACHINE_COUNT }` | The number of worker machines to be added to the workload cluster |

Additionally, value of the command argument to `clusterctl config cluster <cluster-name>` (`<cluster-name>` in this case), will 
be applied to every occurrence of the `${ CLUSTER_NAME }` variable.

## OwnerReferences chain

Each provider is responsible to ensure that all the providers resources (like e.g. `VSphereCluster`, `VSphereMachine`, `VSphereVM` etc. 
for the `vsphere` provider) MUST have a `Metadata.OwnerReferences` entry that links directly or indirectly to a `Cluster` object.

Please note that all the provider specific resources that are referenced by the Cluster API core objects will get the `OwnerReference`
sets by the Cluster API core controllers, e.g.:

- The Cluster controller ensures that all the objects referenced in `Cluster.Spec.InfrastructureRef` get an `OwnerReference` 
  that links directly to the corresponding `Cluster`.
- The Machine controller ensures that all the objects referenced in `Machine.Spec.InfrastructureRef` get an `OwnerReference` 
  that links to the corresponding `Machine`, and the `Machine` is linked to the `Cluster` through its own `OwnerReference` chain.  

That means that, practically speaking, provider implementers are responsible for ensuring that the `OwnerReference`s
are set only for objects that are not directly referenced by Cluster API core objects, e.g.:

- All the `VSphereVM` instances should get an `OwnerReference` that links to the corresponding `VSphereMachine`, and the `VSphereMachine`
  is linked to the `Cluster` through its own `OwnerReference` chain.

## Additional notes

### Components YAML transformations

Provider authors should be aware of the following transformations that `clusterctl` applies during component installation:

* Variable substitution;
* Enforcement of target namespace:
    * The name of the namespace object is set;
    * The namespace field of all the objects is set (with exception of cluster wide objects like e.g. ClusterRoles);
    * ClusterRole and ClusterRoleBinding are renamed by adding a “${namespace}-“ prefix to the name; this change reduces the risks 
    of conflicts between several instances of the same provider in case of multi tenancy; 
* Enforcement of watching namespace;
* All components are labeled;

### Cluster template transformations

Provider authors should be aware of the following transformations that `clusterctl` applies during components installation:

* Variable substitution;
* Enforcement of target namespace:
    * The namespace field of all the objects is set;

### Links to external objects

The `clusterctl` command requires that both the components YAML and the cluster templates contain _all_ the required 
objects.

If, for any reason, the provider authors/YAML designers decide not to comply with this recommendation and e.g. to

* implement links to external objects from a component YAML (e.g. secrets, aggregated ClusterRoles NOT included in the component YAML)
* implement link to external objects from a cluster template (e.g. secrets, configMaps NOT included in the cluster template)

The provider authors/YAML designers should be aware that it is their responsibility to ensure the proper
functioning of all the `clusterctl` features both in single tenancy or multi-tenancy scenarios and/or document known limitations.

### Move 

Provider authors should be aware that `clusterctl move` command implements a discovery mechanism that considers:

* All the objects of Kind defined in one of the CRDs installed by clusterctl using `clusterctl init`. 
* `Secret` and `ConfigMap` objects.
* the `OwnerReference` chain of the above objects.

`clusterctl move` does NOT consider any objects:

* Not included in the set of objects defined above.
* Included in the set of objects defined above, but not directly or indirectly to a `Cluster` object through the `OwnerReference` chain.
 
If moving some of excluded object is required, the provider authors should create documentation describing the
the exact move sequence to be executed by the user.

Additionally, provider authors should be aware that `clusterctl move` assumes all the provider's Controllers respect the
`Cluster.Spec.Paused` field introduced in the v1alpha3 Cluster API specification. 
 
### Adopt

WIP
