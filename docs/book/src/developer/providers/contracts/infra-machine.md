# Contract rules for InfraMachine

Infrastructure providers SHOULD implement an InfraMachine resource.

The goal of an InfraMachine resource is to manage the lifecycle of a provider-specific machine instances.
These may be physical or virtual instances, and they represent the infrastructure for Kubernetes nodes.

The InfraMachine resource will be referenced by one of the Cluster API core resources, Machine.

The [Machine's controller](../../core/controllers/machine.md) will be responsible to coordinate operations of the InfraMachine,
and the interaction between the Machine's controller and the InfraMachine resource is based on the contract
rules defined in this page.

Once contract rules are satisfied by an InfraMachine implementation, other implementation details
could be addressed according to the specific needs (Cluster API is not prescriptive).

Nevertheless, it is always recommended to take a look at Cluster API controllers,
in-tree providers, other providers and use them as a reference implementation (unless custom solutions are required
in order to address very specific needs).

In order to facilitate the initial design for each InfraMachine resource, a few [implementation best practices] and [infrastructure Provider Security Guidance]
are explicitly called out in dedicated pages.

<aside class="note warning">

<h1>Never rely on Cluster API behaviours not defined as a contract rule!</h1>

When developing a provider, you MUST consider any Cluster API behaviour that is not defined by a contract rule
as a Cluster API internal implementation detail, and internal implementation details can change at any time.

Accordingly, in order to not expose users to the risk that your provider breaks when the Cluster API internal behavior
changes, you MUST NOT rely on any Cluster API internal behaviour when implementing an InfraMachine resource.

Instead, whenever you need something more from the Cluster API contract, you MUST engage the community.

The Cluster API maintainers welcome feedback and contributions to the contract in order to improve how it's defined,
its clarity and visibility to provider implementers and its suitability across the different kinds of Cluster API providers.

To provide feedback or open a discussion about the provider contract please [open an issue on the Cluster API](https://github.com/kubernetes-sigs/cluster-api/issues/new?assignees=&labels=&template=feature_request.md)
repo or add an item to the agenda in the [Cluster API community meeting](https://git.k8s.io/community/sig-cluster-lifecycle/README.md#cluster-api).

</aside>

## Rules (contract version v1beta1)

| Rule                                                                 | Mandatory | Note                                 |
|----------------------------------------------------------------------|-----------|--------------------------------------|
| [All resources: scope]                                               | Yes       |                                      |
| [All resources: `TypeMeta` and `ObjectMeta`field]                    | Yes       |                                      |
| [All resources: `APIVersion` field value]                            | Yes       |                                      |
| [InfraMachine, InfraMachineList resource definition]                 | Yes       |                                      |
| [InfraMachine: provider ID]                                          | Yes       |                                      |
| [InfraMachine: failure domain]                                       | No        |                                      |
| [InfraMachine: addresses]                                            | No        |                                      |
| [InfraMachine: initialization completed]                             | Yes       |                                      |
| [InfraMachine: conditions]                                           | No        |                                      |
| [InfraMachine: terminal failures]                                    | No        |                                      |
| [InfraMachineTemplate, InfraMachineTemplateList resource definition] | Yes       |                                      |
| [InfraMachineTemplate: support for SSA dry run]                      | No        | Mandatory for ClusterClasses support |
| [Multi tenancy]                                                      | No        | Mandatory for clusterctl CLI support |
| [Clusterctl support]                                                 | No        | Mandatory for clusterctl CLI support |
| [InfraMachine: pausing]                                              | No        |                                      |

Note:
- `All resources` refers to all the provider's resources "core" Cluster API interacts with;
  In the context of this page: `InfraMachine`, `InfraMachineTemplate` and corresponding list types

### All resources: scope

All resources MUST be namespace-scoped.

### All resources: `TypeMeta` and `ObjectMeta` field

All resources MUST have the standard Kubernetes `TypeMeta` and `ObjectMeta` fields.

### All resources: `APIVersion` field value

In Kubernetes `APIVersion` is a combination of API group and version.
Special consideration MUST applies to both API group and version for all the resources Cluster API interacts with.

#### All resources: API group

The domain for Cluster API resources is `cluster.x-k8s.io`, and infrastructure providers under the Kubernetes SIGS org
generally use `infrastructure.cluster.x-k8s.io` as API group.

If your provider uses a different API group, you MUST grant full read/write RBAC permissions for resources in your API group
to the Cluster API core controllers. The canonical way to do so is via a `ClusterRole` resource with the [aggregation label]
`cluster.x-k8s.io/aggregate-to-manager: "true"`.

The following is an example ClusterRole for a `FooMachine` resource in the `infrastructure.foo.com` API group:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
    name: capi-foo-clusters
    labels:
      cluster.x-k8s.io/aggregate-to-manager: "true"
rules:
- apiGroups:
    - infrastructure.foo.com
  resources:
    - foomachines
    - foomachinetemplates
  verbs:
    - create
    - delete
    - get
    - list
    - patch
    - update
    - watch
```

Note: The write permissions are required because Cluster API manages InfraMachines generated from InfraMachineTemplates;
when using ClusterClass and managed topologies, also InfraMachineTemplates are managed directly by Cluster API.

#### All resources: version

The resource Version defines the stability of the API and its backward compatibility guarantees.
Examples include `v1alpha1`, `v1beta1`, `v1`, etc. and are governed by the [Kubernetes API Deprecation Policy].

Your provider SHOULD abide by the same policies.

Note: The version of your provider does not need to be in sync with the version of core Cluster API resources.
Instead, prefer choosing a version that matches the stability of the provider API and its backward compatibility guarantees.

Additionally:

Providers MUST set `cluster.x-k8s.io/<version>` label on the InfraMachine Custom Resource Definitions.

The label is a map from a Cluster API contract version to your Custom Resource Definition versions.
The value is an underscore-delimited (_) list of versions. Each value MUST point to an available version in your CRD Spec.

The label allows Cluster API controllers to perform automatic conversions for object references, the controllers will pick
the last available version in the list if multiple versions are found.

To apply the label to CRDs it’s possible to use commonLabels in your `kustomize.yaml` file, usually in `config/crd`:

```yaml
commonLabels:
  cluster.x-k8s.io/v1alpha2: v1alpha1
  cluster.x-k8s.io/v1alpha3: v1alpha2
  cluster.x-k8s.io/v1beta1: v1beta1
```

An example of this is in the [Kubeadm Bootstrap provider](https://github.com/kubernetes-sigs/cluster-api/blob/release-1.1/controlplane/kubeadm/config/crd/kustomization.yaml).

### InfraMachine, InfraMachineList resource definition

You MUST define a InfraMachine resource.
The InfraMachine resource name must have the format produced by `sigs.k8s.io/cluster-api/util/contract.CalculateCRDName(Group, Kind)`.

Note: Cluster API is using such a naming convention to avoid an expensive CRD lookup operation when looking for labels from
the CRD definition of the InfraMachine resource.

It is a generally applied convention to use names in the format `${env}Machine`, where ${env} is a, possibly short, name
for the environment in question. For example `GCPMachine` is an implementation for the Google Cloud Platform, and `AWSMachine`
is one for Amazon Web Services.

```go
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=foomachines,shortName=foom,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// FooMachine is the Schema for foomachines.
type FooMachine struct {
    metav1.TypeMeta `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec FooMachineSpec `json:"spec,omitempty"`
    Status FooMachineStatus `json:"status,omitempty"`
}

type FooMachineSpec struct {
    // See other rules for more details about mandatory/optional fields in InfraMachine spec.
    // Other fields SHOULD be added based on the needs of your provider.
}

type FooMachineStatus struct {
    // See other rules for more details about mandatory/optional fields in InfraMachine status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

For each InfraMachine resource, you MUST also add the corresponding list resource.
The list resource MUST be named as `<InfraMachine>List`.

```go
// +kubebuilder:object:root=true

// FooMachineList contains a list of foomachines.
type FooMachineList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []FooMachine `json:"items"`
}
```

### InfraMachine: provider ID

Each Machine needs a provider ID to identify the Kubernetes Node that runs on the machine. 
Node's Provider id  MUST surface on `spec.providerID` in the InfraMachine resource.

```go
type FooMachineSpec struct {
    // providerID must match the provider ID as seen on the node object corresponding to this machine.
	// For Kubernetes Nodes running on the Foo provider, this value is set by the corresponding CPI component 
	// and it has the format docker:////<vm-name>. 
    // +optional
    ProviderID *string `json:"providerID,omitempty"`
    
    // See other rules for more details about mandatory/optional fields in InfraMachine spec.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

Once `spec.providerID` is set on the InfraMachine resource and the [InfraMachine initialization completed],
the Cluster controller will surface this info in Machine's `spec.providerID`.

### InfraMachine: failure domain

In case you are developing an infrastructure provider which has a notion of failure domains where machines should be
placed in, the InfraMachine resource MUST comply to the value that exists in the `spec.failureDomain` field of the Machine
(in other words, the InfraMachine MUST be placed in the failure domain specified at Machine level).

Please note, that for allowing a transparent transition from when there was no failure domain support in Cluster API
and InfraMachine was authoritative WRT to failure domain placement (before CAPI v0.3.0),
Cluster API still supports a _deprecated_ reverse process for failure domain management.

In the _deprecated_ reverse process, the failure domain where the machine should be placed is defined in the InfraMachine's 
`spec.failureDomain` field; the value of this field is then surfaced on the corresponding field at Machine level.

<aside class="note warning">

<h1>Heads up! this will change with the v1beta2 contract</h1>

Machine's controller will stop supporting the _deprecated_ reverse process; the InfraMachine's `spec.failureDomain`,
if still present, will be ignored.

However, InfraMachine will be allowed to surface the failure domain where the machine is actually placed in by 
implementing a new, optional `status.failureDomain`; this info, if present, will then surface at Machine level in a 
new corresponding field (also in status).

```go
type FooMachineStatus struct {
    // failureDomain is the unique identifier of the failure domain where this Machine has been placed in.
    // For this Foo infrastructure provider, the name is equivalent to the name of one of the available regions.
    FailureDomain *string `json:"failureDomain,omitempty"`

    // See other rules for more details about mandatory/optional fields in InfraMachineStatus.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

</aside>

### InfraMachine: addresses

Infrastructure provider have the opportunity to surface machines addresses on the InfraMachine resource; this information
won't be used by core Cluster API controller, but it is really useful for operator troubleshooting issues on machines.

In case you want to surface machine's addresses, you MUST surface them in `status.addresses` in the InfraMachine resource.

```go
type FooMachineStatus struct {
    // addresses contains the associated addresses for the machine.
    // +optional
    Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

    // See other rules for more details about mandatory/optional fields in InfraMachine status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

Each MachineAddress must have a type; accepted types are `Hostname`, `ExternalIP`, `InternalIP`, `ExternalDNS` or `InternalDNS`.

Once `status.addresses` is set on the InfraMachine resource and the [InfraMachine initialization completed],
the Machine controller will surface this info in Machine's `status.addresses`.

### InfraMachine: initialization completed

Each InfraMachine MUST report when Machine's infrastructure is fully provisioned (initialization) by setting
`status.ready` in the InfraMachine resource.

```go
type FooMachineStatus struct {
    // ready denotes that the foo machine infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed. Please use conditions
	// to check the operational state of the infra machine.
    // +optional
    Ready bool `json:"ready"`
    
    // See other rules for more details about mandatory/optional fields in InfraMachine status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

Once `status.ready` the Machine "core" controller will bubble up this info in Machine's `status.infrastructureReady`;
Also InfraMachine's `spec.providerID` and `status.addresses` will be surfaced on Machine's
corresponding fields at the same time.

<aside class="note warning">

<h1>Heads up! this will change with the v1beta2 contract</h1>

When the v1beta2 contract will be released (tentative Apr 2025), `status.initialization.provisioned` will be used
instead of `status.ready`. However, `status.ready` will be supported until v1beta1 removal (~one year later).

See [Improving status in CAPI resources].

</aside>

### InfraMachine: conditions

According to [Kubernetes API Conventions], Conditions provide a standard mechanism for higher-level
status reporting from a controller.

Providers implementers SHOULD implement `status.conditions` for their InfraMachine resource.
In case conditions are implemented, Cluster API condition type MUST be used.

If a condition with type `Ready` exist, such condition will be mirrored in Machine's `InfrastructureReady` condition.

Please note that the `Ready` condition is expected to surface the status of the InfraMachine during its own entire lifecycle,
including initial provisioning, the final deletion process, and the period in between these two moments.

See [Cluster API condition proposal] for more context.

<aside class="note warning">

<h1>Heads up! this will change with the v1beta2 contract</h1>

When the v1beta2 contract will be released (tentative Apr 2025), Cluster API will start using Kubernetes metav1.Condition
types and fully comply to [Kubernetes API Conventions].

In order to support providers continuing to use legacy Cluster API condition types, providers transitioning to
metav1.Condition or even providers adopting custom condition types, Cluster API will start to accept `Ready` condition that
provides following information:
- `type`
- `status`
- `reason` ((optional, if omitted, a default one will be used)
- `message` (optional)
- `lastTransitionTime` (optional, if omitted, time.Now will be used)

Other fields will be ignored

See [Improving status in CAPI resources] for more context.

Please note that provider that will continue to use legacy Cluster API condition types MUST carefully take into account
the implication of this choice which are described both in the document above and in the notice at the beginning of the [Cluster API condition proposal]..

</aside>

### InfraMachine: terminal failures

Each InfraMachine SHOULD report when Machine's enter in a state that cannot be recovered (terminal failure) by
setting `status.failureReason` and `status.failureMessage` in the InfraMachine resource.

```go
type FooMachineStatus struct {
    // failureReason will be set in the event that there is a terminal problem reconciling the FooMachine 
    // and will contain a succinct value suitable for machine interpretation.
    //
    // This field should not be set for transitive errors that can be fixed automatically or with manual intervention,
    // but instead indicate that something is fundamentally wrong with the FooMachine and that it cannot be recovered.
    // +optional
    FailureReason *capierrors.ClusterStatusError `json:"failureReason,omitempty"`
    
    // failureMessage will be set in the event that there is a terminal problem reconciling the FooMachine
    // and will contain a more verbose string suitable for logging and human consumption.
    //
    // This field should not be set for transitive errors that can be fixed automatically or with manual intervention,
    // but instead indicate that something is fundamentally wrong with the FooMachine and that it cannot be recovered.
    // +optional
    FailureMessage *string `json:"failureMessage,omitempty"`
    
    // See other rules for more details about mandatory/optional fields in InfraMachine status.
    // Other fields SHOULD be added based on the needs of your provider.
}
```

Once `status.failureReason` and `status.failureMessage` are set on the InfraMachine resource, the Machine "core" controller
will surface those info in the corresponding fields in Machine's `status`.

Please note that once failureReason/failureMessage is set in Machine's `status`, the only way to recover is to delete and
recreate the Machine (it is a terminal failure).

<aside class="note warning">

<h1>Heads up! this will change with the v1beta2 contract</h1>

When the v1beta2 contract will be released (tentative Apr 2025), support for `status.failureReason` and `status.failureMessage`
will be dropped.

See [Improving status in CAPI resources].

</aside>

### InfraMachineTemplate, InfraMachineTemplateList resource definition

For a given InfraMachine resource, you MUST also add a corresponding InfraMachineTemplate resources in order to use it
when defining set of machines, e.g. MachineDeployments.

The template resource MUST be named as `<InfraMachine>Template`.

```go
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=foomachinetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion

// FooMachineTemplate is the Schema for the foomachinetemplates API.
type FooMachineTemplate struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec FooMachineTemplateSpec `json:"spec,omitempty"`
}

type FooMachineTemplateSpec struct {
    Template FooMachineTemplateResource `json:"template"`
}

type FooMachineTemplateResource struct {
    // Standard object's metadata.
    // More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
    // +optional
    ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty"`
    Spec FooMachineSpec `json:"spec"`
}
```

NOTE: in this example InfraMachineTemplate's `spec.template.spec` embeds `FooMachineSpec` from InfraMachine. This might not always be
the best choice depending of if/how InfraMachine's spec fields applies to many machines vs only one.

For each InfraMachineTemplate resource, you MUST also add the corresponding list resource.
The list resource MUST be named as `<InfraMachineTemplate>List`.

```go
// +kubebuilder:object:root=true

// FooMachineTemplateList contains a list of FooMachineTemplates.
type FooMachineTemplateList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []FooMachineTemplate `json:"items"`
}
```

### InfraMachineTemplate: support for SSA dry run

When Cluster API's topology controller is trying to identify differences between templates defined in a ClusterClass and
the current Cluster topology, it is required to run [Server Side Apply] (SSA) dry run call.

However, in case you immutability checks for your InfraMachineTemplate, this can lead the SSA dry run call to errors.

In order to avoid this InfraMachineTemplate MUST specifically implement support for SSA dry run calls from the topology controller. 

The implementation requires to use controller runtime's `CustomValidator`, available in CR versions >= v0.12.3.

This will allow to skip the immutability check only when the topology controller is dry running while preserving the
validation behavior for all other cases.

See [the DockerMachineTemplate webhook] as a reference for a compatible implementation.

### Multi tenancy

Multi tenancy in Cluster API defines the capability of an infrastructure provider to manage different credentials,
each one of them corresponding to an infrastructure tenant.

See [infrastructure Provider Security Guidance] for considerations about cloud provider credential management.

Please also note that Cluster API does not support running multiples instances of the same provider, which someone can
assume an alternative solution to implement multi tenancy; same applies to the clusterctl CLI.

See [Support running multiple instances of the same provider] for more context.

However, if you want to make it possible for users to run multiples instances of your provider, your controller's SHOULD:

- support the `--namespace` flag.
- support the `--watch-filter` flag.

Please, read carefully the page linked above to fully understand implications and risks related to this option.

### Clusterctl support

The clusterctl command is designed to work with all the providers compliant with the rules defined in the [clusterctl provider contract].

### InfraMachine: pausing

Providers SHOULD implement the pause behaviour for every object with a reconciliation loop. This is done by checking if `spec.paused` is set on the Cluster object and by checking for the `cluster.x-k8s.io/paused` annotation on the InfraMachine object.

If implementing the pause behavior, providers SHOULD surface the paused status of an object using the Paused condition: `Status.Conditions[Paused]`.

## Typical InfraMachine reconciliation workflow

A machine infrastructure provider must respond to changes to its InfraMachine resources. This process is
typically called reconciliation. The provider must watch for new, updated, and deleted resources and respond
accordingly.

As a reference you can look at the following workflow to understand how the typical reconciliation workflow
is implemented in InfraMachine controllers:

![Machine infrastructure provider activity diagram](../../../images/machine-infra-provider.png)

### Normal resource

1. If the resource does not have a `Machine` owner, exit the reconciliation
    1. The Cluster API `Machine` reconciler populates this based on the value in the `Machines`'s
       `spec.infrastructureRef` field
1. If the resource has `status.failureReason` or `status.failureMessage` set, exit the reconciliation
1. If the `Cluster` to which this resource belongs cannot be found, exit the reconciliation
1. Add the provider-specific finalizer, if needed
1. If the associated `Cluster`'s `status.infrastructureReady` is `false`, exit the reconciliation
    1. **Note**: This check should not be blocking any further delete reconciliation flows.
    1. **Note**: This check should only be performed after appropriate owner references (if any) are updated.
1. If the associated `Machine`'s `spec.bootstrap.dataSecretName` is `nil`, exit the reconciliation
1. Reconcile provider-specific machine infrastructure
    1. If any errors are encountered:
        1. If they are terminal failures, set `status.failureReason` and `status.failureMessage`
        1. Exit the reconciliation
    1. If this is a control plane machine, register the instance with the provider's control plane load balancer
       (optional)
1. Set `spec.providerID` to the provider-specific identifier for the provider's machine instance
1. Set `status.ready` to `true`
1. Set `status.addresses` to the provider-specific set of instance addresses (optional)
1. Set `spec.failureDomain` to the provider-specific failure domain the instance is running in (optional)
1. Patch the resource to persist changes

### Deleted resource

1. If the resource has a `Machine` owner
    1. Perform deletion of provider-specific machine infrastructure
    1. If this is a control plane machine, deregister the instance from the provider's control plane load balancer
       (optional)
    1. If any errors are encountered, exit the reconciliation
1. Remove the provider-specific finalizer from the resource
1. Patch the resource to persist changes

[All resources: Scope]: #all-resources-scope
[All resources: `TypeMeta` and `ObjectMeta`field]: #all-resources-typemeta-and-objectmeta-field
[All resources: `APIVersion` field value]: #all-resources-apiversion-field-value
[aggregation label]: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#aggregated-clusterroles
[Kubernetes API Deprecation Policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/
[InfraMachine, InfraMachineList resource definition]: #inframachine-inframachinelist-resource-definition
[InfraMachine: provider ID]: #inframachine-provider-id
[InfraMachine: failure domain]: #inframachine-failure-domain
[InfraMachine: addresses]: #inframachine-addresses
[InfraMachine: initialization completed]: #inframachine-initialization-completed
[Improving status in CAPI resources]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md
[InfraMachine: conditions]: #inframachine-conditions
[Kubernetes API Conventions]: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
[Cluster API condition proposal]: https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20200506-conditions.md
[InfraMachine: terminal failures]: #inframachine-terminal-failures
[InfraMachineTemplate, InfraMachineTemplateList resource definition]: #inframachinetemplate-inframachinetemplatelist-resource-definition
[InfraMachineTemplate: support for SSA dry run]: #inframachinetemplate-support-for-ssa-dry-run
[Multi tenancy]: #multi-tenancy
[Support running multiple instances of the same provider]: ../../core/support-multiple-instances.md
[Clusterctl support]: #clusterctl-support
[clusterctl provider contract]: clusterctl.md
[implementation best practices]: ../best-practices.md
[infrastructure Provider Security Guidance]: ../security-guidelines.md
[Server Side Apply]: https://kubernetes.io/docs/reference/using-api/server-side-apply/
[the DockerMachineTemplate webhook]: https://github.com/kubernetes-sigs/cluster-api/blob/main/test/infrastructure/docker/internal/webhooks/dockermachinetemplate_webhook.go
[InfraMachine: pausing] #inframachine-pausing
