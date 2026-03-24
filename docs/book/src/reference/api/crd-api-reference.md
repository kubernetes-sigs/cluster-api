# API Reference

## Packages
- [addons.cluster.x-k8s.io/v1beta1](#addonsclusterx-k8siov1beta1)
- [addons.cluster.x-k8s.io/v1beta2](#addonsclusterx-k8siov1beta2)
- [bootstrap.cluster.x-k8s.io/v1beta1](#bootstrapclusterx-k8siov1beta1)
- [bootstrap.cluster.x-k8s.io/v1beta2](#bootstrapclusterx-k8siov1beta2)
- [cluster.x-k8s.io/v1beta1](#clusterx-k8siov1beta1)
- [cluster.x-k8s.io/v1beta2](#clusterx-k8siov1beta2)
- [controlplane.cluster.x-k8s.io/v1beta1](#controlplaneclusterx-k8siov1beta1)
- [controlplane.cluster.x-k8s.io/v1beta2](#controlplaneclusterx-k8siov1beta2)
- [ipam.cluster.x-k8s.io/v1alpha1](#ipamclusterx-k8siov1alpha1)
- [ipam.cluster.x-k8s.io/v1beta1](#ipamclusterx-k8siov1beta1)
- [ipam.cluster.x-k8s.io/v1beta2](#ipamclusterx-k8siov1beta2)
- [runtime.cluster.x-k8s.io/v1alpha1](#runtimeclusterx-k8siov1alpha1)
- [runtime.cluster.x-k8s.io/v1beta2](#runtimeclusterx-k8siov1beta2)


## addons.cluster.x-k8s.io/v1beta1

Package v1beta1 contains API Schema definitions for the addons v1beta1 API group

Deprecated: This package is deprecated and is going to be removed when support for v1beta1 will be dropped.

### Resource Types
- [ClusterResourceSet](#clusterresourceset)
- [ClusterResourceSetBinding](#clusterresourcesetbinding)



#### ClusterResourceSet



ClusterResourceSet is the Schema for the clusterresourcesets API.
For advanced use cases an add-on provider should be used instead.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `addons.cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `ClusterResourceSet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[ClusterResourceSetSpec](#clusterresourcesetspec)_ | spec is the desired state of ClusterResourceSet. |  | Optional: \{\} <br /> |


#### ClusterResourceSetBinding



ClusterResourceSetBinding lists all matching ClusterResourceSets with the cluster it belongs to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `addons.cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `ClusterResourceSetBinding` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[ClusterResourceSetBindingSpec](#clusterresourcesetbindingspec)_ | spec is the desired state of ClusterResourceSetBinding. |  | Optional: \{\} <br /> |


#### ClusterResourceSetBindingSpec



ClusterResourceSetBindingSpec defines the desired state of ClusterResourceSetBinding.



_Appears in:_
- [ClusterResourceSetBinding](#clusterresourcesetbinding)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bindings` _[ResourceSetBinding](#resourcesetbinding) array_ | bindings is a list of ClusterResourceSets and their resources. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `clusterName` _string_ | clusterName is the name of the Cluster this binding applies to.<br />Note: this field mandatory in v1beta2. |  | MaxLength: 63 <br />MinLength: 1 <br />Optional: \{\} <br /> |




#### ClusterResourceSetSpec



ClusterResourceSetSpec defines the desired state of ClusterResourceSet.



_Appears in:_
- [ClusterResourceSet](#clusterresourceset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | clusterSelector is the label selector for Clusters. The Clusters that are<br />selected by this will be the ones affected by this ClusterResourceSet.<br />It must match the Cluster labels. This field is immutable.<br />Label selector cannot be empty. |  | Required: \{\} <br /> |
| `resources` _[ResourceRef](#resourceref) array_ | resources is a list of Secrets/ConfigMaps where each contains 1 or more resources to be applied to remote clusters. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `strategy` _string_ | strategy is the strategy to be used during applying resources. Defaults to ApplyOnce. This field is immutable. |  | Enum: [ApplyOnce Reconcile] <br />Optional: \{\} <br /> |




#### ResourceBinding



ResourceBinding shows the status of a resource that belongs to a ClusterResourceSet matched by the owner cluster of the ClusterResourceSetBinding object.



_Appears in:_
- [ResourceSetBinding](#resourcesetbinding)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the resource that is in the same namespace with ClusterResourceSet object. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `kind` _string_ | kind of the resource. Supported kinds are: Secrets and ConfigMaps. |  | Enum: [Secret ConfigMap] <br />Required: \{\} <br /> |
| `hash` _string_ | hash is the hash of a resource's data. This can be used to decide if a resource is changed.<br />For "ApplyOnce" ClusterResourceSet.spec.strategy, this is no-op as that strategy does not act on change. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `applied` _boolean_ | applied is to track if a resource is applied to the cluster or not. |  | Required: \{\} <br /> |


#### ResourceRef



ResourceRef specifies a resource.



_Appears in:_
- [ClusterResourceSetSpec](#clusterresourcesetspec)
- [ResourceBinding](#resourcebinding)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the resource that is in the same namespace with ClusterResourceSet object. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `kind` _string_ | kind of the resource. Supported kinds are: Secrets and ConfigMaps. |  | Enum: [Secret ConfigMap] <br />Required: \{\} <br /> |


#### ResourceSetBinding



ResourceSetBinding keeps info on all of the resources in a ClusterResourceSet.



_Appears in:_
- [ClusterResourceSetBindingSpec](#clusterresourcesetbindingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterResourceSetName` _string_ | clusterResourceSetName is the name of the ClusterResourceSet that is applied to the owner cluster of the binding. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `resources` _[ResourceBinding](#resourcebinding) array_ | resources is a list of resources that the ClusterResourceSet has. |  | MaxItems: 100 <br />Optional: \{\} <br /> |



## addons.cluster.x-k8s.io/v1beta2

Package v1beta2 contains API Schema definitions for the addons v1beta2 API group.

### Resource Types
- [ClusterResourceSet](#clusterresourceset)
- [ClusterResourceSetBinding](#clusterresourcesetbinding)



#### ClusterResourceSet



ClusterResourceSet is the Schema for the clusterresourcesets API.
For advanced use cases an add-on provider should be used instead.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `addons.cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `ClusterResourceSet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[ClusterResourceSetSpec](#clusterresourcesetspec)_ | spec is the desired state of ClusterResourceSet. |  | Required: \{\} <br /> |


#### ClusterResourceSetBinding



ClusterResourceSetBinding lists all matching ClusterResourceSets with the cluster it belongs to.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `addons.cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `ClusterResourceSetBinding` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[ClusterResourceSetBindingSpec](#clusterresourcesetbindingspec)_ | spec is the desired state of ClusterResourceSetBinding. |  | Required: \{\} <br /> |


#### ClusterResourceSetBindingSpec



ClusterResourceSetBindingSpec defines the desired state of ClusterResourceSetBinding.



_Appears in:_
- [ClusterResourceSetBinding](#clusterresourcesetbinding)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bindings` _[ResourceSetBinding](#resourcesetbinding) array_ | bindings is a list of ClusterResourceSets and their resources. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `clusterName` _string_ | clusterName is the name of the Cluster this binding applies to. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |




#### ClusterResourceSetSpec



ClusterResourceSetSpec defines the desired state of ClusterResourceSet.



_Appears in:_
- [ClusterResourceSet](#clusterresourceset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | clusterSelector is the label selector for Clusters. The Clusters that are<br />selected by this will be the ones affected by this ClusterResourceSet.<br />It must match the Cluster labels. This field is immutable.<br />Label selector cannot be empty. |  | Required: \{\} <br /> |
| `resources` _[ResourceRef](#resourceref) array_ | resources is a list of Secrets/ConfigMaps where each contains 1 or more resources to be applied to remote clusters. |  | MaxItems: 100 <br />MinItems: 1 <br />Required: \{\} <br /> |
| `strategy` _string_ | strategy is the strategy to be used during applying resources. Defaults to ApplyOnce. This field is immutable. |  | Enum: [ApplyOnce Reconcile] <br />Optional: \{\} <br /> |




#### ResourceBinding



ResourceBinding shows the status of a resource that belongs to a ClusterResourceSet matched by the owner cluster of the ClusterResourceSetBinding object.



_Appears in:_
- [ResourceSetBinding](#resourcesetbinding)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the resource that is in the same namespace with ClusterResourceSet object. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `kind` _string_ | kind of the resource. Supported kinds are: Secrets and ConfigMaps. |  | Enum: [Secret ConfigMap] <br />Required: \{\} <br /> |
| `hash` _string_ | hash is the hash of a resource's data. This can be used to decide if a resource is changed.<br />For "ApplyOnce" ClusterResourceSet.spec.strategy, this is no-op as that strategy does not act on change. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `applied` _boolean_ | applied is to track if a resource is applied to the cluster or not. |  | Required: \{\} <br /> |


#### ResourceRef



ResourceRef specifies a resource.



_Appears in:_
- [ClusterResourceSetSpec](#clusterresourcesetspec)
- [ResourceBinding](#resourcebinding)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the resource that is in the same namespace with ClusterResourceSet object. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `kind` _string_ | kind of the resource. Supported kinds are: Secrets and ConfigMaps. |  | Enum: [Secret ConfigMap] <br />Required: \{\} <br /> |


#### ResourceSetBinding



ResourceSetBinding keeps info on all of the resources in a ClusterResourceSet.



_Appears in:_
- [ClusterResourceSetBindingSpec](#clusterresourcesetbindingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterResourceSetName` _string_ | clusterResourceSetName is the name of the ClusterResourceSet that is applied to the owner cluster of the binding. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `resources` _[ResourceBinding](#resourcebinding) array_ | resources is a list of resources that the ClusterResourceSet has. |  | MaxItems: 100 <br />Optional: \{\} <br /> |



## bootstrap.cluster.x-k8s.io/v1beta1

Package v1beta1 contains API Schema definitions for the kubeadm v1beta1 API group.

Deprecated: This package is deprecated and is going to be removed when support for v1beta1 will be dropped.

### Resource Types
- [KubeadmConfig](#kubeadmconfig)
- [KubeadmConfigTemplate](#kubeadmconfigtemplate)



#### APIEndpoint



APIEndpoint struct contains elements of API server instance deployed on a node.



_Appears in:_
- [InitConfiguration](#initconfiguration)
- [JoinControlPlane](#joincontrolplane)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `advertiseAddress` _string_ | advertiseAddress sets the IP address for the API server to advertise. |  | MaxLength: 39 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `bindPort` _integer_ | bindPort sets the secure port for the API Server to bind to.<br />Defaults to 6443. |  | Optional: \{\} <br /> |


#### APIServer



APIServer holds settings necessary for API server deployments in the cluster.



_Appears in:_
- [ClusterConfiguration](#clusterconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `extraArgs` _object (keys:string, values:string)_ | extraArgs is an extra set of flags to pass to the control plane component. |  | Optional: \{\} <br /> |
| `extraVolumes` _[HostPathMount](#hostpathmount) array_ | extraVolumes is an extra set of host volumes, mounted to the control plane component. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `extraEnvs` _[EnvVar](#envvar) array_ | extraEnvs is an extra set of environment variables to pass to the control plane component.<br />Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.<br />This option takes effect only on Kubernetes >=1.31.0. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `certSANs` _string array_ | certSANs sets extra Subject Alternative Names for the API Server signing cert. |  | MaxItems: 100 <br />items:MaxLength: 253 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `timeoutForControlPlane` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | timeoutForControlPlane controls the timeout that we use for API server to appear |  | Optional: \{\} <br /> |


#### BootstrapToken



BootstrapToken describes one bootstrap token, stored as a Secret in the cluster.



_Appears in:_
- [InitConfiguration](#initconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `token` _[BootstrapTokenString](#bootstraptokenstring)_ | token is used for establishing bidirectional trust between nodes and control-planes.<br />Used for joining nodes in the cluster. |  | Type: string <br />Required: \{\} <br /> |
| `description` _string_ | description sets a human-friendly message why this token exists and what it's used<br />for, so other administrators can know its purpose. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `ttl` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | ttl defines the time to live for this token. Defaults to 24h.<br />Expires and TTL are mutually exclusive. |  | Optional: \{\} <br /> |
| `usages` _string array_ | usages describes the ways in which this token can be used. Can by default be used<br />for establishing bidirectional trust, but that can be changed here. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `groups` _string array_ | groups specifies the extra groups that this token will authenticate as when/if<br />used for authentication |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### BootstrapTokenDiscovery



BootstrapTokenDiscovery is used to set the options for bootstrap token based discovery.



_Appears in:_
- [Discovery](#discovery)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `token` _string_ | token is a token used to validate cluster information<br />fetched from the control-plane. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `apiServerEndpoint` _string_ | apiServerEndpoint is an IP or domain name to the API server from which info will be fetched. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `caCertHashes` _string array_ | caCertHashes specifies a set of public key pins to verify<br />when token-based discovery is used. The root CA found during discovery<br />must match one of these values. Specifying an empty set disables root CA<br />pinning, which can be unsafe. Each hash is specified as "<type>:<value>",<br />where the only currently supported type is "sha256". This is a hex-encoded<br />SHA-256 hash of the Subject Public Key Info (SPKI) object in DER-encoded<br />ASN.1. These hashes can be calculated using, for example, OpenSSL:<br />openssl x509 -pubkey -in ca.crt openssl rsa -pubin -outform der 2>&/dev/null \| openssl dgst -sha256 -hex |  | MaxItems: 100 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `unsafeSkipCAVerification` _boolean_ | unsafeSkipCAVerification allows token-based discovery<br />without CA verification via CACertHashes. This can weaken<br />the security of kubeadm since other nodes can impersonate the control-plane. |  | Optional: \{\} <br /> |


#### BootstrapTokenString



BootstrapTokenString is a token of the format abcdef.abcdef0123456789 that is used
for both validation of the practically of the API server from a joining node's point
of view and as an authentication method for the node in the bootstrap phase of
"kubeadm join". This token is and should be short-lived.

_Validation:_
- Type: string

_Appears in:_
- [BootstrapToken](#bootstraptoken)



#### ClusterConfiguration



ClusterConfiguration contains cluster-wide configuration for a kubeadm cluster.



_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `etcd` _[Etcd](#etcd)_ | etcd holds configuration for etcd.<br />NB: This value defaults to a Local (stacked) etcd |  | Optional: \{\} <br /> |
| `networking` _[Networking](#networking)_ | networking holds configuration for the networking topology of the cluster.<br />NB: This value defaults to the Cluster object spec.clusterNetwork. |  | Optional: \{\} <br /> |
| `kubernetesVersion` _string_ | kubernetesVersion is the target version of the control plane.<br />NB: This value defaults to the Machine object spec.version |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `controlPlaneEndpoint` _string_ | controlPlaneEndpoint sets a stable IP address or DNS name for the control plane; it<br />can be a valid IP address or a RFC-1123 DNS subdomain, both with optional TCP port.<br />In case the ControlPlaneEndpoint is not specified, the AdvertiseAddress + BindPort<br />are used; in case the ControlPlaneEndpoint is specified but without a TCP port,<br />the BindPort is used.<br />Possible usages are:<br />e.g. In a cluster with more than one control plane instances, this field should be<br />assigned the address of the external load balancer in front of the<br />control plane instances.<br />e.g.  in environments with enforced node recycling, the ControlPlaneEndpoint<br />could be used for assigning a stable DNS to the control plane.<br />NB: This value defaults to the first value in the Cluster object status.apiEndpoints array. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `apiServer` _[APIServer](#apiserver)_ | apiServer contains extra settings for the API server control plane component |  | Optional: \{\} <br /> |
| `controllerManager` _[ControlPlaneComponent](#controlplanecomponent)_ | controllerManager contains extra settings for the controller manager control plane component |  | Optional: \{\} <br /> |
| `scheduler` _[ControlPlaneComponent](#controlplanecomponent)_ | scheduler contains extra settings for the scheduler control plane component |  | Optional: \{\} <br /> |
| `dns` _[DNS](#dns)_ | dns defines the options for the DNS add-on installed in the cluster. |  | Optional: \{\} <br /> |
| `certificatesDir` _string_ | certificatesDir specifies where to store or look for all required certificates.<br />NB: if not provided, this will default to `/etc/kubernetes/pki` |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `imageRepository` _string_ | imageRepository sets the container registry to pull images from.<br />* If not set, the default registry of kubeadm will be used, i.e.<br />  * registry.k8s.io (new registry): >= v1.22.17, >= v1.23.15, >= v1.24.9, >= v1.25.0<br />  * k8s.gcr.io (old registry): all older versions<br />  Please note that when imageRepository is not set we don't allow upgrades to<br />  versions >= v1.22.0 which use the old registry (k8s.gcr.io). Please use<br />  a newer patch version with the new registry instead (i.e. >= v1.22.17,<br />  >= v1.23.15, >= v1.24.9, >= v1.25.0).<br />* If the version is a CI build (kubernetes version starts with `ci/` or `ci-cross/`)<br /> `gcr.io/k8s-staging-ci-images` will be used as a default for control plane components<br />  and for kube-proxy, while `registry.k8s.io` will be used for all the other images. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `featureGates` _object (keys:string, values:boolean)_ | featureGates enabled by the user. |  | Optional: \{\} <br /> |
| `clusterName` _string_ | clusterName is the cluster name |  | MaxLength: 63 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### ContainerLinuxConfig



ContainerLinuxConfig contains CLC-specific configuration.

We use a structured type here to allow adding additional fields, for example 'version'.



_Appears in:_
- [IgnitionSpec](#ignitionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `additionalConfig` _string_ | additionalConfig contains additional configuration to be merged with the Ignition<br />configuration generated by the bootstrapper controller. More info: https://coreos.github.io/ignition/operator-notes/#config-merging<br />The data format is documented here: https://kinvolk.io/docs/flatcar-container-linux/latest/provisioning/cl-config/ |  | MaxLength: 32768 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `strict` _boolean_ | strict controls if AdditionalConfig should be strictly parsed. If so, warnings are treated as errors. |  | Optional: \{\} <br /> |


#### ControlPlaneComponent



ControlPlaneComponent holds settings common to control plane component of the cluster.



_Appears in:_
- [APIServer](#apiserver)
- [ClusterConfiguration](#clusterconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `extraArgs` _object (keys:string, values:string)_ | extraArgs is an extra set of flags to pass to the control plane component. |  | Optional: \{\} <br /> |
| `extraVolumes` _[HostPathMount](#hostpathmount) array_ | extraVolumes is an extra set of host volumes, mounted to the control plane component. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `extraEnvs` _[EnvVar](#envvar) array_ | extraEnvs is an extra set of environment variables to pass to the control plane component.<br />Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.<br />This option takes effect only on Kubernetes >=1.31.0. |  | MaxItems: 100 <br />Optional: \{\} <br /> |


#### DNS



DNS defines the DNS addon that should be used in the cluster.



_Appears in:_
- [ClusterConfiguration](#clusterconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `imageRepository` _string_ | imageRepository sets the container registry to pull images from.<br />if not set, the ImageRepository defined in ClusterConfiguration will be used instead. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `imageTag` _string_ | imageTag allows to specify a tag for the image.<br />In case this value is set, kubeadm does not change automatically the version of the above components during upgrades. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### Discovery



Discovery specifies the options for the kubelet to use during the TLS Bootstrap process.



_Appears in:_
- [JoinConfiguration](#joinconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bootstrapToken` _[BootstrapTokenDiscovery](#bootstraptokendiscovery)_ | bootstrapToken is used to set the options for bootstrap token based discovery<br />BootstrapToken and File are mutually exclusive |  | Optional: \{\} <br /> |
| `file` _[FileDiscovery](#filediscovery)_ | file is used to specify a file or URL to a kubeconfig file from which to load cluster information<br />BootstrapToken and File are mutually exclusive |  | Optional: \{\} <br /> |
| `tlsBootstrapToken` _string_ | tlsBootstrapToken is a token used for TLS bootstrapping.<br />If .BootstrapToken is set, this field is defaulted to .BootstrapToken.Token, but can be overridden.<br />If .File is set, this field **must be set** in case the KubeConfigFile does not contain any other authentication information |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | timeout modifies the discovery timeout |  | Optional: \{\} <br /> |


#### DiskSetup



DiskSetup defines input for generated disk_setup and fs_setup in cloud-init.



_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `partitions` _[Partition](#partition) array_ | partitions specifies the list of the partitions to setup. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `filesystems` _[Filesystem](#filesystem) array_ | filesystems specifies the list of file systems to setup. |  | MaxItems: 100 <br />Optional: \{\} <br /> |


#### Encoding

_Underlying type:_ _string_

Encoding specifies the cloud-init file encoding.

_Validation:_
- Enum: [base64 gzip gzip+base64]

_Appears in:_
- [File](#file)

| Field | Description |
| --- | --- |
| `base64` | Base64 implies the contents of the file are encoded as base64.<br /> |
| `gzip` | Gzip implies the contents of the file are encoded with gzip.<br /> |
| `gzip+base64` | GzipBase64 implies the contents of the file are first base64 encoded and then gzip encoded.<br /> |


#### EnvVar



EnvVar represents an environment variable present in a Container.



_Appears in:_
- [APIServer](#apiserver)
- [ControlPlaneComponent](#controlplanecomponent)
- [LocalEtcd](#localetcd)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the environment variable.<br />May consist of any printable ASCII characters except '='. |  |  |
| `value` _string_ | Variable references $(VAR_NAME) are expanded<br />using the previously defined environment variables in the container and<br />any service environment variables. If a variable cannot be resolved,<br />the reference in the input string will be unchanged. Double $$ are reduced<br />to a single $, which allows for escaping the $(VAR_NAME) syntax: i.e.<br />"$$(VAR_NAME)" will produce the string literal "$(VAR_NAME)".<br />Escaped references will never be expanded, regardless of whether the variable<br />exists or not.<br />Defaults to "". |  | Optional: \{\} <br /> |
| `valueFrom` _[EnvVarSource](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#envvarsource-v1-core)_ | Source for the environment variable's value. Cannot be used if value is not empty. |  | Optional: \{\} <br /> |


#### Etcd



Etcd contains elements describing Etcd configuration.



_Appears in:_
- [ClusterConfiguration](#clusterconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `local` _[LocalEtcd](#localetcd)_ | local provides configuration knobs for configuring the local etcd instance<br />Local and External are mutually exclusive |  | Optional: \{\} <br /> |
| `external` _[ExternalEtcd](#externaletcd)_ | external describes how to connect to an external etcd cluster<br />Local and External are mutually exclusive |  | Optional: \{\} <br /> |


#### ExternalEtcd



ExternalEtcd describes an external etcd cluster.
Kubeadm has no knowledge of where certificate files live and they must be supplied.



_Appears in:_
- [Etcd](#etcd)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `endpoints` _string array_ | endpoints of etcd members. Required for ExternalEtcd. |  | MaxItems: 50 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Required: \{\} <br /> |
| `caFile` _string_ | caFile is an SSL Certificate Authority file used to secure etcd communication.<br />Required if using a TLS connection. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `certFile` _string_ | certFile is an SSL certification file used to secure etcd communication.<br />Required if using a TLS connection. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `keyFile` _string_ | keyFile is an SSL key file used to secure etcd communication.<br />Required if using a TLS connection. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |


#### File



File defines the input for generating write_files in cloud-init.



_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `path` _string_ | path specifies the full path on disk where to store the file. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `owner` _string_ | owner specifies the ownership of the file, e.g. "root:root". |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `permissions` _string_ | permissions specifies the permissions to assign to the file, e.g. "0640". |  | MaxLength: 16 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `encoding` _[Encoding](#encoding)_ | encoding specifies the encoding of the file contents. |  | Enum: [base64 gzip gzip+base64] <br />Optional: \{\} <br /> |
| `append` _boolean_ | append specifies whether to append Content to existing file if Path exists. |  | Optional: \{\} <br /> |
| `content` _string_ | content is the actual content of the file. |  | MaxLength: 10240 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `contentFrom` _[FileSource](#filesource)_ | contentFrom is a referenced source of content to populate the file. |  | Optional: \{\} <br /> |


#### FileDiscovery



FileDiscovery is used to specify a file or URL to a kubeconfig file from which to load cluster information.



_Appears in:_
- [Discovery](#discovery)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kubeConfigPath` _string_ | kubeConfigPath is used to specify the actual file path or URL to the kubeconfig file from which to load cluster information |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `kubeConfig` _[FileDiscoveryKubeConfig](#filediscoverykubeconfig)_ | kubeConfig is used (optionally) to generate a KubeConfig based on the KubeadmConfig's information.<br />The file is generated at the path specified in KubeConfigPath.<br />Host address (server field) information is automatically populated based on the Cluster's ControlPlaneEndpoint.<br />Certificate Authority (certificate-authority-data field) is gathered from the cluster's CA secret. |  | Optional: \{\} <br /> |


#### FileDiscoveryKubeConfig



FileDiscoveryKubeConfig contains elements describing how to generate the kubeconfig for bootstrapping.



_Appears in:_
- [FileDiscovery](#filediscovery)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `cluster` _[KubeConfigCluster](#kubeconfigcluster)_ | cluster contains information about how to communicate with the kubernetes cluster.<br />By default the following fields are automatically populated:<br />- Server with the Cluster's ControlPlaneEndpoint.<br />- CertificateAuthorityData with the Cluster's CA certificate. |  | Optional: \{\} <br /> |
| `user` _[KubeConfigUser](#kubeconfiguser)_ | user contains information that describes identity information.<br />This is used to tell the kubernetes cluster who you are. |  | Required: \{\} <br /> |


#### FileSource



FileSource is a union of all possible external source types for file data.
Only one field may be populated in any given instance. Developers adding new
sources of data for target systems should add them here.



_Appears in:_
- [File](#file)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secret` _[SecretFileSource](#secretfilesource)_ | secret represents a secret that should populate this file. |  | Required: \{\} <br /> |


#### Filesystem



Filesystem defines the file systems to be created.



_Appears in:_
- [DiskSetup](#disksetup)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `device` _string_ | device specifies the device name |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `filesystem` _string_ | filesystem specifies the file system type. |  | MaxLength: 128 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `label` _string_ | label specifies the file system label to be used. If set to None, no label is used. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `partition` _string_ | partition specifies the partition to use. The valid options are: "auto\|any", "auto", "any", "none", and <NUM>, where NUM is the actual partition number. |  | MaxLength: 128 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `overwrite` _boolean_ | overwrite defines whether or not to overwrite any existing filesystem.<br />If true, any pre-existing file system will be destroyed. Use with Caution. |  | Optional: \{\} <br /> |
| `replaceFS` _string_ | replaceFS is a special directive, used for Microsoft Azure that instructs cloud-init to replace a file system of <FS_TYPE>.<br />NOTE: unless you define a label, this requires the use of the 'any' partition directive. |  | MaxLength: 128 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `extraOpts` _string array_ | extraOpts defined extra options to add to the command for creating the file system. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### Format

_Underlying type:_ _string_

Format specifies the output format of the bootstrap data

_Validation:_
- Enum: [cloud-config ignition]

_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description |
| --- | --- |
| `cloud-config` | CloudConfig make the bootstrap data to be of cloud-config format.<br /> |
| `ignition` | Ignition make the bootstrap data to be of Ignition format.<br /> |


#### HostPathMount



HostPathMount contains elements describing volumes that are mounted from the
host.



_Appears in:_
- [APIServer](#apiserver)
- [ControlPlaneComponent](#controlplanecomponent)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the volume inside the pod template. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `hostPath` _string_ | hostPath is the path in the host that will be mounted inside<br />the pod. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `mountPath` _string_ | mountPath is the path inside the pod where hostPath will be mounted. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `readOnly` _boolean_ | readOnly controls write access to the volume |  | Optional: \{\} <br /> |
| `pathType` _[HostPathType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#hostpathtype-v1-core)_ | pathType is the type of the HostPath. |  | Optional: \{\} <br /> |


#### IgnitionSpec



IgnitionSpec contains Ignition specific configuration.



_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `containerLinuxConfig` _[ContainerLinuxConfig](#containerlinuxconfig)_ | containerLinuxConfig contains CLC specific configuration. |  | Optional: \{\} <br /> |


#### ImageMeta



ImageMeta allows to customize the image used for components that are not
originated from the Kubernetes/Kubernetes release process.



_Appears in:_
- [DNS](#dns)
- [LocalEtcd](#localetcd)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `imageRepository` _string_ | imageRepository sets the container registry to pull images from.<br />if not set, the ImageRepository defined in ClusterConfiguration will be used instead. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `imageTag` _string_ | imageTag allows to specify a tag for the image.<br />In case this value is set, kubeadm does not change automatically the version of the above components during upgrades. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### InitConfiguration



InitConfiguration contains a list of elements that is specific "kubeadm init"-only runtime
information.



_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bootstrapTokens` _[BootstrapToken](#bootstraptoken) array_ | bootstrapTokens is respected at `kubeadm init` time and describes a set of Bootstrap Tokens to create.<br />This information IS NOT uploaded to the kubeadm cluster configmap, partly because of its sensitive nature |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `nodeRegistration` _[NodeRegistrationOptions](#noderegistrationoptions)_ | nodeRegistration holds fields that relate to registering the new control-plane node to the cluster.<br />When used in the context of control plane nodes, NodeRegistration should remain consistent<br />across both InitConfiguration and JoinConfiguration |  | Optional: \{\} <br /> |
| `localAPIEndpoint` _[APIEndpoint](#apiendpoint)_ | localAPIEndpoint represents the endpoint of the API server instance that's deployed on this control plane node<br />In HA setups, this differs from ClusterConfiguration.ControlPlaneEndpoint in the sense that ControlPlaneEndpoint<br />is the global endpoint for the cluster, which then loadbalances the requests to each individual API server. This<br />configuration object lets you customize what IP/DNS name and port the local API server advertises it's accessible<br />on. By default, kubeadm tries to auto-detect the IP of the default interface and use that, but in case that process<br />fails you may set the desired value here. |  | Optional: \{\} <br /> |
| `skipPhases` _string array_ | skipPhases is a list of phases to skip during command execution.<br />The list of phases can be obtained with the "kubeadm init --help" command.<br />This option takes effect only on Kubernetes >=1.22.0. |  | MaxItems: 50 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `patches` _[Patches](#patches)_ | patches contains options related to applying patches to components deployed by kubeadm during<br />"kubeadm init". The minimum kubernetes version needed to support Patches is v1.22 |  | Optional: \{\} <br /> |


#### JoinConfiguration



JoinConfiguration contains elements describing a particular node.



_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeRegistration` _[NodeRegistrationOptions](#noderegistrationoptions)_ | nodeRegistration holds fields that relate to registering the new control-plane node to the cluster.<br />When used in the context of control plane nodes, NodeRegistration should remain consistent<br />across both InitConfiguration and JoinConfiguration |  | Optional: \{\} <br /> |
| `caCertPath` _string_ | caCertPath is the path to the SSL certificate authority used to<br />secure comunications between node and control-plane.<br />Defaults to "/etc/kubernetes/pki/ca.crt". |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `discovery` _[Discovery](#discovery)_ | discovery specifies the options for the kubelet to use during the TLS Bootstrap process |  | Optional: \{\} <br /> |
| `controlPlane` _[JoinControlPlane](#joincontrolplane)_ | controlPlane defines the additional control plane instance to be deployed on the joining node.<br />If nil, no additional control plane instance will be deployed. |  | Optional: \{\} <br /> |
| `skipPhases` _string array_ | skipPhases is a list of phases to skip during command execution.<br />The list of phases can be obtained with the "kubeadm init --help" command.<br />This option takes effect only on Kubernetes >=1.22.0. |  | MaxItems: 50 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `patches` _[Patches](#patches)_ | patches contains options related to applying patches to components deployed by kubeadm during<br />"kubeadm join". The minimum kubernetes version needed to support Patches is v1.22 |  | Optional: \{\} <br /> |


#### JoinControlPlane



JoinControlPlane contains elements describing an additional control plane instance to be deployed on the joining node.



_Appears in:_
- [JoinConfiguration](#joinconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `localAPIEndpoint` _[APIEndpoint](#apiendpoint)_ | localAPIEndpoint represents the endpoint of the API server instance to be deployed on this node. |  | Optional: \{\} <br /> |


#### KubeConfigAuthExec



KubeConfigAuthExec specifies a command to provide client credentials. The command is exec'd
and outputs structured stdout holding credentials.

See the client.authentication.k8s.io API group for specifications of the exact input
and output format.



_Appears in:_
- [KubeConfigUser](#kubeconfiguser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `command` _string_ | command to execute. |  | MaxLength: 1024 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `args` _string array_ | args is the arguments to pass to the command when executing it. |  | MaxItems: 100 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `env` _[KubeConfigAuthExecEnv](#kubeconfigauthexecenv) array_ | env defines additional environment variables to expose to the process. These<br />are unioned with the host's environment, as well as variables client-go uses<br />to pass argument to the plugin. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `apiVersion` _string_ | apiVersion is preferred input version of the ExecInfo. The returned ExecCredentials MUST use<br />the same encoding version as the input.<br />Defaults to client.authentication.k8s.io/v1 if not set. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `provideClusterInfo` _boolean_ | provideClusterInfo determines whether or not to provide cluster information,<br />which could potentially contain very large CA data, to this exec plugin as a<br />part of the KUBERNETES_EXEC_INFO environment variable. By default, it is set<br />to false. Package k8s.io/client-go/tools/auth/exec provides helper methods for<br />reading this environment variable. |  | Optional: \{\} <br /> |


#### KubeConfigAuthExecEnv

_Underlying type:_ _[struct{Name string "json:\"name\""; Value string "json:\"value\""}](#struct{name-string-"json:\"name\"";-value-string-"json:\"value\""})_

KubeConfigAuthExecEnv is used for setting environment variables when executing an exec-based
credential plugin.



_Appears in:_
- [KubeConfigAuthExec](#kubeconfigauthexec)



#### KubeConfigAuthProvider



KubeConfigAuthProvider holds the configuration for a specified auth provider.



_Appears in:_
- [KubeConfigUser](#kubeconfiguser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the name of the authentication plugin. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `config` _object (keys:string, values:string)_ | config holds the parameters for the authentication plugin. |  | Optional: \{\} <br /> |


#### KubeConfigCluster



KubeConfigCluster contains information about how to communicate with a kubernetes cluster.

Adapted from clientcmdv1.Cluster.



_Appears in:_
- [FileDiscoveryKubeConfig](#filediscoverykubeconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `server` _string_ | server is the address of the kubernetes cluster (https://hostname:port).<br />Defaults to https:// + Cluster.Spec.ControlPlaneEndpoint. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `tlsServerName` _string_ | tlsServerName is used to check server certificate. If TLSServerName is empty, the hostname used to contact the server is used. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `insecureSkipTLSVerify` _boolean_ | insecureSkipTLSVerify skips the validity check for the server's certificate. This will make your HTTPS connections insecure. |  | Optional: \{\} <br /> |
| `certificateAuthorityData` _integer array_ | certificateAuthorityData contains PEM-encoded certificate authority certificates.<br />Defaults to the Cluster's CA certificate if empty. |  | MaxLength: 51200 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `proxyURL` _string_ | proxyURL is the URL to the proxy to be used for all requests made by this<br />client. URLs with "http", "https", and "socks5" schemes are supported.  If<br />this configuration is not provided or the empty string, the client<br />attempts to construct a proxy configuration from http_proxy and<br />https_proxy environment variables. If these environment variables are not<br />set, the client does not attempt to proxy requests.<br />socks5 proxying does not currently support spdy streaming endpoints (exec,<br />attach, port forward). |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### KubeConfigUser



KubeConfigUser contains information that describes identity information.
This is used to tell the kubernetes cluster who you are.

Either authProvider or exec must be filled.

Adapted from clientcmdv1.AuthInfo.



_Appears in:_
- [FileDiscoveryKubeConfig](#filediscoverykubeconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `authProvider` _[KubeConfigAuthProvider](#kubeconfigauthprovider)_ | authProvider specifies a custom authentication plugin for the kubernetes cluster. |  | Optional: \{\} <br /> |
| `exec` _[KubeConfigAuthExec](#kubeconfigauthexec)_ | exec specifies a custom exec-based authentication plugin for the kubernetes cluster. |  | Optional: \{\} <br /> |


#### KubeadmConfig



KubeadmConfig is the Schema for the kubeadmconfigs API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `bootstrap.cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `KubeadmConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmConfigSpec](#kubeadmconfigspec)_ | spec is the desired state of KubeadmConfig. |  | Optional: \{\} <br /> |


#### KubeadmConfigSpec



KubeadmConfigSpec defines the desired state of KubeadmConfig.
Either ClusterConfiguration and InitConfiguration should be defined or the JoinConfiguration should be defined.



_Appears in:_
- [KubeadmConfig](#kubeadmconfig)
- [KubeadmConfigTemplateResource](#kubeadmconfigtemplateresource)
- [KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)
- [KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterConfiguration` _[ClusterConfiguration](#clusterconfiguration)_ | clusterConfiguration along with InitConfiguration are the configurations necessary for the init command |  | Optional: \{\} <br /> |
| `initConfiguration` _[InitConfiguration](#initconfiguration)_ | initConfiguration along with ClusterConfiguration are the configurations necessary for the init command |  | Optional: \{\} <br /> |
| `joinConfiguration` _[JoinConfiguration](#joinconfiguration)_ | joinConfiguration is the kubeadm configuration for the join command |  | Optional: \{\} <br /> |
| `files` _[File](#file) array_ | files specifies extra files to be passed to user_data upon creation. |  | MaxItems: 200 <br />Optional: \{\} <br /> |
| `diskSetup` _[DiskSetup](#disksetup)_ | diskSetup specifies options for the creation of partition tables and file systems on devices. |  | Optional: \{\} <br /> |
| `mounts` _[MountPoints](#mountpoints) array_ | mounts specifies a list of mount points to be setup. |  | MaxItems: 100 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `bootCommands` _string array_ | bootCommands specifies extra commands to run very early in the boot process via the cloud-init bootcmd<br />module. bootcmd will run on every boot, 'cloud-init-per' command can be used to make bootcmd run exactly<br />once. This is typically run in the cloud-init.service systemd unit. This has no effect in Ignition. |  | MaxItems: 1000 <br />items:MaxLength: 10240 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `preKubeadmCommands` _string array_ | preKubeadmCommands specifies extra commands to run before kubeadm runs.<br />With cloud-init, this is prepended to the runcmd module configuration, and is typically executed in<br />the cloud-final.service systemd unit. In Ignition, this is prepended to /etc/kubeadm.sh. |  | MaxItems: 1000 <br />items:MaxLength: 10240 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `postKubeadmCommands` _string array_ | postKubeadmCommands specifies extra commands to run after kubeadm runs.<br />With cloud-init, this is appended to the runcmd module configuration, and is typically executed in<br />the cloud-final.service systemd unit. In Ignition, this is appended to /etc/kubeadm.sh. |  | MaxItems: 1000 <br />items:MaxLength: 10240 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `users` _[User](#user) array_ | users specifies extra users to add |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `ntp` _[NTP](#ntp)_ | ntp specifies NTP configuration |  | Optional: \{\} <br /> |
| `format` _[Format](#format)_ | format specifies the output format of the bootstrap data |  | Enum: [cloud-config ignition] <br />Optional: \{\} <br /> |
| `verbosity` _integer_ | verbosity is the number for the kubeadm log level verbosity.<br />It overrides the `--v` flag in kubeadm commands. |  | Optional: \{\} <br /> |
| `useExperimentalRetryJoin` _boolean_ | useExperimentalRetryJoin replaces a basic kubeadm command with a shell<br />script with retries for joins.<br />This is meant to be an experimental temporary workaround on some environments<br />where joins fail due to timing (and other issues). The long term goal is to add retries to<br />kubeadm proper and use that functionality.<br />This will add about 40KB to userdata<br />For more information, refer to https://github.com/kubernetes-sigs/cluster-api/pull/2763#discussion_r397306055.<br />Deprecated: This experimental fix is no longer needed and this field will be removed in a future release.<br />When removing also remove from staticcheck exclude-rules for SA1019 in golangci.yml |  | Optional: \{\} <br /> |
| `ignition` _[IgnitionSpec](#ignitionspec)_ | ignition contains Ignition specific configuration. |  | Optional: \{\} <br /> |


#### KubeadmConfigTemplate



KubeadmConfigTemplate is the Schema for the kubeadmconfigtemplates API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `bootstrap.cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `KubeadmConfigTemplate` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmConfigTemplateSpec](#kubeadmconfigtemplatespec)_ | spec is the desired state of KubeadmConfigTemplate. |  | Optional: \{\} <br /> |


#### KubeadmConfigTemplateResource



KubeadmConfigTemplateResource defines the Template structure.



_Appears in:_
- [KubeadmConfigTemplateSpec](#kubeadmconfigtemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[KubeadmConfigSpec](#kubeadmconfigspec)_ | spec is the desired state of KubeadmConfig. |  | Optional: \{\} <br /> |


#### KubeadmConfigTemplateSpec



KubeadmConfigTemplateSpec defines the desired state of KubeadmConfigTemplate.



_Appears in:_
- [KubeadmConfigTemplate](#kubeadmconfigtemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _[KubeadmConfigTemplateResource](#kubeadmconfigtemplateresource)_ | template defines the desired state of KubeadmConfigTemplate. |  | Required: \{\} <br /> |


#### LocalEtcd



LocalEtcd describes that kubeadm should run an etcd cluster locally.



_Appears in:_
- [Etcd](#etcd)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `imageRepository` _string_ | imageRepository sets the container registry to pull images from.<br />if not set, the ImageRepository defined in ClusterConfiguration will be used instead. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `imageTag` _string_ | imageTag allows to specify a tag for the image.<br />In case this value is set, kubeadm does not change automatically the version of the above components during upgrades. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `dataDir` _string_ | dataDir is the directory etcd will place its data.<br />Defaults to "/var/lib/etcd". |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `extraArgs` _object (keys:string, values:string)_ | extraArgs are extra arguments provided to the etcd binary<br />when run inside a static pod. |  | Optional: \{\} <br /> |
| `extraEnvs` _[EnvVar](#envvar) array_ | extraEnvs is an extra set of environment variables to pass to the control plane component.<br />Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.<br />This option takes effect only on Kubernetes >=1.31.0. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `serverCertSANs` _string array_ | serverCertSANs sets extra Subject Alternative Names for the etcd server signing cert. |  | MaxItems: 100 <br />items:MaxLength: 253 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `peerCertSANs` _string array_ | peerCertSANs sets extra Subject Alternative Names for the etcd peer signing cert. |  | MaxItems: 100 <br />items:MaxLength: 253 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### MountPoints

_Underlying type:_ _string array_

MountPoints defines input for generated mounts in cloud-init.

_Validation:_
- items:MaxLength: 512
- items:MinLength: 1

_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)



#### NTP



NTP defines input for generated ntp in cloud-init.



_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `servers` _string array_ | servers specifies which NTP servers to use |  | MaxItems: 100 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `enabled` _boolean_ | enabled specifies whether NTP should be enabled |  | Optional: \{\} <br /> |


#### Networking



Networking contains elements describing cluster's networking configuration.



_Appears in:_
- [ClusterConfiguration](#clusterconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serviceSubnet` _string_ | serviceSubnet is the subnet used by k8s services.<br />Defaults to a comma-delimited string of the Cluster object's spec.clusterNetwork.pods.cidrBlocks, or<br />to "10.96.0.0/12" if that's unset. |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `podSubnet` _string_ | podSubnet is the subnet used by pods.<br />If unset, the API server will not allocate CIDR ranges for every node.<br />Defaults to a comma-delimited string of the Cluster object's spec.clusterNetwork.services.cidrBlocks if that is set |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `dnsDomain` _string_ | dnsDomain is the dns domain used by k8s services. Defaults to "cluster.local". |  | MaxLength: 253 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### NodeRegistrationOptions



NodeRegistrationOptions holds fields that relate to registering a new control-plane or node to the cluster, either via "kubeadm init" or "kubeadm join".
Note: The NodeRegistrationOptions struct has to be kept in sync with the structs in MarshalJSON.



_Appears in:_
- [InitConfiguration](#initconfiguration)
- [JoinConfiguration](#joinconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the `.Metadata.Name` field of the Node API object that will be created in this `kubeadm init` or `kubeadm join` operation.<br />This field is also used in the CommonName field of the kubelet's client certificate to the API server.<br />Defaults to the hostname of the node if not provided. |  | MaxLength: 253 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `criSocket` _string_ | criSocket is used to retrieve container runtime info. This information will be annotated to the Node API object, for later re-use |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `taints` _[Taint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#taint-v1-core) array_ | taints specifies the taints the Node API object should be registered with. If this field is unset, i.e. nil, in the `kubeadm init` process<br />it will be defaulted to []v1.Taint\{'node-role.kubernetes.io/master=""'\}. If you don't want to taint your control-plane node, set this field to an<br />empty slice, i.e. `taints: []` in the YAML file. This field is solely used for Node registration. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `kubeletExtraArgs` _object (keys:string, values:string)_ | kubeletExtraArgs passes through extra arguments to the kubelet. The arguments here are passed to the kubelet command line via the environment file<br />kubeadm writes at runtime for the kubelet to source. This overrides the generic base-level configuration in the kubelet-config-1.X ConfigMap<br />Flags have higher priority when parsing. These values are local and specific to the node kubeadm is executing on. |  | Optional: \{\} <br /> |
| `ignorePreflightErrors` _string array_ | ignorePreflightErrors provides a slice of pre-flight errors to be ignored when the current node is registered. |  | MaxItems: 50 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `imagePullPolicy` _string_ | imagePullPolicy specifies the policy for image pulling<br />during kubeadm "init" and "join" operations. The value of<br />this field must be one of "Always", "IfNotPresent" or<br />"Never". Defaults to "IfNotPresent". This can be used only<br />with Kubernetes version equal to 1.22 and later. |  | Enum: [Always IfNotPresent Never] <br />Optional: \{\} <br /> |
| `imagePullSerial` _boolean_ | imagePullSerial specifies if image pulling performed by kubeadm must be done serially or in parallel.<br />This option takes effect only on Kubernetes >=1.31.0.<br />Default: true (defaulted in kubeadm) |  | Optional: \{\} <br /> |


#### Partition



Partition defines how to create and layout a partition.



_Appears in:_
- [DiskSetup](#disksetup)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `device` _string_ | device is the name of the device. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `layout` _boolean_ | layout specifies the device layout.<br />If it is true, a single partition will be created for the entire device.<br />When layout is false, it means don't partition or ignore existing partitioning. |  | Required: \{\} <br /> |
| `overwrite` _boolean_ | overwrite describes whether to skip checks and create the partition if a partition or filesystem is found on the device.<br />Use with caution. Default is 'false'. |  | Optional: \{\} <br /> |
| `tableType` _string_ | tableType specifies the tupe of partition table. The following are supported:<br />'mbr': default and setups a MS-DOS partition table<br />'gpt': setups a GPT partition table |  | Enum: [mbr gpt] <br />Optional: \{\} <br /> |


#### PasswdSource



PasswdSource is a union of all possible external source types for passwd data.
Only one field may be populated in any given instance. Developers adding new
sources of data for target systems should add them here.



_Appears in:_
- [User](#user)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secret` _[SecretPasswdSource](#secretpasswdsource)_ | secret represents a secret that should populate this password. |  | Required: \{\} <br /> |


#### Patches



Patches contains options related to applying patches to components deployed by kubeadm.



_Appears in:_
- [InitConfiguration](#initconfiguration)
- [JoinConfiguration](#joinconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `directory` _string_ | directory is a path to a directory that contains files named "target[suffix][+patchtype].extension".<br />For example, "kube-apiserver0+merge.yaml" or just "etcd.json". "target" can be one of<br />"kube-apiserver", "kube-controller-manager", "kube-scheduler", "etcd". "patchtype" can be one<br />of "strategic" "merge" or "json" and they match the patch formats supported by kubectl.<br />The default "patchtype" is "strategic". "extension" must be either "json" or "yaml".<br />"suffix" is an optional string that can be used to determine which patches are applied<br />first alpha-numerically.<br />These files can be written into the target directory via KubeadmConfig.Files which<br />specifies additional files to be created on the machine, either with content inline or<br />by referencing a secret. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### SecretFileSource



SecretFileSource adapts a Secret into a FileSource.

The contents of the target Secret's Data field will be presented
as files using the keys in the Data field as the file names.



_Appears in:_
- [FileSource](#filesource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the secret in the KubeadmBootstrapConfig's namespace to use. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `key` _string_ | key is the key in the secret's data map for this value. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |


#### SecretPasswdSource



SecretPasswdSource adapts a Secret into a PasswdSource.

The contents of the target Secret's Data field will be presented
as passwd using the keys in the Data field as the file names.



_Appears in:_
- [PasswdSource](#passwdsource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the secret in the KubeadmBootstrapConfig's namespace to use. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `key` _string_ | key is the key in the secret's data map for this value. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |


#### User



User defines the input for a generated user in cloud-init.



_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name specifies the user name |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `gecos` _string_ | gecos specifies the gecos to use for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `groups` _string_ | groups specifies the additional groups for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `homeDir` _string_ | homeDir specifies the home directory to use for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `inactive` _boolean_ | inactive specifies whether to mark the user as inactive |  | Optional: \{\} <br /> |
| `shell` _string_ | shell specifies the user's shell |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `passwd` _string_ | passwd specifies a hashed password for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `passwdFrom` _[PasswdSource](#passwdsource)_ | passwdFrom is a referenced source of passwd to populate the passwd. |  | Optional: \{\} <br /> |
| `primaryGroup` _string_ | primaryGroup specifies the primary group for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `lockPassword` _boolean_ | lockPassword specifies if password login should be disabled |  | Optional: \{\} <br /> |
| `sudo` _string_ | sudo specifies a sudo role for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `sshAuthorizedKeys` _string array_ | sshAuthorizedKeys specifies a list of ssh authorized keys for the user |  | MaxItems: 100 <br />items:MaxLength: 2048 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |



## bootstrap.cluster.x-k8s.io/v1beta2

Package v1beta2 contains API Schema definitions for the kubeadm v1beta2 API group.

### Resource Types
- [KubeadmConfig](#kubeadmconfig)
- [KubeadmConfigTemplate](#kubeadmconfigtemplate)



#### APIEndpoint



APIEndpoint struct contains elements of API server instance deployed on a node.

_Validation:_
- MinProperties: 1

_Appears in:_
- [InitConfiguration](#initconfiguration)
- [JoinControlPlane](#joincontrolplane)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `advertiseAddress` _string_ | advertiseAddress sets the IP address for the API server to advertise. |  | MaxLength: 39 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `bindPort` _integer_ | bindPort sets the secure port for the API Server to bind to.<br />Defaults to 6443. |  | Minimum: 1 <br />Optional: \{\} <br /> |


#### APIServer



APIServer holds settings necessary for API server deployments in the cluster.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ClusterConfiguration](#clusterconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `extraArgs` _[Arg](#arg) array_ | extraArgs is a list of args to pass to the control plane component.<br />The arg name must match the command line flag name except without leading dash(es).<br />Extra arguments will override existing default arguments set by kubeadm. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `extraVolumes` _[HostPathMount](#hostpathmount) array_ | extraVolumes is an extra set of host volumes, mounted to the control plane component. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `extraEnvs` _[EnvVar](#envvar)_ | extraEnvs is an extra set of environment variables to pass to the control plane component.<br />Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.<br />This option takes effect only on Kubernetes >=1.31.0. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `certSANs` _string array_ | certSANs sets extra Subject Alternative Names for the API Server signing cert. |  | MaxItems: 100 <br />MinItems: 1 <br />items:MaxLength: 253 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### Arg



Arg represents an argument with a name and a value.



_Appears in:_
- [APIServer](#apiserver)
- [ControllerManager](#controllermanager)
- [LocalEtcd](#localetcd)
- [NodeRegistrationOptions](#noderegistrationoptions)
- [Scheduler](#scheduler)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the Name of the extraArg. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `value` _string_ | value is the Value of the extraArg. |  | MaxLength: 1024 <br />MinLength: 0 <br />Required: \{\} <br /> |


#### BootstrapToken



BootstrapToken describes one bootstrap token, stored as a Secret in the cluster.



_Appears in:_
- [InitConfiguration](#initconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `token` _[BootstrapTokenString](#bootstraptokenstring)_ | token is used for establishing bidirectional trust between nodes and control-planes.<br />Used for joining nodes in the cluster. |  | MaxLength: 23 <br />MinLength: 1 <br />Type: string <br />Required: \{\} <br /> |
| `description` _string_ | description sets a human-friendly message why this token exists and what it's used<br />for, so other administrators can know its purpose. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `ttlSeconds` _integer_ | ttlSeconds defines the time to live for this token. Defaults to 24h.<br />Expires and ttlSeconds are mutually exclusive. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `usages` _string array_ | usages describes the ways in which this token can be used. Can by default be used<br />for establishing bidirectional trust, but that can be changed here. |  | MaxItems: 100 <br />MinItems: 1 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `groups` _string array_ | groups specifies the extra groups that this token will authenticate as when/if<br />used for authentication |  | MaxItems: 100 <br />MinItems: 1 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### BootstrapTokenDiscovery



BootstrapTokenDiscovery is used to set the options for bootstrap token based discovery.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Discovery](#discovery)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `token` _string_ | token is a token used to validate cluster information<br />fetched from the control-plane. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `apiServerEndpoint` _string_ | apiServerEndpoint is an IP or domain name to the API server from which info will be fetched. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `caCertHashes` _string array_ | caCertHashes specifies a set of public key pins to verify<br />when token-based discovery is used. The root CA found during discovery<br />must match one of these values. Specifying an empty set disables root CA<br />pinning, which can be unsafe. Each hash is specified as "<type>:<value>",<br />where the only currently supported type is "sha256". This is a hex-encoded<br />SHA-256 hash of the Subject Public Key Info (SPKI) object in DER-encoded<br />ASN.1. These hashes can be calculated using, for example, OpenSSL:<br />openssl x509 -pubkey -in ca.crt openssl rsa -pubin -outform der 2>&/dev/null \| openssl dgst -sha256 -hex |  | MaxItems: 100 <br />MinItems: 1 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `unsafeSkipCAVerification` _boolean_ | unsafeSkipCAVerification allows token-based discovery<br />without CA verification via CACertHashes. This can weaken<br />the security of kubeadm since other nodes can impersonate the control-plane. |  | Optional: \{\} <br /> |


#### BootstrapTokenString



BootstrapTokenString is a token of the format abcdef.abcdef0123456789 that is used
for both validation of the practically of the API server from a joining node's point
of view and as an authentication method for the node in the bootstrap phase of
"kubeadm join". This token is and should be short-lived.

_Validation:_
- MaxLength: 23
- MinLength: 1
- Type: string

_Appears in:_
- [BootstrapToken](#bootstraptoken)



#### ClusterConfiguration



ClusterConfiguration contains cluster-wide configuration for a kubeadm cluster.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `etcd` _[Etcd](#etcd)_ | etcd holds configuration for etcd.<br />NB: This value defaults to a Local (stacked) etcd |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `controlPlaneEndpoint` _string_ | controlPlaneEndpoint sets a stable IP address or DNS name for the control plane; it<br />can be a valid IP address or a RFC-1123 DNS subdomain, both with optional TCP port.<br />In case the ControlPlaneEndpoint is not specified, the AdvertiseAddress + BindPort<br />are used; in case the ControlPlaneEndpoint is specified but without a TCP port,<br />the BindPort is used.<br />Possible usages are:<br />e.g. In a cluster with more than one control plane instances, this field should be<br />assigned the address of the external load balancer in front of the<br />control plane instances.<br />e.g.  in environments with enforced node recycling, the ControlPlaneEndpoint<br />could be used for assigning a stable DNS to the control plane.<br />NB: This value defaults to the first value in the Cluster object status.apiEndpoints array. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `apiServer` _[APIServer](#apiserver)_ | apiServer contains extra settings for the API server control plane component |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `controllerManager` _[ControllerManager](#controllermanager)_ | controllerManager contains extra settings for the controller manager control plane component |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `scheduler` _[Scheduler](#scheduler)_ | scheduler contains extra settings for the scheduler control plane component |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `dns` _[DNS](#dns)_ | dns defines the options for the DNS add-on installed in the cluster. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `certificatesDir` _string_ | certificatesDir specifies where to store or look for all required certificates.<br />NB: if not provided, this will default to `/etc/kubernetes/pki` |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `imageRepository` _string_ | imageRepository sets the container registry to pull images from.<br />If not set, the default registry of kubeadm will be used (registry.k8s.io). |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `featureGates` _object (keys:string, values:boolean)_ | featureGates enabled by the user. |  | Optional: \{\} <br /> |
| `certificateValidityPeriodDays` _integer_ | certificateValidityPeriodDays specifies the validity period for non-CA certificates generated by kubeadm.<br />If not specified, kubeadm will use a default of 365 days (1 year).<br />This field is only supported with Kubernetes v1.31 or above. |  | Maximum: 1095 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `caCertificateValidityPeriodDays` _integer_ | caCertificateValidityPeriodDays specifies the validity period for CA certificates generated by Cluster API.<br />If not specified, Cluster API will use a default of 3650 days (10 years).<br />This field cannot be modified. |  | Maximum: 36500 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `encryptionAlgorithm` _[EncryptionAlgorithmType](#encryptionalgorithmtype)_ | encryptionAlgorithm holds the type of asymmetric encryption algorithm used for keys and certificates.<br />Can be one of "RSA-2048", "RSA-3072", "RSA-4096", "ECDSA-P256" or "ECDSA-P384".<br />For Kubernetes 1.34 or above, "ECDSA-P384" is supported.<br />If not specified, Cluster API will use RSA-2048 as default.<br />When this field is modified every certificate generated afterward will use the new<br />encryptionAlgorithm. Existing CA certificates and service account keys are not rotated.<br />This field is only supported with Kubernetes v1.31 or above. |  | Enum: [ECDSA-P256 ECDSA-P384 RSA-2048 RSA-3072 RSA-4096] <br />Optional: \{\} <br /> |


#### ContainerLinuxConfig



ContainerLinuxConfig contains CLC-specific configuration.

We use a structured type here to allow adding additional fields, for example 'version'.

_Validation:_
- MinProperties: 1

_Appears in:_
- [IgnitionSpec](#ignitionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `additionalConfig` _string_ | additionalConfig contains additional configuration to be merged with the Ignition<br />configuration generated by the bootstrapper controller. More info: https://coreos.github.io/ignition/operator-notes/#config-merging<br />The data format is documented here: https://kinvolk.io/docs/flatcar-container-linux/latest/provisioning/cl-config/ |  | MaxLength: 32768 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `strict` _boolean_ | strict controls if AdditionalConfig should be strictly parsed. If so, warnings are treated as errors. |  | Optional: \{\} <br /> |


#### ControllerManager



ControllerManager holds settings necessary for controller-manager deployments in the cluster.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ClusterConfiguration](#clusterconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `extraArgs` _[Arg](#arg) array_ | extraArgs is a list of args to pass to the control plane component.<br />The arg name must match the command line flag name except without leading dash(es).<br />Extra arguments will override existing default arguments set by kubeadm. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `extraVolumes` _[HostPathMount](#hostpathmount) array_ | extraVolumes is an extra set of host volumes, mounted to the control plane component. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `extraEnvs` _[EnvVar](#envvar)_ | extraEnvs is an extra set of environment variables to pass to the control plane component.<br />Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.<br />This option takes effect only on Kubernetes >=1.31.0. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### DNS



DNS defines the DNS addon that should be used in the cluster.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ClusterConfiguration](#clusterconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `imageRepository` _string_ | imageRepository sets the container registry to pull images from.<br />if not set, the ImageRepository defined in ClusterConfiguration will be used instead. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `imageTag` _string_ | imageTag allows to specify a tag for the image.<br />In case this value is set, kubeadm does not change automatically the version of the above components during upgrades. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### Discovery



Discovery specifies the options for the kubelet to use during the TLS Bootstrap process.

_Validation:_
- MinProperties: 1

_Appears in:_
- [JoinConfiguration](#joinconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bootstrapToken` _[BootstrapTokenDiscovery](#bootstraptokendiscovery)_ | bootstrapToken is used to set the options for bootstrap token based discovery<br />BootstrapToken and File are mutually exclusive |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `file` _[FileDiscovery](#filediscovery)_ | file is used to specify a file or URL to a kubeconfig file from which to load cluster information<br />BootstrapToken and File are mutually exclusive |  | Optional: \{\} <br /> |
| `tlsBootstrapToken` _string_ | tlsBootstrapToken is a token used for TLS bootstrapping.<br />If .BootstrapToken is set, this field is defaulted to .BootstrapToken.Token, but can be overridden.<br />If .File is set, this field **must be set** in case the KubeConfigFile does not contain any other authentication information |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### DiskSetup



DiskSetup defines input for generated disk_setup and fs_setup in cloud-init.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `partitions` _[Partition](#partition) array_ | partitions specifies the list of the partitions to setup. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `filesystems` _[Filesystem](#filesystem) array_ | filesystems specifies the list of file systems to setup. |  | MaxItems: 100 <br />Optional: \{\} <br /> |


#### Encoding

_Underlying type:_ _string_

Encoding specifies the cloud-init file encoding.

_Validation:_
- Enum: [base64 gzip gzip+base64]

_Appears in:_
- [File](#file)

| Field | Description |
| --- | --- |
| `base64` | Base64 implies the contents of the file are encoded as base64.<br /> |
| `gzip` | Gzip implies the contents of the file are encoded with gzip.<br /> |
| `gzip+base64` | GzipBase64 implies the contents of the file are first base64 encoded and then gzip encoded.<br /> |


#### EncryptionAlgorithmType

_Underlying type:_ _string_

EncryptionAlgorithmType can define an asymmetric encryption algorithm type.

_Validation:_
- Enum: [ECDSA-P256 ECDSA-P384 RSA-2048 RSA-3072 RSA-4096]

_Appears in:_
- [ClusterConfiguration](#clusterconfiguration)

| Field | Description |
| --- | --- |
| `ECDSA-P256` | EncryptionAlgorithmECDSAP256 defines the ECDSA encryption algorithm type with curve P256.<br /> |
| `ECDSA-P384` | EncryptionAlgorithmECDSAP384 defines the ECDSA encryption algorithm type with curve P384.<br /> |
| `RSA-2048` | EncryptionAlgorithmRSA2048 defines the RSA encryption algorithm type with key size 2048 bits.<br /> |
| `RSA-3072` | EncryptionAlgorithmRSA3072 defines the RSA encryption algorithm type with key size 3072 bits.<br /> |
| `RSA-4096` | EncryptionAlgorithmRSA4096 defines the RSA encryption algorithm type with key size 4096 bits.<br /> |


#### EnvVar



EnvVar represents an environment variable present in a Container.



_Appears in:_
- [APIServer](#apiserver)
- [ControllerManager](#controllermanager)
- [LocalEtcd](#localetcd)
- [Scheduler](#scheduler)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the environment variable.<br />May consist of any printable ASCII characters except '='. |  |  |
| `value` _string_ | Variable references $(VAR_NAME) are expanded<br />using the previously defined environment variables in the container and<br />any service environment variables. If a variable cannot be resolved,<br />the reference in the input string will be unchanged. Double $$ are reduced<br />to a single $, which allows for escaping the $(VAR_NAME) syntax: i.e.<br />"$$(VAR_NAME)" will produce the string literal "$(VAR_NAME)".<br />Escaped references will never be expanded, regardless of whether the variable<br />exists or not.<br />Defaults to "". |  | Optional: \{\} <br /> |
| `valueFrom` _[EnvVarSource](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#envvarsource-v1-core)_ | Source for the environment variable's value. Cannot be used if value is not empty. |  | Optional: \{\} <br /> |


#### Etcd



Etcd contains elements describing Etcd configuration.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ClusterConfiguration](#clusterconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `local` _[LocalEtcd](#localetcd)_ | local provides configuration knobs for configuring the local etcd instance<br />Local and External are mutually exclusive |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `external` _[ExternalEtcd](#externaletcd)_ | external describes how to connect to an external etcd cluster<br />Local and External are mutually exclusive |  | Optional: \{\} <br /> |


#### ExternalEtcd



ExternalEtcd describes an external etcd cluster.
Kubeadm has no knowledge of where certificate files live and they must be supplied.



_Appears in:_
- [Etcd](#etcd)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `endpoints` _string array_ | endpoints of etcd members. Required for ExternalEtcd. |  | MaxItems: 50 <br />MinItems: 1 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Required: \{\} <br /> |
| `caFile` _string_ | caFile is an SSL Certificate Authority file used to secure etcd communication.<br />Required if using a TLS connection. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `certFile` _string_ | certFile is an SSL certification file used to secure etcd communication.<br />Required if using a TLS connection. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `keyFile` _string_ | keyFile is an SSL key file used to secure etcd communication.<br />Required if using a TLS connection. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |


#### File



File defines the input for generating write_files in cloud-init.



_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `path` _string_ | path specifies the full path on disk where to store the file. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `owner` _string_ | owner specifies the ownership of the file, e.g. "root:root". |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `permissions` _string_ | permissions specifies the permissions to assign to the file, e.g. "0640". |  | MaxLength: 16 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `encoding` _[Encoding](#encoding)_ | encoding specifies the encoding of the file contents. |  | Enum: [base64 gzip gzip+base64] <br />Optional: \{\} <br /> |
| `append` _boolean_ | append specifies whether to append Content to existing file if Path exists. |  | Optional: \{\} <br /> |
| `content` _string_ | content is the actual content of the file. |  | MaxLength: 10240 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `contentFrom` _[FileSource](#filesource)_ | contentFrom is a referenced source of content to populate the file. |  | Optional: \{\} <br /> |


#### FileDiscovery



FileDiscovery is used to specify a file or URL to a kubeconfig file from which to load cluster information.



_Appears in:_
- [Discovery](#discovery)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kubeConfigPath` _string_ | kubeConfigPath is used to specify the actual file path or URL to the kubeconfig file from which to load cluster information |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `kubeConfig` _[FileDiscoveryKubeConfig](#filediscoverykubeconfig)_ | kubeConfig is used (optionally) to generate a KubeConfig based on the KubeadmConfig's information.<br />The file is generated at the path specified in KubeConfigPath.<br />Host address (server field) information is automatically populated based on the Cluster's ControlPlaneEndpoint.<br />Certificate Authority (certificate-authority-data field) is gathered from the cluster's CA secret. |  | Optional: \{\} <br /> |


#### FileDiscoveryKubeConfig



FileDiscoveryKubeConfig contains elements describing how to generate the kubeconfig for bootstrapping.



_Appears in:_
- [FileDiscovery](#filediscovery)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `cluster` _[KubeConfigCluster](#kubeconfigcluster)_ | cluster contains information about how to communicate with the kubernetes cluster.<br />By default the following fields are automatically populated:<br />- Server with the Cluster's ControlPlaneEndpoint.<br />- CertificateAuthorityData with the Cluster's CA certificate. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `user` _[KubeConfigUser](#kubeconfiguser)_ | user contains information that describes identity information.<br />This is used to tell the kubernetes cluster who you are. |  | MinProperties: 1 <br />Required: \{\} <br /> |


#### FileSource



FileSource is a union of all possible external source types for file data.
Only one field may be populated in any given instance. Developers adding new
sources of data for target systems should add them here.



_Appears in:_
- [File](#file)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secret` _[SecretFileSource](#secretfilesource)_ | secret represents a secret that should populate this file. |  | Required: \{\} <br /> |


#### Filesystem



Filesystem defines the file systems to be created.



_Appears in:_
- [DiskSetup](#disksetup)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `device` _string_ | device specifies the device name |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `filesystem` _string_ | filesystem specifies the file system type. |  | MaxLength: 128 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `label` _string_ | label specifies the file system label to be used. If set to None, no label is used. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `partition` _string_ | partition specifies the partition to use. The valid options are: "auto\|any", "auto", "any", "none", and <NUM>, where NUM is the actual partition number. |  | MaxLength: 128 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `overwrite` _boolean_ | overwrite defines whether or not to overwrite any existing filesystem.<br />If true, any pre-existing file system will be destroyed. Use with Caution. |  | Optional: \{\} <br /> |
| `replaceFS` _string_ | replaceFS is a special directive, used for Microsoft Azure that instructs cloud-init to replace a file system of <FS_TYPE>.<br />NOTE: unless you define a label, this requires the use of the 'any' partition directive. |  | MaxLength: 128 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `extraOpts` _string array_ | extraOpts defined extra options to add to the command for creating the file system. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### Format

_Underlying type:_ _string_

Format specifies the output format of the bootstrap data

_Validation:_
- Enum: [cloud-config ignition]

_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description |
| --- | --- |
| `cloud-config` | CloudConfig make the bootstrap data to be of cloud-config format.<br /> |
| `ignition` | Ignition make the bootstrap data to be of Ignition format.<br /> |


#### HostPathMount



HostPathMount contains elements describing volumes that are mounted from the
host.



_Appears in:_
- [APIServer](#apiserver)
- [ControllerManager](#controllermanager)
- [Scheduler](#scheduler)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the volume inside the pod template. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `hostPath` _string_ | hostPath is the path in the host that will be mounted inside<br />the pod. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `mountPath` _string_ | mountPath is the path inside the pod where hostPath will be mounted. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `readOnly` _boolean_ | readOnly controls write access to the volume |  | Optional: \{\} <br /> |
| `pathType` _[HostPathType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#hostpathtype-v1-core)_ | pathType is the type of the HostPath. |  | Optional: \{\} <br /> |


#### IgnitionSpec



IgnitionSpec contains Ignition specific configuration.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `containerLinuxConfig` _[ContainerLinuxConfig](#containerlinuxconfig)_ | containerLinuxConfig contains CLC specific configuration. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### InitConfiguration



InitConfiguration contains a list of elements that is specific "kubeadm init"-only runtime
information.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bootstrapTokens` _[BootstrapToken](#bootstraptoken) array_ | bootstrapTokens is respected at `kubeadm init` time and describes a set of Bootstrap Tokens to create.<br />This information IS NOT uploaded to the kubeadm cluster configmap, partly because of its sensitive nature |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `nodeRegistration` _[NodeRegistrationOptions](#noderegistrationoptions)_ | nodeRegistration holds fields that relate to registering the new control-plane node to the cluster.<br />When used in the context of control plane nodes, NodeRegistration should remain consistent<br />across both InitConfiguration and JoinConfiguration |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `localAPIEndpoint` _[APIEndpoint](#apiendpoint)_ | localAPIEndpoint represents the endpoint of the API server instance that's deployed on this control plane node<br />In HA setups, this differs from ClusterConfiguration.ControlPlaneEndpoint in the sense that ControlPlaneEndpoint<br />is the global endpoint for the cluster, which then loadbalances the requests to each individual API server. This<br />configuration object lets you customize what IP/DNS name and port the local API server advertises it's accessible<br />on. By default, kubeadm tries to auto-detect the IP of the default interface and use that, but in case that process<br />fails you may set the desired value here. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `skipPhases` _string array_ | skipPhases is a list of phases to skip during command execution.<br />The list of phases can be obtained with the "kubeadm init --help" command.<br />This option takes effect only on Kubernetes >=1.22.0. |  | MaxItems: 50 <br />MinItems: 1 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `patches` _[Patches](#patches)_ | patches contains options related to applying patches to components deployed by kubeadm during<br />"kubeadm init". The minimum kubernetes version needed to support Patches is v1.22 |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `timeouts` _[Timeouts](#timeouts)_ | timeouts holds various timeouts that apply to kubeadm commands. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### JoinConfiguration



JoinConfiguration contains elements describing a particular node.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeRegistration` _[NodeRegistrationOptions](#noderegistrationoptions)_ | nodeRegistration holds fields that relate to registering the new control-plane node to the cluster.<br />When used in the context of control plane nodes, NodeRegistration should remain consistent<br />across both InitConfiguration and JoinConfiguration |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `caCertPath` _string_ | caCertPath is the path to the SSL certificate authority used to<br />secure communications between node and control-plane.<br />Defaults to "/etc/kubernetes/pki/ca.crt". |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `discovery` _[Discovery](#discovery)_ | discovery specifies the options for the kubelet to use during the TLS Bootstrap process |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `controlPlane` _[JoinControlPlane](#joincontrolplane)_ | controlPlane defines the additional control plane instance to be deployed on the joining node.<br />If nil, no additional control plane instance will be deployed. |  | Optional: \{\} <br /> |
| `skipPhases` _string array_ | skipPhases is a list of phases to skip during command execution.<br />The list of phases can be obtained with the "kubeadm init --help" command.<br />This option takes effect only on Kubernetes >=1.22.0. |  | MaxItems: 50 <br />MinItems: 1 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `patches` _[Patches](#patches)_ | patches contains options related to applying patches to components deployed by kubeadm during<br />"kubeadm join". The minimum kubernetes version needed to support Patches is v1.22 |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `timeouts` _[Timeouts](#timeouts)_ | timeouts holds various timeouts that apply to kubeadm commands. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### JoinControlPlane



JoinControlPlane contains elements describing an additional control plane instance to be deployed on the joining node.



_Appears in:_
- [JoinConfiguration](#joinconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `localAPIEndpoint` _[APIEndpoint](#apiendpoint)_ | localAPIEndpoint represents the endpoint of the API server instance to be deployed on this node. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### KubeConfigAuthExec



KubeConfigAuthExec specifies a command to provide client credentials. The command is exec'd
and outputs structured stdout holding credentials.

See the client.authentication.k8s.io API group for specifications of the exact input
and output format.



_Appears in:_
- [KubeConfigUser](#kubeconfiguser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `command` _string_ | command to execute. |  | MaxLength: 1024 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `args` _string array_ | args is the arguments to pass to the command when executing it. |  | MaxItems: 100 <br />MinItems: 1 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `env` _[KubeConfigAuthExecEnv](#kubeconfigauthexecenv) array_ | env defines additional environment variables to expose to the process. These<br />are unioned with the host's environment, as well as variables client-go uses<br />to pass argument to the plugin. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `apiVersion` _string_ | apiVersion is preferred input version of the ExecInfo. The returned ExecCredentials MUST use<br />the same encoding version as the input.<br />Defaults to client.authentication.k8s.io/v1 if not set. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `provideClusterInfo` _boolean_ | provideClusterInfo determines whether or not to provide cluster information,<br />which could potentially contain very large CA data, to this exec plugin as a<br />part of the KUBERNETES_EXEC_INFO environment variable. By default, it is set<br />to false. Package k8s.io/client-go/tools/auth/exec provides helper methods for<br />reading this environment variable. |  | Optional: \{\} <br /> |


#### KubeConfigAuthExecEnv



KubeConfigAuthExecEnv is used for setting environment variables when executing an exec-based
credential plugin.



_Appears in:_
- [KubeConfigAuthExec](#kubeconfigauthexec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the environment variable |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `value` _string_ | value of the environment variable |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |


#### KubeConfigAuthProvider



KubeConfigAuthProvider holds the configuration for a specified auth provider.



_Appears in:_
- [KubeConfigUser](#kubeconfiguser)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the name of the authentication plugin. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `config` _object (keys:string, values:string)_ | config holds the parameters for the authentication plugin. |  | Optional: \{\} <br /> |


#### KubeConfigCluster



KubeConfigCluster contains information about how to communicate with a kubernetes cluster.

Adapted from clientcmdv1.Cluster.

_Validation:_
- MinProperties: 1

_Appears in:_
- [FileDiscoveryKubeConfig](#filediscoverykubeconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `server` _string_ | server is the address of the kubernetes cluster (https://hostname:port).<br />Defaults to https:// + Cluster.Spec.ControlPlaneEndpoint. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `tlsServerName` _string_ | tlsServerName is used to check server certificate. If TLSServerName is empty, the hostname used to contact the server is used. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `insecureSkipTLSVerify` _boolean_ | insecureSkipTLSVerify skips the validity check for the server's certificate. This will make your HTTPS connections insecure. |  | Optional: \{\} <br /> |
| `certificateAuthorityData` _integer array_ | certificateAuthorityData contains PEM-encoded certificate authority certificates.<br />Defaults to the Cluster's CA certificate if empty. |  | MaxLength: 51200 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `proxyURL` _string_ | proxyURL is the URL to the proxy to be used for all requests made by this<br />client. URLs with "http", "https", and "socks5" schemes are supported.  If<br />this configuration is not provided or the empty string, the client<br />attempts to construct a proxy configuration from http_proxy and<br />https_proxy environment variables. If these environment variables are not<br />set, the client does not attempt to proxy requests.<br />socks5 proxying does not currently support spdy streaming endpoints (exec,<br />attach, port forward). |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### KubeConfigUser



KubeConfigUser contains information that describes identity information.
This is used to tell the kubernetes cluster who you are.

Either authProvider or exec must be filled.

Adapted from clientcmdv1.AuthInfo.

_Validation:_
- MinProperties: 1

_Appears in:_
- [FileDiscoveryKubeConfig](#filediscoverykubeconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `authProvider` _[KubeConfigAuthProvider](#kubeconfigauthprovider)_ | authProvider specifies a custom authentication plugin for the kubernetes cluster. |  | Optional: \{\} <br /> |
| `exec` _[KubeConfigAuthExec](#kubeconfigauthexec)_ | exec specifies a custom exec-based authentication plugin for the kubernetes cluster. |  | Optional: \{\} <br /> |


#### KubeadmConfig



KubeadmConfig is the Schema for the kubeadmconfigs API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `bootstrap.cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `KubeadmConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmConfigSpec](#kubeadmconfigspec)_ | spec is the desired state of KubeadmConfig. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### KubeadmConfigSpec



KubeadmConfigSpec defines the desired state of KubeadmConfig.
Either ClusterConfiguration and InitConfiguration should be defined or the JoinConfiguration should be defined.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmConfig](#kubeadmconfig)
- [KubeadmConfigTemplateResource](#kubeadmconfigtemplateresource)
- [KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)
- [KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterConfiguration` _[ClusterConfiguration](#clusterconfiguration)_ | clusterConfiguration along with InitConfiguration are the configurations necessary for the init command |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `initConfiguration` _[InitConfiguration](#initconfiguration)_ | initConfiguration along with ClusterConfiguration are the configurations necessary for the init command |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `joinConfiguration` _[JoinConfiguration](#joinconfiguration)_ | joinConfiguration is the kubeadm configuration for the join command |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `files` _[File](#file) array_ | files specifies extra files to be passed to user_data upon creation. |  | MaxItems: 200 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `diskSetup` _[DiskSetup](#disksetup)_ | diskSetup specifies options for the creation of partition tables and file systems on devices. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `mounts` _[MountPoints](#mountpoints) array_ | mounts specifies a list of mount points to be setup. |  | MaxItems: 100 <br />MinItems: 1 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `bootCommands` _string array_ | bootCommands specifies extra commands to run very early in the boot process via the cloud-init bootcmd<br />module. bootcmd will run on every boot, 'cloud-init-per' command can be used to make bootcmd run exactly<br />once. This is typically run in the cloud-init.service systemd unit. This has no effect in Ignition. |  | MaxItems: 1000 <br />MinItems: 1 <br />items:MaxLength: 10240 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `preKubeadmCommands` _string array_ | preKubeadmCommands specifies extra commands to run before kubeadm runs.<br />With cloud-init, this is prepended to the runcmd module configuration, and is typically executed in<br />the cloud-final.service systemd unit. In Ignition, this is prepended to /etc/kubeadm.sh. |  | MaxItems: 1000 <br />MinItems: 1 <br />items:MaxLength: 10240 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `postKubeadmCommands` _string array_ | postKubeadmCommands specifies extra commands to run after kubeadm runs.<br />With cloud-init, this is appended to the runcmd module configuration, and is typically executed in<br />the cloud-final.service systemd unit. In Ignition, this is appended to /etc/kubeadm.sh. |  | MaxItems: 1000 <br />MinItems: 1 <br />items:MaxLength: 10240 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `users` _[User](#user) array_ | users specifies extra users to add |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `ntp` _[NTP](#ntp)_ | ntp specifies NTP configuration |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `format` _[Format](#format)_ | format specifies the output format of the bootstrap data.<br />Defaults to cloud-config if not set. |  | Enum: [cloud-config ignition] <br />Optional: \{\} <br /> |
| `verbosity` _integer_ | verbosity is the number for the kubeadm log level verbosity.<br />It overrides the `--v` flag in kubeadm commands. |  | Optional: \{\} <br /> |
| `ignition` _[IgnitionSpec](#ignitionspec)_ | ignition contains Ignition specific configuration. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### KubeadmConfigTemplate



KubeadmConfigTemplate is the Schema for the kubeadmconfigtemplates API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `bootstrap.cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `KubeadmConfigTemplate` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmConfigTemplateSpec](#kubeadmconfigtemplatespec)_ | spec is the desired state of KubeadmConfigTemplate. |  | Optional: \{\} <br /> |


#### KubeadmConfigTemplateResource



KubeadmConfigTemplateResource defines the Template structure.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmConfigTemplateSpec](#kubeadmconfigtemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmConfigSpec](#kubeadmconfigspec)_ | spec is the desired state of KubeadmConfig. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### KubeadmConfigTemplateSpec



KubeadmConfigTemplateSpec defines the desired state of KubeadmConfigTemplate.



_Appears in:_
- [KubeadmConfigTemplate](#kubeadmconfigtemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _[KubeadmConfigTemplateResource](#kubeadmconfigtemplateresource)_ | template defines the desired state of KubeadmConfigTemplate. |  | MinProperties: 1 <br />Required: \{\} <br /> |


#### LocalEtcd



LocalEtcd describes that kubeadm should run an etcd cluster locally.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Etcd](#etcd)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `imageRepository` _string_ | imageRepository sets the container registry to pull images from.<br />if not set, the ImageRepository defined in ClusterConfiguration will be used instead. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `imageTag` _string_ | imageTag allows to specify a tag for the image.<br />In case this value is set, kubeadm does not change automatically the version of the above components during upgrades. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `dataDir` _string_ | dataDir is the directory etcd will place its data.<br />Defaults to "/var/lib/etcd". |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `extraArgs` _[Arg](#arg) array_ | extraArgs is a list of args to pass to etcd.<br />The arg name must match the command line flag name except without leading dash(es).<br />Extra arguments will override existing default arguments set by kubeadm. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `extraEnvs` _[EnvVar](#envvar)_ | extraEnvs is an extra set of environment variables to pass to etcd.<br />Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.<br />This option takes effect only on Kubernetes >=1.31.0. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `serverCertSANs` _string array_ | serverCertSANs sets extra Subject Alternative Names for the etcd server signing cert. |  | MaxItems: 100 <br />MinItems: 1 <br />items:MaxLength: 253 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `peerCertSANs` _string array_ | peerCertSANs sets extra Subject Alternative Names for the etcd peer signing cert. |  | MaxItems: 100 <br />MinItems: 1 <br />items:MaxLength: 253 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### MountPoints

_Underlying type:_ _string array_

MountPoints defines input for generated mounts in cloud-init.

_Validation:_
- MaxItems: 100
- MinItems: 1
- items:MaxLength: 512
- items:MinLength: 1

_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)



#### NTP



NTP defines input for generated ntp in cloud-init.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `servers` _string array_ | servers specifies which NTP servers to use |  | MaxItems: 100 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `enabled` _boolean_ | enabled specifies whether NTP should be enabled |  | Optional: \{\} <br /> |


#### NodeRegistrationOptions



NodeRegistrationOptions holds fields that relate to registering a new control-plane or node to the cluster, either via "kubeadm init" or "kubeadm join".
Note: The NodeRegistrationOptions struct has to be kept in sync with the structs in MarshalJSON.

_Validation:_
- MinProperties: 1

_Appears in:_
- [InitConfiguration](#initconfiguration)
- [JoinConfiguration](#joinconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the `.Metadata.Name` field of the Node API object that will be created in this `kubeadm init` or `kubeadm join` operation.<br />This field is also used in the CommonName field of the kubelet's client certificate to the API server.<br />Defaults to the hostname of the node if not provided. |  | MaxLength: 253 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `criSocket` _string_ | criSocket is used to retrieve container runtime info. This information will be annotated to the Node API object, for later re-use |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `taints` _[Taint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#taint-v1-core)_ | taints specifies the taints the Node API object should be registered with. If this field is unset, i.e. nil, in the `kubeadm init` process<br />it will be defaulted to []v1.Taint\{'node-role.kubernetes.io/master=""'\}. If you don't want to taint your control-plane node, set this field to an<br />empty slice, i.e. `taints: []` in the YAML file. This field is solely used for Node registration. |  | MaxItems: 100 <br />MinItems: 0 <br />Optional: \{\} <br /> |
| `kubeletExtraArgs` _[Arg](#arg) array_ | kubeletExtraArgs is a list of args to pass to kubelet.<br />The arg name must match the command line flag name except without leading dash(es).<br />Extra arguments will override existing default arguments set by kubeadm. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `ignorePreflightErrors` _string array_ | ignorePreflightErrors provides a slice of pre-flight errors to be ignored when the current node is registered, e.g. 'IsPrivilegedUser,Swap'.<br />Value 'all' ignores errors from all checks. |  | MaxItems: 50 <br />MinItems: 1 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `imagePullPolicy` _[PullPolicy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#pullpolicy-v1-core)_ | imagePullPolicy specifies the policy for image pulling<br />during kubeadm "init" and "join" operations. The value of<br />this field must be one of "Always", "IfNotPresent" or<br />"Never". Defaults to "IfNotPresent" if not set. |  | Enum: [Always IfNotPresent Never] <br />Optional: \{\} <br /> |
| `imagePullSerial` _boolean_ | imagePullSerial specifies if image pulling performed by kubeadm must be done serially or in parallel.<br />This option takes effect only on Kubernetes >=1.31.0.<br />Default: true (defaulted in kubeadm) |  | Optional: \{\} <br /> |


#### Partition



Partition defines how to create and layout a partition.



_Appears in:_
- [DiskSetup](#disksetup)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `device` _string_ | device is the name of the device. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `layout` _boolean_ | layout specifies the device layout.<br />If it is true, a single partition will be created for the entire device.<br />When layout is false, it means don't partition or ignore existing partitioning. |  | Required: \{\} <br /> |
| `overwrite` _boolean_ | overwrite describes whether to skip checks and create the partition if a partition or filesystem is found on the device.<br />Use with caution. Default is 'false'. |  | Optional: \{\} <br /> |
| `tableType` _string_ | tableType specifies the tupe of partition table. The following are supported:<br />'mbr': default and setups a MS-DOS partition table<br />'gpt': setups a GPT partition table |  | Enum: [mbr gpt] <br />Optional: \{\} <br /> |


#### PasswdSource



PasswdSource is a union of all possible external source types for passwd data.
Only one field may be populated in any given instance. Developers adding new
sources of data for target systems should add them here.



_Appears in:_
- [User](#user)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secret` _[SecretPasswdSource](#secretpasswdsource)_ | secret represents a secret that should populate this password. |  | Required: \{\} <br /> |


#### Patches



Patches contains options related to applying patches to components deployed by kubeadm.

_Validation:_
- MinProperties: 1

_Appears in:_
- [InitConfiguration](#initconfiguration)
- [JoinConfiguration](#joinconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `directory` _string_ | directory is a path to a directory that contains files named "target[suffix][+patchtype].extension".<br />For example, "kube-apiserver0+merge.yaml" or just "etcd.json". "target" can be one of<br />"kube-apiserver", "kube-controller-manager", "kube-scheduler", "etcd". "patchtype" can be one<br />of "strategic" "merge" or "json" and they match the patch formats supported by kubectl.<br />The default "patchtype" is "strategic". "extension" must be either "json" or "yaml".<br />"suffix" is an optional string that can be used to determine which patches are applied<br />first alpha-numerically.<br />These files can be written into the target directory via KubeadmConfig.Files which<br />specifies additional files to be created on the machine, either with content inline or<br />by referencing a secret. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### Scheduler



Scheduler holds settings necessary for scheduler deployments in the cluster.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ClusterConfiguration](#clusterconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `extraArgs` _[Arg](#arg) array_ | extraArgs is a list of args to pass to the control plane component.<br />The arg name must match the command line flag name except without leading dash(es).<br />Extra arguments will override existing default arguments set by kubeadm. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `extraVolumes` _[HostPathMount](#hostpathmount) array_ | extraVolumes is an extra set of host volumes, mounted to the control plane component. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `extraEnvs` _[EnvVar](#envvar)_ | extraEnvs is an extra set of environment variables to pass to the control plane component.<br />Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.<br />This option takes effect only on Kubernetes >=1.31.0. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### SecretFileSource



SecretFileSource adapts a Secret into a FileSource.

The contents of the target Secret's Data field will be presented
as files using the keys in the Data field as the file names.



_Appears in:_
- [FileSource](#filesource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the secret in the KubeadmBootstrapConfig's namespace to use. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `key` _string_ | key is the key in the secret's data map for this value. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |


#### SecretPasswdSource



SecretPasswdSource adapts a Secret into a PasswdSource.

The contents of the target Secret's Data field will be presented
as passwd using the keys in the Data field as the file names.



_Appears in:_
- [PasswdSource](#passwdsource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the secret in the KubeadmBootstrapConfig's namespace to use. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `key` _string_ | key is the key in the secret's data map for this value. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |


#### Timeouts



Timeouts holds various timeouts that apply to kubeadm commands.

_Validation:_
- MinProperties: 1

_Appears in:_
- [InitConfiguration](#initconfiguration)
- [JoinConfiguration](#joinconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `controlPlaneComponentHealthCheckSeconds` _integer_ | controlPlaneComponentHealthCheckSeconds is the amount of time to wait for a control plane<br />component, such as the API server, to be healthy during "kubeadm init" and "kubeadm join".<br />If not set, it defaults to 4m (240s). |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `kubeletHealthCheckSeconds` _integer_ | kubeletHealthCheckSeconds is the amount of time to wait for the kubelet to be healthy<br />during "kubeadm init" and "kubeadm join".<br />If not set, it defaults to 4m (240s). |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `kubernetesAPICallSeconds` _integer_ | kubernetesAPICallSeconds is the amount of time to wait for the kubeadm client to complete a request to<br />the API server. This applies to all types of methods (GET, POST, etc).<br />If not set, it defaults to 1m (60s). |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `etcdAPICallSeconds` _integer_ | etcdAPICallSeconds is the amount of time to wait for the kubeadm etcd client to complete a request to<br />the etcd cluster.<br />If not set, it defaults to 2m (120s). |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `tlsBootstrapSeconds` _integer_ | tlsBootstrapSeconds is the amount of time to wait for the kubelet to complete TLS bootstrap<br />for a joining node.<br />If not set, it defaults to 5m (300s). |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `discoverySeconds` _integer_ | discoverySeconds is the amount of time to wait for kubeadm to validate the API server identity<br />for a joining node.<br />If not set, it defaults to 5m (300s). |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### User



User defines the input for a generated user in cloud-init.



_Appears in:_
- [KubeadmConfigSpec](#kubeadmconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name specifies the user name |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `gecos` _string_ | gecos specifies the gecos to use for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `groups` _string_ | groups specifies the additional groups for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `homeDir` _string_ | homeDir specifies the home directory to use for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `inactive` _boolean_ | inactive specifies whether to mark the user as inactive |  | Optional: \{\} <br /> |
| `shell` _string_ | shell specifies the user's shell |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `passwd` _string_ | passwd specifies a hashed password for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `passwdFrom` _[PasswdSource](#passwdsource)_ | passwdFrom is a referenced source of passwd to populate the passwd. |  | Optional: \{\} <br /> |
| `primaryGroup` _string_ | primaryGroup specifies the primary group for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `lockPassword` _boolean_ | lockPassword specifies if password login should be disabled |  | Optional: \{\} <br /> |
| `sudo` _string_ | sudo specifies a sudo role for the user |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `sshAuthorizedKeys` _string array_ | sshAuthorizedKeys specifies a list of ssh authorized keys for the user |  | MaxItems: 100 <br />items:MaxLength: 2048 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |



## cluster.x-k8s.io/v1beta1

Package v1beta1 contains API Schema definitions for the cluster v1beta1 API group

Deprecated: This package is deprecated and is going to be removed when support for v1beta1 will be dropped.

### Resource Types
- [Cluster](#cluster)
- [ClusterClass](#clusterclass)
- [Machine](#machine)
- [MachineDeployment](#machinedeployment)
- [MachineDrainRule](#machinedrainrule)
- [MachineHealthCheck](#machinehealthcheck)
- [MachinePool](#machinepool)
- [MachineSet](#machineset)



#### APIEndpoint



APIEndpoint represents a reachable Kubernetes API endpoint.



_Appears in:_
- [ClusterSpec](#clusterspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `host` _string_ | host is the hostname on which the API server is serving. |  | MaxLength: 512 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the port on which the API server is serving. |  | Optional: \{\} <br /> |


#### Bootstrap



Bootstrap encapsulates fields to configure the Machine’s bootstrapping mechanism.



_Appears in:_
- [MachineSpec](#machinespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `configRef` _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectreference-v1-core)_ | configRef is a reference to a bootstrap provider-specific resource<br />that holds configuration details. The reference is optional to<br />allow users/operators to specify Bootstrap.DataSecretName without<br />the need of a controller. |  | Optional: \{\} <br /> |
| `dataSecretName` _string_ | dataSecretName is the name of the secret that stores the bootstrap data script.<br />If nil, the Machine should remain in the Pending state. |  | MaxLength: 253 <br />MinLength: 0 <br />Optional: \{\} <br /> |


#### Cluster



Cluster is the Schema for the clusters API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `Cluster` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[ClusterSpec](#clusterspec)_ | spec is the desired state of Cluster. |  | Optional: \{\} <br /> |


#### ClusterAvailabilityGate



ClusterAvailabilityGate contains the type of a Cluster condition to be used as availability gate.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)
- [ClusterSpec](#clusterspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditionType` _string_ | conditionType refers to a condition with matching type in the Cluster's condition list.<br />If the conditions doesn't exist, it will be treated as unknown.<br />Note: Both Cluster API conditions or conditions added by 3rd party controllers can be used as availability gates. |  | MaxLength: 316 <br />MinLength: 1 <br />Pattern: `^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$` <br />Required: \{\} <br /> |
| `polarity` _[ConditionPolarity](#conditionpolarity)_ | polarity of the conditionType specified in this availabilityGate.<br />Valid values are Positive, Negative and omitted.<br />When omitted, the default behaviour will be Positive.<br />A positive polarity means that the condition should report a true status under normal conditions.<br />A negative polarity means that the condition should report a false status under normal conditions. |  | Enum: [Positive Negative] <br />Optional: \{\} <br /> |


#### ClusterClass



ClusterClass is a template which can be used to create managed topologies.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `ClusterClass` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[ClusterClassSpec](#clusterclassspec)_ | spec is the desired state of ClusterClass. |  | Optional: \{\} <br /> |


#### ClusterClassPatch



ClusterClassPatch defines a patch which is applied to customize the referenced templates.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the patch. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `description` _string_ | description is a human-readable description of this patch. |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `enabledIf` _string_ | enabledIf is a Go template to be used to calculate if a patch should be enabled.<br />It can reference variables defined in .spec.variables and builtin variables.<br />The patch will be enabled if the template evaluates to `true`, otherwise it will<br />be disabled.<br />If EnabledIf is not set, the patch will be enabled per default. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `definitions` _[PatchDefinition](#patchdefinition) array_ | definitions define inline patches.<br />Note: Patches will be applied in the order of the array.<br />Note: Exactly one of Definitions or External must be set. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `external` _[ExternalPatchDefinition](#externalpatchdefinition)_ | external defines an external patch.<br />Note: Exactly one of Definitions or External must be set. |  | Optional: \{\} <br /> |


#### ClusterClassSpec



ClusterClassSpec describes the desired state of the ClusterClass.



_Appears in:_
- [ClusterClass](#clusterclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `availabilityGates` _[ClusterAvailabilityGate](#clusteravailabilitygate) array_ | availabilityGates specifies additional conditions to include when evaluating Cluster Available condition.<br />NOTE: this field is considered only for computing v1beta2 conditions.<br />NOTE: If a Cluster is using this ClusterClass, and this Cluster defines a custom list of availabilityGates,<br />such list overrides availabilityGates defined in this field. |  | MaxItems: 32 <br />Optional: \{\} <br /> |
| `infrastructure` _[LocalObjectTemplate](#localobjecttemplate)_ | infrastructure is a reference to a provider-specific template that holds<br />the details for provisioning infrastructure specific cluster<br />for the underlying provider.<br />The underlying provider is responsible for the implementation<br />of the template to an infrastructure cluster. |  | Optional: \{\} <br /> |
| `infrastructureNamingStrategy` _[InfrastructureNamingStrategy](#infrastructurenamingstrategy)_ | infrastructureNamingStrategy allows changing the naming pattern used when creating the infrastructure object. |  | Optional: \{\} <br /> |
| `controlPlane` _[ControlPlaneClass](#controlplaneclass)_ | controlPlane is a reference to a local struct that holds the details<br />for provisioning the Control Plane for the Cluster. |  | Optional: \{\} <br /> |
| `workers` _[WorkersClass](#workersclass)_ | workers describes the worker nodes for the cluster.<br />It is a collection of node types which can be used to create<br />the worker nodes of the cluster. |  | Optional: \{\} <br /> |
| `variables` _[ClusterClassVariable](#clusterclassvariable) array_ | variables defines the variables which can be configured<br />in the Cluster topology and are then used in patches. |  | MaxItems: 1000 <br />Optional: \{\} <br /> |
| `patches` _[ClusterClassPatch](#clusterclasspatch) array_ | patches defines the patches which are applied to customize<br />referenced templates of a ClusterClass.<br />Note: Patches will be applied in the order of the array. |  | MaxItems: 1000 <br />Optional: \{\} <br /> |




#### ClusterClassStatusVariableDefinition



ClusterClassStatusVariableDefinition defines a variable which appears in the status of a ClusterClass.



_Appears in:_
- [ClusterClassStatusVariable](#clusterclassstatusvariable)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `from` _string_ | from specifies the origin of the variable definition.<br />This will be `inline` for variables defined in the ClusterClass or the name of a patch defined in the ClusterClass<br />for variables discovered from a DiscoverVariables runtime extensions. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `required` _boolean_ | required specifies if the variable is required.<br />Note: this applies to the variable as a whole and thus the<br />top-level object defined in the schema. If nested fields are<br />required, this will be specified inside the schema. |  | Required: \{\} <br /> |
| `metadata` _[ClusterClassVariableMetadata](#clusterclassvariablemetadata)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `schema` _[VariableSchema](#variableschema)_ | schema defines the schema of the variable. |  | Required: \{\} <br /> |


#### ClusterClassVariable



ClusterClassVariable defines a variable which can
be configured in the Cluster topology and used in patches.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the variable. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `required` _boolean_ | required specifies if the variable is required.<br />Note: this applies to the variable as a whole and thus the<br />top-level object defined in the schema. If nested fields are<br />required, this will be specified inside the schema. |  | Required: \{\} <br /> |
| `metadata` _[ClusterClassVariableMetadata](#clusterclassvariablemetadata)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `schema` _[VariableSchema](#variableschema)_ | schema defines the schema of the variable. |  | Required: \{\} <br /> |


#### ClusterClassVariableMetadata



ClusterClassVariableMetadata is the metadata of a variable.
It can be used to add additional data for higher level tools to
a ClusterClassVariable.

Deprecated: This struct is deprecated and is going to be removed in the next apiVersion.



_Appears in:_
- [ClusterClassStatusVariableDefinition](#clusterclassstatusvariabledefinition)
- [ClusterClassVariable](#clusterclassvariable)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `labels` _object (keys:string, values:string)_ | labels is a map of string keys and values that can be used to organize and categorize<br />(scope and select) variables. |  | Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ | annotations is an unstructured key value map that can be used to store and<br />retrieve arbitrary metadata.<br />They are not queryable. |  | Optional: \{\} <br /> |




#### ClusterNetwork



ClusterNetwork specifies the different networking
parameters for a cluster.



_Appears in:_
- [ClusterSpec](#clusterspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiServerPort` _integer_ | apiServerPort specifies the port the API Server should bind to.<br />Defaults to 6443. |  | Optional: \{\} <br /> |
| `services` _[NetworkRanges](#networkranges)_ | services is the network ranges from which service VIPs are allocated. |  | Optional: \{\} <br /> |
| `pods` _[NetworkRanges](#networkranges)_ | pods is the network ranges from which Pod networks are allocated. |  | Optional: \{\} <br /> |
| `serviceDomain` _string_ | serviceDomain is the domain name for services. |  | MaxLength: 253 <br />MinLength: 1 <br />Optional: \{\} <br /> |




#### ClusterSpec



ClusterSpec defines the desired state of Cluster.



_Appears in:_
- [Cluster](#cluster)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `paused` _boolean_ | paused can be used to prevent controllers from processing the Cluster and all its associated objects. |  | Optional: \{\} <br /> |
| `clusterNetwork` _[ClusterNetwork](#clusternetwork)_ | clusterNetwork represents the cluster network configuration. |  | Optional: \{\} <br /> |
| `controlPlaneEndpoint` _[APIEndpoint](#apiendpoint)_ | controlPlaneEndpoint represents the endpoint used to communicate with the control plane. |  | Optional: \{\} <br /> |
| `controlPlaneRef` _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectreference-v1-core)_ | controlPlaneRef is an optional reference to a provider-specific resource that holds<br />the details for provisioning the Control Plane for a Cluster. |  | Optional: \{\} <br /> |
| `infrastructureRef` _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectreference-v1-core)_ | infrastructureRef is a reference to a provider-specific resource that holds the details<br />for provisioning infrastructure for a cluster in said provider. |  | Optional: \{\} <br /> |
| `topology` _[Topology](#topology)_ | topology encapsulates the topology for the cluster.<br />NOTE: It is required to enable the ClusterTopology<br />feature gate flag to activate managed topologies support. |  | Optional: \{\} <br /> |
| `availabilityGates` _[ClusterAvailabilityGate](#clusteravailabilitygate) array_ | availabilityGates specifies additional conditions to include when evaluating Cluster Available condition.<br />If this field is not defined and the Cluster implements a managed topology, availabilityGates<br />from the corresponding ClusterClass will be used, if any.<br />NOTE: this field is considered only for computing v1beta2 conditions. |  | MaxItems: 32 <br />Optional: \{\} <br /> |


#### ClusterVariable



ClusterVariable can be used to customize the Cluster through patches. Each ClusterVariable is associated with a
Variable definition in the ClusterClass `status` variables.



_Appears in:_
- [ControlPlaneVariables](#controlplanevariables)
- [MachineDeploymentVariables](#machinedeploymentvariables)
- [MachinePoolVariables](#machinepoolvariables)
- [Topology](#topology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the variable. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `definitionFrom` _string_ | definitionFrom specifies where the definition of this Variable is from.<br />Deprecated: This field is deprecated, must not be set anymore and is going to be removed in the next apiVersion. |  | MaxLength: 256 <br />Optional: \{\} <br /> |
| `value` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#json-v1-apiextensions-k8s-io)_ | value of the variable.<br />Note: the value will be validated against the schema of the corresponding ClusterClassVariable<br />from the ClusterClass.<br />Note: We have to use apiextensionsv1.JSON instead of a custom JSON type, because controller-tools has a<br />hard-coded schema for apiextensionsv1.JSON which cannot be produced by another type via controller-tools,<br />i.e. it is not possible to have no type field.<br />Ref: https://github.com/kubernetes-sigs/controller-tools/blob/d0e03a142d0ecdd5491593e941ee1d6b5d91dba6/pkg/crd/known_types.go#L106-L111 |  | Required: \{\} <br /> |


#### Condition



Condition defines an observation of a Cluster API resource operational state.



_Appears in:_
- [Conditions](#conditions)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[ConditionType](#conditiontype)_ | type of condition in CamelCase or in foo.example.com/CamelCase.<br />Many .condition.type values are consistent across resources like Available, but because arbitrary conditions<br />can be useful (see .node.status.conditions), the ability to deconflict is important. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `severity` _[ConditionSeverity](#conditionseverity)_ | severity provides an explicit classification of Reason code, so the users or machines can immediately<br />understand the current situation and act accordingly.<br />The Severity field MUST be set only when Status=False. |  | MaxLength: 32 <br />Optional: \{\} <br /> |
| `reason` _string_ | reason is the reason for the condition's last transition in CamelCase.<br />The specific API may choose whether or not this field is considered a guaranteed API.<br />This field may be empty. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `message` _string_ | message is a human readable message indicating details about the transition.<br />This field may be empty. |  | MaxLength: 10240 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### ConditionPolarity

_Underlying type:_ _string_

ConditionPolarity defines the polarity for a metav1.Condition.



_Appears in:_
- [ClusterAvailabilityGate](#clusteravailabilitygate)
- [MachineReadinessGate](#machinereadinessgate)

| Field | Description |
| --- | --- |
| `Positive` | PositivePolarityCondition describe a condition with positive polarity, a condition<br />where the normal state is True. e.g. NetworkReady.<br /> |
| `Negative` | NegativePolarityCondition describe a condition with negative polarity, a condition<br />where the normal state is False. e.g. MemoryPressure.<br /> |


#### ConditionSeverity

_Underlying type:_ _string_

ConditionSeverity expresses the severity of a Condition Type failing.

_Validation:_
- MaxLength: 32

_Appears in:_
- [Condition](#condition)

| Field | Description |
| --- | --- |
| `Error` | ConditionSeverityError specifies that a condition with `Status=False` is an error.<br /> |
| `Warning` | ConditionSeverityWarning specifies that a condition with `Status=False` is a warning.<br /> |
| `Info` | ConditionSeverityInfo specifies that a condition with `Status=False` is informative.<br /> |
| `` | ConditionSeverityNone should apply only to conditions with `Status=True`.<br /> |


#### ConditionType

_Underlying type:_ _string_

ConditionType is a valid value for Condition.Type.

_Validation:_
- MaxLength: 256
- MinLength: 1

_Appears in:_
- [Condition](#condition)

| Field | Description |
| --- | --- |
| `Ready` | ReadyCondition defines the Ready condition type that summarizes the operational state of a Cluster API object.<br /> |
| `InfrastructureReady` | InfrastructureReadyCondition reports a summary of current status of the infrastructure object defined for this cluster/machine/machinepool.<br />This condition is mirrored from the Ready condition in the infrastructure ref object, and<br />the absence of this condition might signal problems in the reconcile external loops or the fact that<br />the infrastructure provider does not implement the Ready condition yet.<br /> |
| `VariablesReconciled` | ClusterClassVariablesReconciledCondition reports if the ClusterClass variables, including both inline and external<br />variables, have been successfully reconciled.<br />This signals that the ClusterClass is ready to be used to default and validate variables on Clusters using<br />this ClusterClass.<br /> |
| `ControlPlaneInitialized` | ControlPlaneInitializedCondition reports if the cluster's control plane has been initialized such that the<br />cluster's apiserver is reachable. If no Control Plane provider is in use this condition reports that at least one<br />control plane Machine has a node reference. Once this Condition is marked true, its value is never changed. See<br />the ControlPlaneReady condition for an indication of the current readiness of the cluster's control plane.<br /> |
| `ControlPlaneReady` | ControlPlaneReadyCondition reports the ready condition from the control plane object defined for this cluster.<br />This condition is mirrored from the Ready condition in the control plane ref object, and<br />the absence of this condition might signal problems in the reconcile external loops or the fact that<br />the control plane provider does not implement the Ready condition yet.<br /> |
| `BootstrapReady` | BootstrapReadyCondition reports a summary of current status of the bootstrap object defined for this machine.<br />This condition is mirrored from the Ready condition in the bootstrap ref object, and<br />the absence of this condition might signal problems in the reconcile external loops or the fact that<br />the bootstrap provider does not implement the Ready condition yet.<br /> |
| `DrainingSucceeded` | DrainingSucceededCondition provide evidence of the status of the node drain operation which happens during the machine<br />deletion process.<br /> |
| `PreDrainDeleteHookSucceeded` | PreDrainDeleteHookSucceededCondition reports a machine waiting for a PreDrainDeleteHook before being delete.<br /> |
| `PreTerminateDeleteHookSucceeded` | PreTerminateDeleteHookSucceededCondition reports a machine waiting for a PreDrainDeleteHook before being delete.<br /> |
| `VolumeDetachSucceeded` | VolumeDetachSucceededCondition reports a machine waiting for volumes to be detached.<br /> |
| `HealthCheckSucceeded` | MachineHealthCheckSucceededCondition is set on machines that have passed a healthcheck by the MachineHealthCheck controller.<br />In the event that the health check fails it will be set to False.<br /> |
| `OwnerRemediated` | MachineOwnerRemediatedCondition is set on machines that have failed a healthcheck by the MachineHealthCheck controller.<br />MachineOwnerRemediatedCondition is set to False after a health check fails, but should be changed to True by the owning controller after remediation succeeds.<br /> |
| `ExternalRemediationTemplateAvailable` | ExternalRemediationTemplateAvailableCondition is set on machinehealthchecks when MachineHealthCheck controller uses external remediation.<br />ExternalRemediationTemplateAvailableCondition is set to false if external remediation template is not found.<br /> |
| `ExternalRemediationRequestAvailable` | ExternalRemediationRequestAvailableCondition is set on machinehealthchecks when MachineHealthCheck controller uses external remediation.<br />ExternalRemediationRequestAvailableCondition is set to false if creating external remediation request fails.<br /> |
| `NodeHealthy` | MachineNodeHealthyCondition provides info about the operational state of the Kubernetes node hosted on the machine by summarizing  node conditions.<br />If the conditions defined in a Kubernetes node (i.e., NodeReady, NodeMemoryPressure, NodeDiskPressure and NodePIDPressure) are in a healthy state, it will be set to True.<br /> |
| `RemediationAllowed` | RemediationAllowedCondition is set on MachineHealthChecks to show the status of whether the MachineHealthCheck is<br />allowed to remediate any Machines or whether it is blocked from remediating any further.<br /> |
| `Available` | MachineDeploymentAvailableCondition means the MachineDeployment is available, that is, at least the minimum available<br />machines required (i.e. Spec.Replicas-MaxUnavailable when MachineDeploymentStrategyType = RollingUpdate) are up and running for at least minReadySeconds.<br /> |
| `MachineSetReady` | MachineSetReadyCondition reports a summary of current status of the MachineSet owned by the MachineDeployment.<br /> |
| `MachinesCreated` | MachinesCreatedCondition documents that the machines controlled by the MachineSet are created.<br />When this condition is false, it indicates that there was an error when cloning the infrastructure/bootstrap template or<br />when generating the machine object.<br /> |
| `MachinesReady` | MachinesReadyCondition reports an aggregate of current status of the machines controlled by the MachineSet.<br /> |
| `Resized` | ResizedCondition documents a MachineSet is resizing the set of controlled machines.<br /> |
| `TopologyReconciled` | TopologyReconciledCondition provides evidence about the reconciliation of a Cluster topology into<br />the managed objects of the Cluster.<br />Status false means that for any reason, the values defined in Cluster.spec.topology are not yet applied to<br />managed objects on the Cluster; status true means that Cluster.spec.topology have been applied to<br />the objects in the Cluster (but this does not imply those objects are already reconciled to the spec provided).<br /> |
| `RefVersionsUpToDate` | ClusterClassRefVersionsUpToDateCondition documents if the references in the ClusterClass are<br />up-to-date (i.e. they are using the latest apiVersion of the current Cluster API contract from<br />the corresponding CRD).<br /> |
| `ReplicasReady` | ReplicasReadyCondition reports an aggregate of current status of the replicas controlled by the MachinePool.<br /> |




#### ControlPlaneClass



ControlPlaneClass defines the class for the control plane.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `ref` _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectreference-v1-core)_ | ref is a required reference to a custom resource<br />offered by a provider. |  | Required: \{\} <br /> |
| `machineInfrastructure` _[LocalObjectTemplate](#localobjecttemplate)_ | machineInfrastructure defines the metadata and infrastructure information<br />for control plane machines.<br />This field is supported if and only if the control plane provider template<br />referenced above is Machine based and supports setting replicas. |  | Optional: \{\} <br /> |
| `machineHealthCheck` _[MachineHealthCheckClass](#machinehealthcheckclass)_ | machineHealthCheck defines a MachineHealthCheck for this ControlPlaneClass.<br />This field is supported if and only if the ControlPlane provider template<br />referenced above is Machine based and supports setting replicas. |  | Optional: \{\} <br /> |
| `namingStrategy` _[ControlPlaneClassNamingStrategy](#controlplaneclassnamingstrategy)_ | namingStrategy allows changing the naming pattern used when creating the control plane provider object. |  | Optional: \{\} <br /> |
| `nodeDrainTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`<br />NOTE: This value can be overridden while defining a Cluster.Topology. |  | Optional: \{\} <br /> |
| `nodeVolumeDetachTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.<br />NOTE: This value can be overridden while defining a Cluster.Topology. |  | Optional: \{\} <br /> |
| `nodeDeletionTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds.<br />NOTE: This value can be overridden while defining a Cluster.Topology. |  | Optional: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition.<br />This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready<br />computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.<br />NOTE: This field is considered only for computing v1beta2 conditions.<br />NOTE: If a Cluster defines a custom list of readinessGates for the control plane,<br />such list overrides readinessGates defined in this field.<br />NOTE: Specific control plane provider implementations might automatically extend the list of readinessGates;<br />e.g. the kubeadm control provider adds ReadinessGates for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc. |  | MaxItems: 32 <br />Optional: \{\} <br /> |


#### ControlPlaneClassNamingStrategy



ControlPlaneClassNamingStrategy defines the naming strategy for control plane objects.



_Appears in:_
- [ControlPlaneClass](#controlplaneclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the name of the ControlPlane object.<br />If not defined, it will fallback to `\{\{ .cluster.name \}\}-\{\{ .random \}\}`.<br />If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will<br />get concatenated with a random suffix of length 5.<br />The templating mechanism provides the following arguments:<br />* `.cluster.name`: The name of the cluster object.<br />* `.random`: A random alphanumeric string, without vowels, of length 5. |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### ControlPlaneTopology



ControlPlaneTopology specifies the parameters for the control plane nodes in the cluster.



_Appears in:_
- [Topology](#topology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of control plane nodes.<br />If the value is nil, the ControlPlane object is created without the number of Replicas<br />and it's assumed that the control plane controller does not implement support for this field.<br />When specified against a control plane provider that lacks support for this field, this value will be ignored. |  | Optional: \{\} <br /> |
| `machineHealthCheck` _[MachineHealthCheckTopology](#machinehealthchecktopology)_ | machineHealthCheck allows to enable, disable and override<br />the MachineHealthCheck configuration in the ClusterClass for this control plane. |  | Optional: \{\} <br /> |
| `nodeDrainTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: NodeDrainTimeout is different from `kubectl drain --timeout` |  | Optional: \{\} <br /> |
| `nodeVolumeDetachTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Optional: \{\} <br /> |
| `nodeDeletionTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds. |  | Optional: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition.<br />This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready<br />computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.<br />If this field is not defined, readinessGates from the corresponding ControlPlaneClass will be used, if any.<br />NOTE: This field is considered only for computing v1beta2 conditions.<br />NOTE: Specific control plane provider implementations might automatically extend the list of readinessGates;<br />e.g. the kubeadm control provider adds ReadinessGates for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc. |  | MaxItems: 32 <br />Optional: \{\} <br /> |
| `variables` _[ControlPlaneVariables](#controlplanevariables)_ | variables can be used to customize the ControlPlane through patches. |  | Optional: \{\} <br /> |


#### ControlPlaneVariables



ControlPlaneVariables can be used to provide variables for the ControlPlane.



_Appears in:_
- [ControlPlaneTopology](#controlplanetopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `overrides` _[ClusterVariable](#clustervariable) array_ | overrides can be used to override Cluster level variables. |  | MaxItems: 1000 <br />Optional: \{\} <br /> |


#### ExternalPatchDefinition



ExternalPatchDefinition defines an external patch.
Note: At least one of GenerateExtension or ValidateExtension must be set.



_Appears in:_
- [ClusterClassPatch](#clusterclasspatch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `generateExtension` _string_ | generateExtension references an extension which is called to generate patches. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `validateExtension` _string_ | validateExtension references an extension which is called to validate the topology. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `discoverVariablesExtension` _string_ | discoverVariablesExtension references an extension which is called to discover variables. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `settings` _object (keys:string, values:string)_ | settings defines key value pairs to be passed to the extensions.<br />Values defined here take precedence over the values defined in the<br />corresponding ExtensionConfig. |  | Optional: \{\} <br /> |


#### FailureDomainSpec



FailureDomainSpec is the Schema for Cluster API failure domains.
It allows controllers to understand how many failure domains a cluster can optionally span across.



_Appears in:_
- [FailureDomains](#failuredomains)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `controlPlane` _boolean_ | controlPlane determines if this failure domain is suitable for use by control plane machines. |  | Optional: \{\} <br /> |
| `attributes` _object (keys:string, values:string)_ | attributes is a free form map of attributes an infrastructure provider might use or require. |  | Optional: \{\} <br /> |




#### FieldValueErrorReason

_Underlying type:_ _string_

FieldValueErrorReason is a machine-readable value providing more detail about why a field failed the validation.



_Appears in:_
- [ValidationRule](#validationrule)

| Field | Description |
| --- | --- |
| `FieldValueRequired` | FieldValueRequired is used to report required values that are not<br />provided (e.g. empty strings, null values, or empty arrays).<br /> |
| `FieldValueDuplicate` | FieldValueDuplicate is used to report collisions of values that must be<br />unique (e.g. unique IDs).<br /> |
| `FieldValueInvalid` | FieldValueInvalid is used to report malformed values (e.g. failed regex<br />match, too long, out of bounds).<br /> |
| `FieldValueForbidden` | FieldValueForbidden is used to report valid (as per formatting rules)<br />values which would be accepted under some conditions, but which are not<br />permitted by the current conditions (such as security policy).<br /> |


#### InfrastructureNamingStrategy



InfrastructureNamingStrategy defines the naming strategy for infrastructure objects.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the name of the Infrastructure object.<br />If not defined, it will fallback to `\{\{ .cluster.name \}\}-\{\{ .random \}\}`.<br />If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will<br />get concatenated with a random suffix of length 5.<br />The templating mechanism provides the following arguments:<br />* `.cluster.name`: The name of the cluster object.<br />* `.random`: A random alphanumeric string, without vowels, of length 5. |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### JSONPatch



JSONPatch defines a JSON patch.



_Appears in:_
- [PatchDefinition](#patchdefinition)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `op` _string_ | op defines the operation of the patch.<br />Note: Only `add`, `replace` and `remove` are supported. |  | Enum: [add replace remove] <br />Required: \{\} <br /> |
| `path` _string_ | path defines the path of the patch.<br />Note: Only the spec of a template can be patched, thus the path has to start with /spec/.<br />Note: For now the only allowed array modifications are `append` and `prepend`, i.e.:<br />* for op: `add`: only index 0 (prepend) and - (append) are allowed<br />* for op: `replace` or `remove`: no indexes are allowed |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `value` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#json-v1-apiextensions-k8s-io)_ | value defines the value of the patch.<br />Note: Either Value or ValueFrom is required for add and replace<br />operations. Only one of them is allowed to be set at the same time.<br />Note: We have to use apiextensionsv1.JSON instead of our JSON type,<br />because controller-tools has a hard-coded schema for apiextensionsv1.JSON<br />which cannot be produced by another type (unset type field).<br />Ref: https://github.com/kubernetes-sigs/controller-tools/blob/d0e03a142d0ecdd5491593e941ee1d6b5d91dba6/pkg/crd/known_types.go#L106-L111 |  | Optional: \{\} <br /> |
| `valueFrom` _[JSONPatchValue](#jsonpatchvalue)_ | valueFrom defines the value of the patch.<br />Note: Either Value or ValueFrom is required for add and replace<br />operations. Only one of them is allowed to be set at the same time. |  | Optional: \{\} <br /> |


#### JSONPatchValue



JSONPatchValue defines the value of a patch.
Note: Only one of the fields is allowed to be set at the same time.



_Appears in:_
- [JSONPatch](#jsonpatch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `variable` _string_ | variable is the variable to be used as value.<br />Variable can be one of the variables defined in .spec.variables or a builtin variable. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `template` _string_ | template is the Go template to be used to calculate the value.<br />A template can reference variables defined in .spec.variables and builtin variables.<br />Note: The template must evaluate to a valid YAML or JSON value. |  | MaxLength: 10240 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### JSONSchemaProps



JSONSchemaProps is a JSON-Schema following Specification Draft 4 (http://json-schema.org/).
This struct has been initially copied from apiextensionsv1.JSONSchemaProps, but all fields
which are not supported in CAPI have been removed.



_Appears in:_
- [JSONSchemaProps](#jsonschemaprops)
- [VariableSchema](#variableschema)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `description` _string_ | description is a human-readable description of this variable. |  | MaxLength: 4096 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `example` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#json-v1-apiextensions-k8s-io)_ | example is an example for this variable. |  | Optional: \{\} <br /> |
| `type` _string_ | type is the type of the variable.<br />Valid values are: object, array, string, integer, number or boolean. |  | Enum: [object array string integer number boolean] <br />Optional: \{\} <br /> |
| `properties` _object (keys:string, values:[JSONSchemaProps](#jsonschemaprops))_ | properties specifies fields of an object.<br />NOTE: Can only be set if type is object.<br />NOTE: Properties is mutually exclusive with AdditionalProperties.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | Schemaless: \{\} <br />Optional: \{\} <br /> |
| `additionalProperties` _[JSONSchemaProps](#jsonschemaprops)_ | additionalProperties specifies the schema of values in a map (keys are always strings).<br />NOTE: Can only be set if type is object.<br />NOTE: AdditionalProperties is mutually exclusive with Properties.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | Schemaless: \{\} <br />Optional: \{\} <br /> |
| `maxProperties` _integer_ | maxProperties is the maximum amount of entries in a map or properties in an object.<br />NOTE: Can only be set if type is object. |  | Optional: \{\} <br /> |
| `minProperties` _integer_ | minProperties is the minimum amount of entries in a map or properties in an object.<br />NOTE: Can only be set if type is object. |  | Optional: \{\} <br /> |
| `required` _string array_ | required specifies which fields of an object are required.<br />NOTE: Can only be set if type is object. |  | MaxItems: 1000 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `items` _[JSONSchemaProps](#jsonschemaprops)_ | items specifies fields of an array.<br />NOTE: Can only be set if type is array.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | Schemaless: \{\} <br />Optional: \{\} <br /> |
| `maxItems` _integer_ | maxItems is the max length of an array variable.<br />NOTE: Can only be set if type is array. |  | Optional: \{\} <br /> |
| `minItems` _integer_ | minItems is the min length of an array variable.<br />NOTE: Can only be set if type is array. |  | Optional: \{\} <br /> |
| `uniqueItems` _boolean_ | uniqueItems specifies if items in an array must be unique.<br />NOTE: Can only be set if type is array. |  | Optional: \{\} <br /> |
| `format` _string_ | format is an OpenAPI v3 format string. Unknown formats are ignored.<br />For a list of supported formats please see: (of the k8s.io/apiextensions-apiserver version we're currently using)<br />https://github.com/kubernetes/apiextensions-apiserver/blob/master/pkg/apiserver/validation/formats.go<br />NOTE: Can only be set if type is string. |  | MaxLength: 32 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `maxLength` _integer_ | maxLength is the max length of a string variable.<br />NOTE: Can only be set if type is string. |  | Optional: \{\} <br /> |
| `minLength` _integer_ | minLength is the min length of a string variable.<br />NOTE: Can only be set if type is string. |  | Optional: \{\} <br /> |
| `pattern` _string_ | pattern is the regex which a string variable must match.<br />NOTE: Can only be set if type is string. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `maximum` _integer_ | maximum is the maximum of an integer or number variable.<br />If ExclusiveMaximum is false, the variable is valid if it is lower than, or equal to, the value of Maximum.<br />If ExclusiveMaximum is true, the variable is valid if it is strictly lower than the value of Maximum.<br />NOTE: Can only be set if type is integer or number. |  | Optional: \{\} <br /> |
| `exclusiveMaximum` _boolean_ | exclusiveMaximum specifies if the Maximum is exclusive.<br />NOTE: Can only be set if type is integer or number. |  | Optional: \{\} <br /> |
| `minimum` _integer_ | minimum is the minimum of an integer or number variable.<br />If ExclusiveMinimum is false, the variable is valid if it is greater than, or equal to, the value of Minimum.<br />If ExclusiveMinimum is true, the variable is valid if it is strictly greater than the value of Minimum.<br />NOTE: Can only be set if type is integer or number. |  | Optional: \{\} <br /> |
| `exclusiveMinimum` _boolean_ | exclusiveMinimum specifies if the Minimum is exclusive.<br />NOTE: Can only be set if type is integer or number. |  | Optional: \{\} <br /> |
| `x-kubernetes-preserve-unknown-fields` _boolean_ | x-kubernetes-preserve-unknown-fields allows setting fields in a variable object<br />which are not defined in the variable schema. This affects fields recursively,<br />except if nested properties or additionalProperties are specified in the schema. |  | Optional: \{\} <br /> |
| `enum` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#json-v1-apiextensions-k8s-io) array_ | enum is the list of valid values of the variable.<br />NOTE: Can be set for all types. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `default` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#json-v1-apiextensions-k8s-io)_ | default is the default value of the variable.<br />NOTE: Can be set for all types. |  | Optional: \{\} <br /> |
| `x-kubernetes-validations` _[ValidationRule](#validationrule) array_ | x-kubernetes-validations describes a list of validation rules written in the CEL expression language. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `x-metadata` _[VariableSchemaMetadata](#variableschemametadata)_ | x-metadata is the metadata of a variable or a nested field within a variable.<br />It can be used to add additional data for higher level tools. |  | Optional: \{\} <br /> |
| `x-kubernetes-int-or-string` _boolean_ | x-kubernetes-int-or-string specifies that this value is<br />either an integer or a string. If this is true, an empty<br />type is allowed and type as child of anyOf is permitted<br />if following one of the following patterns:<br />1) anyOf:<br />   - type: integer<br />   - type: string<br />2) allOf:<br />   - anyOf:<br />     - type: integer<br />     - type: string<br />   - ... zero or more |  | Optional: \{\} <br /> |
| `allOf` _[JSONSchemaProps](#jsonschemaprops) array_ | allOf specifies that the variable must validate against all of the subschemas in the array.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | Schemaless: \{\} <br />Optional: \{\} <br /> |
| `oneOf` _[JSONSchemaProps](#jsonschemaprops) array_ | oneOf specifies that the variable must validate against exactly one of the subschemas in the array.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | Schemaless: \{\} <br />Optional: \{\} <br /> |
| `anyOf` _[JSONSchemaProps](#jsonschemaprops) array_ | anyOf specifies that the variable must validate against one or more of the subschemas in the array.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | Schemaless: \{\} <br />Optional: \{\} <br /> |
| `not` _[JSONSchemaProps](#jsonschemaprops)_ | not specifies that the variable must not validate against the subschema.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | Schemaless: \{\} <br />Optional: \{\} <br /> |


#### LocalObjectTemplate



LocalObjectTemplate defines a template for a topology Class.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)
- [ControlPlaneClass](#controlplaneclass)
- [MachineDeploymentClassTemplate](#machinedeploymentclasstemplate)
- [MachinePoolClassTemplate](#machinepoolclasstemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ref` _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectreference-v1-core)_ | ref is a required reference to a custom resource<br />offered by a provider. |  | Required: \{\} <br /> |


#### Machine



Machine is the Schema for the machines API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `Machine` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[MachineSpec](#machinespec)_ | spec is the desired state of Machine. |  | Optional: \{\} <br /> |


#### MachineAddress



MachineAddress contains information for the node's address.



_Appears in:_
- [MachineAddresses](#machineaddresses)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[MachineAddressType](#machineaddresstype)_ | type is the machine address type, one of Hostname, ExternalIP, InternalIP, ExternalDNS or InternalDNS. |  | Enum: [Hostname ExternalIP InternalIP ExternalDNS InternalDNS] <br />Required: \{\} <br /> |
| `address` _string_ | address is the machine address. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |


#### MachineAddressType

_Underlying type:_ _string_

MachineAddressType describes a valid MachineAddress type.

_Validation:_
- Enum: [Hostname ExternalIP InternalIP ExternalDNS InternalDNS]

_Appears in:_
- [MachineAddress](#machineaddress)

| Field | Description |
| --- | --- |
| `Hostname` |  |
| `ExternalIP` |  |
| `InternalIP` |  |
| `ExternalDNS` |  |
| `InternalDNS` |  |




#### MachineDeployment



MachineDeployment is the Schema for the machinedeployments API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `MachineDeployment` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[MachineDeploymentSpec](#machinedeploymentspec)_ | spec is the desired state of MachineDeployment. |  | Optional: \{\} <br /> |


#### MachineDeploymentClass



MachineDeploymentClass serves as a template to define a set of worker nodes of the cluster
provisioned using the `ClusterClass`.



_Appears in:_
- [WorkersClass](#workersclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `class` _string_ | class denotes a type of worker node present in the cluster,<br />this name MUST be unique within a ClusterClass and can be referenced<br />in the Cluster to create a managed MachineDeployment. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `template` _[MachineDeploymentClassTemplate](#machinedeploymentclasstemplate)_ | template is a local struct containing a collection of templates for creation of<br />MachineDeployment objects representing a set of worker nodes. |  | Required: \{\} <br /> |
| `machineHealthCheck` _[MachineHealthCheckClass](#machinehealthcheckclass)_ | machineHealthCheck defines a MachineHealthCheck for this MachineDeploymentClass. |  | Optional: \{\} <br /> |
| `failureDomain` _string_ | failureDomain is the failure domain the machines will be created in.<br />Must match a key in the FailureDomains map stored on the cluster object.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `namingStrategy` _[MachineDeploymentClassNamingStrategy](#machinedeploymentclassnamingstrategy)_ | namingStrategy allows changing the naming pattern used when creating the MachineDeployment. |  | Optional: \{\} <br /> |
| `nodeDrainTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass. |  | Optional: \{\} <br /> |
| `nodeVolumeDetachTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass. |  | Optional: \{\} <br /> |
| `nodeDeletionTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass. |  | Optional: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a newly created machine should<br />be ready.<br />Defaults to 0 (machine will be considered available as soon as it<br />is ready)<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass. |  | Optional: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition.<br />This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready<br />computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.<br />NOTE: This field is considered only for computing v1beta2 conditions.<br />NOTE: If a Cluster defines a custom list of readinessGates for a MachineDeployment using this MachineDeploymentClass,<br />such list overrides readinessGates defined in this field. |  | MaxItems: 32 <br />Optional: \{\} <br /> |
| `strategy` _[MachineDeploymentStrategy](#machinedeploymentstrategy)_ | strategy is the deployment strategy to use to replace existing machines with<br />new ones.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass. |  | Optional: \{\} <br /> |


#### MachineDeploymentClassNamingStrategy



MachineDeploymentClassNamingStrategy defines the naming strategy for machine deployment objects.



_Appears in:_
- [MachineDeploymentClass](#machinedeploymentclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the name of the MachineDeployment object.<br />If not defined, it will fallback to `\{\{ .cluster.name \}\}-\{\{ .machineDeployment.topologyName \}\}-\{\{ .random \}\}`.<br />If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will<br />get concatenated with a random suffix of length 5.<br />The templating mechanism provides the following arguments:<br />* `.cluster.name`: The name of the cluster object.<br />* `.random`: A random alphanumeric string, without vowels, of length 5.<br />* `.machineDeployment.topologyName`: The name of the MachineDeployment topology (Cluster.spec.topology.workers.machineDeployments[].name). |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentClassTemplate



MachineDeploymentClassTemplate defines how a MachineDeployment generated from a MachineDeploymentClass
should look like.



_Appears in:_
- [MachineDeploymentClass](#machinedeploymentclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `bootstrap` _[LocalObjectTemplate](#localobjecttemplate)_ | bootstrap contains the bootstrap template reference to be used<br />for the creation of worker Machines. |  | Required: \{\} <br /> |
| `infrastructure` _[LocalObjectTemplate](#localobjecttemplate)_ | infrastructure contains the infrastructure template reference to be used<br />for the creation of worker Machines. |  | Required: \{\} <br /> |




#### MachineDeploymentSpec



MachineDeploymentSpec defines the desired state of MachineDeployment.



_Appears in:_
- [MachineDeployment](#machinedeployment)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of desired machines.<br />This is a pointer to distinguish between explicit zero and not specified.<br />Defaults to:<br />* if the Kubernetes autoscaler min size and max size annotations are set:<br />  - if it's a new MachineDeployment, use min size<br />  - if the replicas field of the old MachineDeployment is < min size, use min size<br />  - if the replicas field of the old MachineDeployment is > max size, use max size<br />  - if the replicas field of the old MachineDeployment is in the (min size, max size) range, keep the value from the oldMD<br />* otherwise use 1<br />Note: Defaulting will be run whenever the replicas field is not set:<br />* A new MachineDeployment is created with replicas not set.<br />* On an existing MachineDeployment the replicas field was first set and is now unset.<br />Those cases are especially relevant for the following Kubernetes autoscaler use cases:<br />* A new MachineDeployment is created and replicas should be managed by the autoscaler<br />* An existing MachineDeployment which initially wasn't controlled by the autoscaler<br />  should be later controlled by the autoscaler |  | Optional: \{\} <br /> |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | selector is the label selector for machines. Existing MachineSets whose machines are<br />selected by this will be the ones affected by this deployment.<br />It must match the machine template's labels. |  | Required: \{\} <br /> |
| `template` _[MachineTemplateSpec](#machinetemplatespec)_ | template describes the machines that will be created. |  | Required: \{\} <br /> |
| `strategy` _[MachineDeploymentStrategy](#machinedeploymentstrategy)_ | strategy is the deployment strategy to use to replace existing machines with<br />new ones. |  | Optional: \{\} <br /> |
| `machineNamingStrategy` _[MachineNamingStrategy](#machinenamingstrategy)_ | machineNamingStrategy allows changing the naming pattern used when creating Machines.<br />Note: InfraMachines & BootstrapConfigs will use the same name as the corresponding Machines. |  | Optional: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a Node for a newly created machine should be ready before considering the replica available.<br />Defaults to 0 (machine will be considered available as soon as the Node is ready) |  | Optional: \{\} <br /> |
| `revisionHistoryLimit` _integer_ | revisionHistoryLimit is the number of old MachineSets to retain to allow rollback.<br />This is a pointer to distinguish between explicit zero and not specified.<br />Defaults to 1.<br />Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/issues/10479 for more details. |  | Optional: \{\} <br /> |
| `paused` _boolean_ | paused indicates that the deployment is paused. |  | Optional: \{\} <br /> |
| `progressDeadlineSeconds` _integer_ | progressDeadlineSeconds is the maximum time in seconds for a deployment to make progress before it<br />is considered to be failed. The deployment controller will continue to<br />process failed deployments and a condition with a ProgressDeadlineExceeded<br />reason will be surfaced in the deployment status. Note that progress will<br />not be estimated during the time a deployment is paused. Defaults to 600s.<br />Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/issues/11470 for more details. |  | Optional: \{\} <br /> |


#### MachineDeploymentStrategy



MachineDeploymentStrategy describes how to replace existing machines
with new ones.



_Appears in:_
- [MachineDeploymentClass](#machinedeploymentclass)
- [MachineDeploymentSpec](#machinedeploymentspec)
- [MachineDeploymentTopology](#machinedeploymenttopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[MachineDeploymentStrategyType](#machinedeploymentstrategytype)_ | type of deployment. Allowed values are RollingUpdate and OnDelete.<br />The default is RollingUpdate. |  | Enum: [RollingUpdate OnDelete] <br />Optional: \{\} <br /> |
| `rollingUpdate` _[MachineRollingUpdateDeployment](#machinerollingupdatedeployment)_ | rollingUpdate is the rolling update config params. Present only if<br />MachineDeploymentStrategyType = RollingUpdate. |  | Optional: \{\} <br /> |
| `remediation` _[RemediationStrategy](#remediationstrategy)_ | remediation controls the strategy of remediating unhealthy machines<br />and how remediating operations should occur during the lifecycle of the dependant MachineSets. |  | Optional: \{\} <br /> |


#### MachineDeploymentStrategyType

_Underlying type:_ _string_

MachineDeploymentStrategyType defines the type of MachineDeployment rollout strategies.



_Appears in:_
- [MachineDeploymentStrategy](#machinedeploymentstrategy)

| Field | Description |
| --- | --- |
| `RollingUpdate` | RollingUpdateMachineDeploymentStrategyType replaces the old MachineSet by new one using rolling update<br />i.e. gradually scale down the old MachineSet and scale up the new one.<br /> |
| `OnDelete` | OnDeleteMachineDeploymentStrategyType replaces old MachineSets when the deletion of the associated machines are completed.<br /> |


#### MachineDeploymentTopology



MachineDeploymentTopology specifies the different parameters for a set of worker nodes in the topology.
This set of nodes is managed by a MachineDeployment object whose lifecycle is managed by the Cluster controller.



_Appears in:_
- [WorkersTopology](#workerstopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `class` _string_ | class is the name of the MachineDeploymentClass used to create the set of worker nodes.<br />This should match one of the deployment classes defined in the ClusterClass object<br />mentioned in the `Cluster.Spec.Class` field. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `name` _string_ | name is the unique identifier for this MachineDeploymentTopology.<br />The value is used with other unique identifiers to create a MachineDeployment's Name<br />(e.g. cluster's name, etc). In case the name is greater than the allowed maximum length,<br />the values are hashed together. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `failureDomain` _string_ | failureDomain is the failure domain the machines will be created in.<br />Must match a key in the FailureDomains map stored on the cluster object. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of worker nodes belonging to this set.<br />If the value is nil, the MachineDeployment is created without the number of Replicas (defaulting to 1)<br />and it's assumed that an external entity (like cluster autoscaler) is responsible for the management<br />of this value. |  | Optional: \{\} <br /> |
| `machineHealthCheck` _[MachineHealthCheckTopology](#machinehealthchecktopology)_ | machineHealthCheck allows to enable, disable and override<br />the MachineHealthCheck configuration in the ClusterClass for this MachineDeployment. |  | Optional: \{\} <br /> |
| `nodeDrainTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: NodeDrainTimeout is different from `kubectl drain --timeout` |  | Optional: \{\} <br /> |
| `nodeVolumeDetachTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Optional: \{\} <br /> |
| `nodeDeletionTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds. |  | Optional: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a newly created machine should<br />be ready.<br />Defaults to 0 (machine will be considered available as soon as it<br />is ready) |  | Optional: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition.<br />This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready<br />computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.<br />If this field is not defined, readinessGates from the corresponding MachineDeploymentClass will be used, if any.<br />NOTE: This field is considered only for computing v1beta2 conditions. |  | MaxItems: 32 <br />Optional: \{\} <br /> |
| `strategy` _[MachineDeploymentStrategy](#machinedeploymentstrategy)_ | strategy is the deployment strategy to use to replace existing machines with<br />new ones. |  | Optional: \{\} <br /> |
| `variables` _[MachineDeploymentVariables](#machinedeploymentvariables)_ | variables can be used to customize the MachineDeployment through patches. |  | Optional: \{\} <br /> |


#### MachineDeploymentVariables



MachineDeploymentVariables can be used to provide variables for a specific MachineDeployment.



_Appears in:_
- [MachineDeploymentTopology](#machinedeploymenttopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `overrides` _[ClusterVariable](#clustervariable) array_ | overrides can be used to override Cluster level variables. |  | MaxItems: 1000 <br />Optional: \{\} <br /> |


#### MachineDrainRule



MachineDrainRule is the Schema for the MachineDrainRule API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `MachineDrainRule` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Required: \{\} <br /> |
| `spec` _[MachineDrainRuleSpec](#machinedrainrulespec)_ | spec defines the spec of a MachineDrainRule. |  | Required: \{\} <br /> |


#### MachineDrainRuleDrainBehavior

_Underlying type:_ _string_

MachineDrainRuleDrainBehavior defines the drain behavior. Can be either "Drain", "Skip", or "WaitCompleted".

_Validation:_
- Enum: [Drain Skip WaitCompleted]

_Appears in:_
- [MachineDrainRuleDrainConfig](#machinedrainruledrainconfig)

| Field | Description |
| --- | --- |
| `Drain` | MachineDrainRuleDrainBehaviorDrain means a Pod should be drained.<br /> |
| `Skip` | MachineDrainRuleDrainBehaviorSkip means the drain for a Pod should be skipped.<br /> |
| `WaitCompleted` | MachineDrainRuleDrainBehaviorWaitCompleted means the Pod should not be evicted,<br />but overall drain should wait until the Pod completes.<br /> |


#### MachineDrainRuleDrainConfig



MachineDrainRuleDrainConfig configures if and how Pods are drained.



_Appears in:_
- [MachineDrainRuleSpec](#machinedrainrulespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `behavior` _[MachineDrainRuleDrainBehavior](#machinedrainruledrainbehavior)_ | behavior defines the drain behavior.<br />Can be either "Drain", "Skip", or "WaitCompleted".<br />"Drain" means that the Pods to which this MachineDrainRule applies will be drained.<br />If behavior is set to "Drain" the order in which Pods are drained can be configured<br />with the order field. When draining Pods of a Node the Pods will be grouped by order<br />and one group after another will be drained (by increasing order). Cluster API will<br />wait until all Pods of a group are terminated / removed from the Node before starting<br />with the next group.<br />"Skip" means that the Pods to which this MachineDrainRule applies will be skipped during drain.<br />"WaitCompleted" means that the pods to which this MachineDrainRule applies will never be evicted<br />and we wait for them to be completed, it is enforced that pods marked with this behavior always have Order=0. |  | Enum: [Drain Skip WaitCompleted] <br />Required: \{\} <br /> |
| `order` _integer_ | order defines the order in which Pods are drained.<br />Pods with higher order are drained after Pods with lower order.<br />order can only be set if behavior is set to "Drain".<br />If order is not set, 0 will be used.<br />Valid values for order are from -2147483648 to 2147483647 (inclusive). |  | Optional: \{\} <br /> |


#### MachineDrainRuleMachineSelector



MachineDrainRuleMachineSelector defines to which Machines this MachineDrainRule should be applied.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDrainRuleSpec](#machinedrainrulespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | selector is a label selector which selects Machines by their labels.<br />This field follows standard label selector semantics; if not present or<br />empty, it selects all Machines.<br />If clusterSelector is also set, then the selector as a whole selects<br />Machines matching selector belonging to Clusters selected by clusterSelector.<br />If clusterSelector is not set, it selects all Machines matching selector in<br />all Clusters. |  | Optional: \{\} <br /> |
| `clusterSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | clusterSelector is a label selector which selects Machines by the labels of<br />their Clusters.<br />This field follows standard label selector semantics; if not present or<br />empty, it selects Machines of all Clusters.<br />If selector is also set, then the selector as a whole selects<br />Machines matching selector belonging to Clusters selected by clusterSelector.<br />If selector is not set, it selects all Machines belonging to Clusters<br />selected by clusterSelector. |  | Optional: \{\} <br /> |


#### MachineDrainRulePodSelector



MachineDrainRulePodSelector defines to which Pods this MachineDrainRule should be applied.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDrainRuleSpec](#machinedrainrulespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | selector is a label selector which selects Pods by their labels.<br />This field follows standard label selector semantics; if not present or<br />empty, it selects all Pods.<br />If namespaceSelector is also set, then the selector as a whole selects<br />Pods matching selector in Namespaces selected by namespaceSelector.<br />If namespaceSelector is not set, it selects all Pods matching selector in<br />all Namespaces. |  | Optional: \{\} <br /> |
| `namespaceSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | namespaceSelector is a label selector which selects Pods by the labels of<br />their Namespaces.<br />This field follows standard label selector semantics; if not present or<br />empty, it selects Pods of all Namespaces.<br />If selector is also set, then the selector as a whole selects<br />Pods matching selector in Namespaces selected by namespaceSelector.<br />If selector is not set, it selects all Pods in Namespaces selected by<br />namespaceSelector. |  | Optional: \{\} <br /> |


#### MachineDrainRuleSpec



MachineDrainRuleSpec defines the spec of a MachineDrainRule.



_Appears in:_
- [MachineDrainRule](#machinedrainrule)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `drain` _[MachineDrainRuleDrainConfig](#machinedrainruledrainconfig)_ | drain configures if and how Pods are drained. |  | Required: \{\} <br /> |
| `machines` _[MachineDrainRuleMachineSelector](#machinedrainrulemachineselector) array_ | machines defines to which Machines this MachineDrainRule should be applied.<br />If machines is not set, the MachineDrainRule applies to all Machines in the Namespace.<br />If machines contains multiple selectors, the results are ORed.<br />Within a single Machine selector the results of selector and clusterSelector are ANDed.<br />Machines will be selected from all Clusters in the Namespace unless otherwise<br />restricted with the clusterSelector.<br />Example: Selects control plane Machines in all Clusters or<br />         Machines with label "os" == "linux" in Clusters with label<br />         "stage" == "production".<br /> - selector:<br />     matchExpressions:<br />     - key: cluster.x-k8s.io/control-plane<br />       operator: Exists<br /> - selector:<br />     matchLabels:<br />       os: linux<br />   clusterSelector:<br />     matchExpressions:<br />     - key: stage<br />       operator: In<br />       values:<br />       - production |  | MaxItems: 32 <br />MinItems: 1 <br />MinProperties: 1 <br />Optional: \{\} <br /> |
| `pods` _[MachineDrainRulePodSelector](#machinedrainrulepodselector) array_ | pods defines to which Pods this MachineDrainRule should be applied.<br />If pods is not set, the MachineDrainRule applies to all Pods in all Namespaces.<br />If pods contains multiple selectors, the results are ORed.<br />Within a single Pod selector the results of selector and namespaceSelector are ANDed.<br />Pods will be selected from all Namespaces unless otherwise<br />restricted with the namespaceSelector.<br />Example: Selects Pods with label "app" == "logging" in all Namespaces or<br />         Pods with label "app" == "prometheus" in the "monitoring"<br />         Namespace.<br /> - selector:<br />     matchExpressions:<br />     - key: app<br />       operator: In<br />       values:<br />       - logging<br /> - selector:<br />     matchLabels:<br />       app: prometheus<br />   namespaceSelector:<br />     matchLabels:<br />       kubernetes.io/metadata.name: monitoring |  | MaxItems: 32 <br />MinItems: 1 <br />MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineHealthCheck



MachineHealthCheck is the Schema for the machinehealthchecks API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `MachineHealthCheck` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[MachineHealthCheckSpec](#machinehealthcheckspec)_ | spec is the specification of machine health check policy |  | Optional: \{\} <br /> |


#### MachineHealthCheckClass



MachineHealthCheckClass defines a MachineHealthCheck for a group of Machines.



_Appears in:_
- [ControlPlaneClass](#controlplaneclass)
- [MachineDeploymentClass](#machinedeploymentclass)
- [MachineHealthCheckTopology](#machinehealthchecktopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `unhealthyConditions` _[UnhealthyCondition](#unhealthycondition) array_ | unhealthyConditions contains a list of the conditions that determine<br />whether a node is considered unhealthy. The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the node is unhealthy. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `maxUnhealthy` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxUnhealthy specifies the maximum number of unhealthy machines allowed.<br />Any further remediation is only allowed if at most "maxUnhealthy" machines selected by<br />"selector" are not healthy. |  | Optional: \{\} <br /> |
| `unhealthyRange` _string_ | unhealthyRange specifies the range of unhealthy machines allowed.<br />Any further remediation is only allowed if the number of machines selected by "selector" as not healthy<br />is within the range of "unhealthyRange". Takes precedence over maxUnhealthy.<br />Eg. "[3-5]" - This means that remediation will be allowed only when:<br />(a) there are at least 3 unhealthy machines (and)<br />(b) there are at most 5 unhealthy machines |  | MaxLength: 32 <br />MinLength: 1 <br />Pattern: `^\[[0-9]+-[0-9]+\]$` <br />Optional: \{\} <br /> |
| `nodeStartupTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeStartupTimeout allows to set the maximum time for MachineHealthCheck<br />to consider a Machine unhealthy if a corresponding Node isn't associated<br />through a `Spec.ProviderID` field.<br />The duration set in this field is compared to the greatest of:<br />- Cluster's infrastructure ready condition timestamp (if and when available)<br />- Control Plane's initialized condition timestamp (if and when available)<br />- Machine's infrastructure ready condition timestamp (if and when available)<br />- Machine's metadata creation timestamp<br />Defaults to 10 minutes.<br />If you wish to disable this feature, set the value explicitly to 0. |  | Optional: \{\} <br /> |
| `remediationTemplate` _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectreference-v1-core)_ | remediationTemplate is a reference to a remediation template<br />provided by an infrastructure provider.<br />This field is completely optional, when filled, the MachineHealthCheck controller<br />creates a new object from the template referenced and hands off remediation of the machine to<br />a controller that lives outside of Cluster API. |  | Optional: \{\} <br /> |


#### MachineHealthCheckSpec



MachineHealthCheckSpec defines the desired state of MachineHealthCheck.



_Appears in:_
- [MachineHealthCheck](#machinehealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | selector is a label selector to match machines whose health will be exercised |  | Required: \{\} <br /> |
| `unhealthyConditions` _[UnhealthyCondition](#unhealthycondition) array_ | unhealthyConditions contains a list of the conditions that determine<br />whether a node is considered unhealthy.  The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the node is unhealthy. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `maxUnhealthy` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxUnhealthy specifies the maximum number of unhealthy machines allowed.<br />Any further remediation is only allowed if at most "maxUnhealthy" machines selected by<br />"selector" are not healthy.<br />Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/issues/10722 for more details. |  | Optional: \{\} <br /> |
| `unhealthyRange` _string_ | unhealthyRange specifies the range of unhealthy machines allowed.<br />Any further remediation is only allowed if the number of machines selected by "selector" as not healthy<br />is within the range of "unhealthyRange". Takes precedence over maxUnhealthy.<br />Eg. "[3-5]" - This means that remediation will be allowed only when:<br />(a) there are at least 3 unhealthy machines (and)<br />(b) there are at most 5 unhealthy machines<br />Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/issues/10722 for more details. |  | MaxLength: 32 <br />MinLength: 1 <br />Pattern: `^\[[0-9]+-[0-9]+\]$` <br />Optional: \{\} <br /> |
| `nodeStartupTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeStartupTimeout allows to set the maximum time for MachineHealthCheck<br />to consider a Machine unhealthy if a corresponding Node isn't associated<br />through a `Spec.ProviderID` field.<br />The duration set in this field is compared to the greatest of:<br />- Cluster's infrastructure ready condition timestamp (if and when available)<br />- Control Plane's initialized condition timestamp (if and when available)<br />- Machine's infrastructure ready condition timestamp (if and when available)<br />- Machine's metadata creation timestamp<br />Defaults to 10 minutes.<br />If you wish to disable this feature, set the value explicitly to 0. |  | Optional: \{\} <br /> |
| `remediationTemplate` _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectreference-v1-core)_ | remediationTemplate is a reference to a remediation template<br />provided by an infrastructure provider.<br />This field is completely optional, when filled, the MachineHealthCheck controller<br />creates a new object from the template referenced and hands off remediation of the machine to<br />a controller that lives outside of Cluster API. |  | Optional: \{\} <br /> |


#### MachineHealthCheckTopology



MachineHealthCheckTopology defines a MachineHealthCheck for a group of machines.



_Appears in:_
- [ControlPlaneTopology](#controlplanetopology)
- [MachineDeploymentTopology](#machinedeploymenttopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enable` _boolean_ | enable controls if a MachineHealthCheck should be created for the target machines.<br />If false: No MachineHealthCheck will be created.<br />If not set(default): A MachineHealthCheck will be created if it is defined here or<br /> in the associated ClusterClass. If no MachineHealthCheck is defined then none will be created.<br />If true: A MachineHealthCheck is guaranteed to be created. Cluster validation will<br />block if `enable` is true and no MachineHealthCheck definition is available. |  | Optional: \{\} <br /> |
| `unhealthyConditions` _[UnhealthyCondition](#unhealthycondition) array_ | unhealthyConditions contains a list of the conditions that determine<br />whether a node is considered unhealthy. The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the node is unhealthy. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `maxUnhealthy` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxUnhealthy specifies the maximum number of unhealthy machines allowed.<br />Any further remediation is only allowed if at most "maxUnhealthy" machines selected by<br />"selector" are not healthy. |  | Optional: \{\} <br /> |
| `unhealthyRange` _string_ | unhealthyRange specifies the range of unhealthy machines allowed.<br />Any further remediation is only allowed if the number of machines selected by "selector" as not healthy<br />is within the range of "unhealthyRange". Takes precedence over maxUnhealthy.<br />Eg. "[3-5]" - This means that remediation will be allowed only when:<br />(a) there are at least 3 unhealthy machines (and)<br />(b) there are at most 5 unhealthy machines |  | MaxLength: 32 <br />MinLength: 1 <br />Pattern: `^\[[0-9]+-[0-9]+\]$` <br />Optional: \{\} <br /> |
| `nodeStartupTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeStartupTimeout allows to set the maximum time for MachineHealthCheck<br />to consider a Machine unhealthy if a corresponding Node isn't associated<br />through a `Spec.ProviderID` field.<br />The duration set in this field is compared to the greatest of:<br />- Cluster's infrastructure ready condition timestamp (if and when available)<br />- Control Plane's initialized condition timestamp (if and when available)<br />- Machine's infrastructure ready condition timestamp (if and when available)<br />- Machine's metadata creation timestamp<br />Defaults to 10 minutes.<br />If you wish to disable this feature, set the value explicitly to 0. |  | Optional: \{\} <br /> |
| `remediationTemplate` _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectreference-v1-core)_ | remediationTemplate is a reference to a remediation template<br />provided by an infrastructure provider.<br />This field is completely optional, when filled, the MachineHealthCheck controller<br />creates a new object from the template referenced and hands off remediation of the machine to<br />a controller that lives outside of Cluster API. |  | Optional: \{\} <br /> |


#### MachineNamingStrategy



MachineNamingStrategy allows changing the naming pattern used when creating
Machines.
Note: InfraMachines & BootstrapConfigs will use the same name as the corresponding Machines.



_Appears in:_
- [MachineDeploymentSpec](#machinedeploymentspec)
- [MachineSetSpec](#machinesetspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the names of the<br />Machine objects.<br />If not defined, it will fallback to `\{\{ .machineSet.name \}\}-\{\{ .random \}\}`.<br />If the generated name string exceeds 63 characters, it will be trimmed to<br />58 characters and will<br />get concatenated with a random suffix of length 5.<br />Length of the template string must not exceed 256 characters.<br />The template allows the following variables `.cluster.name`,<br />`.machineSet.name` and `.random`.<br />The variable `.cluster.name` retrieves the name of the cluster object<br />that owns the Machines being created.<br />The variable `.machineSet.name` retrieves the name of the MachineSet<br />object that owns the Machines being created.<br />The variable `.random` is substituted with random alphanumeric string,<br />without vowels, of length 5. This variable is required part of the<br />template. If not provided, validation will fail. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |




#### MachinePool



MachinePool is the Schema for the machinepools API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `MachinePool` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[MachinePoolSpec](#machinepoolspec)_ | spec is the desired state of MachinePool. |  | Optional: \{\} <br /> |


#### MachinePoolClass



MachinePoolClass serves as a template to define a pool of worker nodes of the cluster
provisioned using `ClusterClass`.



_Appears in:_
- [WorkersClass](#workersclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `class` _string_ | class denotes a type of machine pool present in the cluster,<br />this name MUST be unique within a ClusterClass and can be referenced<br />in the Cluster to create a managed MachinePool. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `template` _[MachinePoolClassTemplate](#machinepoolclasstemplate)_ | template is a local struct containing a collection of templates for creation of<br />MachinePools objects representing a pool of worker nodes. |  | Required: \{\} <br /> |
| `failureDomains` _string array_ | failureDomains is the list of failure domains the MachinePool should be attached to.<br />Must match a key in the FailureDomains map stored on the cluster object.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `namingStrategy` _[MachinePoolClassNamingStrategy](#machinepoolclassnamingstrategy)_ | namingStrategy allows changing the naming pattern used when creating the MachinePool. |  | Optional: \{\} <br /> |
| `nodeDrainTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass. |  | Optional: \{\} <br /> |
| `nodeVolumeDetachTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass. |  | Optional: \{\} <br /> |
| `nodeDeletionTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine Pool is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass. |  | Optional: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a newly created machine pool should<br />be ready.<br />Defaults to 0 (machine will be considered available as soon as it<br />is ready)<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass. |  | Optional: \{\} <br /> |


#### MachinePoolClassNamingStrategy



MachinePoolClassNamingStrategy defines the naming strategy for machine pool objects.



_Appears in:_
- [MachinePoolClass](#machinepoolclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the name of the MachinePool object.<br />If not defined, it will fallback to `\{\{ .cluster.name \}\}-\{\{ .machinePool.topologyName \}\}-\{\{ .random \}\}`.<br />If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will<br />get concatenated with a random suffix of length 5.<br />The templating mechanism provides the following arguments:<br />* `.cluster.name`: The name of the cluster object.<br />* `.random`: A random alphanumeric string, without vowels, of length 5.<br />* `.machinePool.topologyName`: The name of the MachinePool topology (Cluster.spec.topology.workers.machinePools[].name). |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### MachinePoolClassTemplate



MachinePoolClassTemplate defines how a MachinePool generated from a MachinePoolClass
should look like.



_Appears in:_
- [MachinePoolClass](#machinepoolclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `bootstrap` _[LocalObjectTemplate](#localobjecttemplate)_ | bootstrap contains the bootstrap template reference to be used<br />for the creation of the Machines in the MachinePool. |  | Required: \{\} <br /> |
| `infrastructure` _[LocalObjectTemplate](#localobjecttemplate)_ | infrastructure contains the infrastructure template reference to be used<br />for the creation of the MachinePool. |  | Required: \{\} <br /> |




#### MachinePoolSpec



MachinePoolSpec defines the desired state of MachinePool.



_Appears in:_
- [MachinePool](#machinepool)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of desired machines. Defaults to 1.<br />This is a pointer to distinguish between explicit zero and not specified. |  | Optional: \{\} <br /> |
| `template` _[MachineTemplateSpec](#machinetemplatespec)_ | template describes the machines that will be created. |  | Required: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a newly created machine instances should<br />be ready.<br />Defaults to 0 (machine instance will be considered available as soon as it<br />is ready) |  | Optional: \{\} <br /> |
| `providerIDList` _string array_ | providerIDList are the identification IDs of machine instances provided by the provider.<br />This field must match the provider IDs as seen on the node objects corresponding to a machine pool's machine instances. |  | MaxItems: 10000 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `failureDomains` _string array_ | failureDomains is the list of failure domains this MachinePool should be attached to. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### MachinePoolTopology



MachinePoolTopology specifies the different parameters for a pool of worker nodes in the topology.
This pool of nodes is managed by a MachinePool object whose lifecycle is managed by the Cluster controller.



_Appears in:_
- [WorkersTopology](#workerstopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `class` _string_ | class is the name of the MachinePoolClass used to create the pool of worker nodes.<br />This should match one of the deployment classes defined in the ClusterClass object<br />mentioned in the `Cluster.Spec.Class` field. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `name` _string_ | name is the unique identifier for this MachinePoolTopology.<br />The value is used with other unique identifiers to create a MachinePool's Name<br />(e.g. cluster's name, etc). In case the name is greater than the allowed maximum length,<br />the values are hashed together. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `failureDomains` _string array_ | failureDomains is the list of failure domains the machine pool will be created in.<br />Must match a key in the FailureDomains map stored on the cluster object. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `nodeDrainTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: NodeDrainTimeout is different from `kubectl drain --timeout` |  | Optional: \{\} <br /> |
| `nodeVolumeDetachTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Optional: \{\} <br /> |
| `nodeDeletionTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the MachinePool<br />hosts after the MachinePool is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds. |  | Optional: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a newly created machine pool should<br />be ready.<br />Defaults to 0 (machine will be considered available as soon as it<br />is ready) |  | Optional: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of nodes belonging to this pool.<br />If the value is nil, the MachinePool is created without the number of Replicas (defaulting to 1)<br />and it's assumed that an external entity (like cluster autoscaler) is responsible for the management<br />of this value. |  | Optional: \{\} <br /> |
| `variables` _[MachinePoolVariables](#machinepoolvariables)_ | variables can be used to customize the MachinePool through patches. |  | Optional: \{\} <br /> |


#### MachinePoolVariables



MachinePoolVariables can be used to provide variables for a specific MachinePool.



_Appears in:_
- [MachinePoolTopology](#machinepooltopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `overrides` _[ClusterVariable](#clustervariable) array_ | overrides can be used to override Cluster level variables. |  | MaxItems: 1000 <br />Optional: \{\} <br /> |


#### MachineReadinessGate



MachineReadinessGate contains the type of a Machine condition to be used as a readiness gate.



_Appears in:_
- [ControlPlaneClass](#controlplaneclass)
- [ControlPlaneTopology](#controlplanetopology)
- [KubeadmControlPlaneMachineTemplate](#kubeadmcontrolplanemachinetemplate)
- [MachineDeploymentClass](#machinedeploymentclass)
- [MachineDeploymentTopology](#machinedeploymenttopology)
- [MachineSpec](#machinespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditionType` _string_ | conditionType refers to a condition with matching type in the Machine's condition list.<br />If the conditions doesn't exist, it will be treated as unknown.<br />Note: Both Cluster API conditions or conditions added by 3rd party controllers can be used as readiness gates. |  | MaxLength: 316 <br />MinLength: 1 <br />Pattern: `^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$` <br />Required: \{\} <br /> |
| `polarity` _[ConditionPolarity](#conditionpolarity)_ | polarity of the conditionType specified in this readinessGate.<br />Valid values are Positive, Negative and omitted.<br />When omitted, the default behaviour will be Positive.<br />A positive polarity means that the condition should report a true status under normal conditions.<br />A negative polarity means that the condition should report a false status under normal conditions. |  | Enum: [Positive Negative] <br />Optional: \{\} <br /> |


#### MachineRollingUpdateDeployment

_Underlying type:_ _[struct{MaxUnavailable *k8s.io/apimachinery/pkg/util/intstr.IntOrString "json:\"maxUnavailable,omitempty\""; MaxSurge *k8s.io/apimachinery/pkg/util/intstr.IntOrString "json:\"maxSurge,omitempty\""; DeletePolicy *string "json:\"deletePolicy,omitempty\""}](#struct{maxunavailable-*k8sioapimachinerypkgutilintstrintorstring-"json:\"maxunavailable,omitempty\"";-maxsurge-*k8sioapimachinerypkgutilintstrintorstring-"json:\"maxsurge,omitempty\"";-deletepolicy-*string-"json:\"deletepolicy,omitempty\""})_

MachineRollingUpdateDeployment is used to control the desired behavior of rolling update.



_Appears in:_
- [MachineDeploymentStrategy](#machinedeploymentstrategy)



#### MachineSet



MachineSet is the Schema for the machinesets API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `MachineSet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[MachineSetSpec](#machinesetspec)_ | spec is the desired state of MachineSet. |  | Optional: \{\} <br /> |






#### MachineSetSpec



MachineSetSpec defines the desired state of MachineSet.



_Appears in:_
- [MachineSet](#machineset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of desired replicas.<br />This is a pointer to distinguish between explicit zero and unspecified.<br />Defaults to:<br />* if the Kubernetes autoscaler min size and max size annotations are set:<br />  - if it's a new MachineSet, use min size<br />  - if the replicas field of the old MachineSet is < min size, use min size<br />  - if the replicas field of the old MachineSet is > max size, use max size<br />  - if the replicas field of the old MachineSet is in the (min size, max size) range, keep the value from the oldMS<br />* otherwise use 1<br />Note: Defaulting will be run whenever the replicas field is not set:<br />* A new MachineSet is created with replicas not set.<br />* On an existing MachineSet the replicas field was first set and is now unset.<br />Those cases are especially relevant for the following Kubernetes autoscaler use cases:<br />* A new MachineSet is created and replicas should be managed by the autoscaler<br />* An existing MachineSet which initially wasn't controlled by the autoscaler<br />  should be later controlled by the autoscaler |  | Optional: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a Node for a newly created machine should be ready before considering the replica available.<br />Defaults to 0 (machine will be considered available as soon as the Node is ready) |  | Optional: \{\} <br /> |
| `deletePolicy` _string_ | deletePolicy defines the policy used to identify nodes to delete when downscaling.<br />Defaults to "Random".  Valid values are "Random, "Newest", "Oldest" |  | Enum: [Random Newest Oldest] <br />Optional: \{\} <br /> |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | selector is a label query over machines that should match the replica count.<br />Label keys and values that must match in order to be controlled by this MachineSet.<br />It must match the machine template's labels.<br />More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors |  | Required: \{\} <br /> |
| `template` _[MachineTemplateSpec](#machinetemplatespec)_ | template is the object that describes the machine that will be created if<br />insufficient replicas are detected.<br />Object references to custom resources are treated as templates. |  | Optional: \{\} <br /> |
| `machineNamingStrategy` _[MachineNamingStrategy](#machinenamingstrategy)_ | machineNamingStrategy allows changing the naming pattern used when creating Machines.<br />Note: InfraMachines & BootstrapConfigs will use the same name as the corresponding Machines. |  | Optional: \{\} <br /> |


#### MachineSpec



MachineSpec defines the desired state of Machine.



_Appears in:_
- [Machine](#machine)
- [MachineTemplateSpec](#machinetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `bootstrap` _[Bootstrap](#bootstrap)_ | bootstrap is a reference to a local struct which encapsulates<br />fields to configure the Machine’s bootstrapping mechanism. |  | Required: \{\} <br /> |
| `infrastructureRef` _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectreference-v1-core)_ | infrastructureRef is a required reference to a custom resource<br />offered by an infrastructure provider. |  | Required: \{\} <br /> |
| `version` _string_ | version defines the desired Kubernetes version.<br />This field is meant to be optionally used by bootstrap providers. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `providerID` _string_ | providerID is the identification ID of the machine provided by the provider.<br />This field must match the provider ID as seen on the node object corresponding to this machine.<br />This field is required by higher level consumers of cluster-api. Example use case is cluster autoscaler<br />with cluster-api as provider. Clean-up logic in the autoscaler compares machines to nodes to find out<br />machines at provider which could not get registered as Kubernetes nodes. With cluster-api as a<br />generic out-of-tree provider for autoscaler, this field is required by autoscaler to be<br />able to have a provider view of the list of machines. Another list of nodes is queried from the k8s apiserver<br />and then a comparison is done to find out unregistered machines and are marked for delete.<br />This field will be set by the actuators and consumed by higher level entities like autoscaler that will<br />be interfacing with cluster-api as generic provider. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `failureDomain` _string_ | failureDomain is the failure domain the machine will be created in.<br />Must match a key in the FailureDomains map stored on the cluster object. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition.<br />This field can be used e.g. by Cluster API control plane providers to extend the semantic of the<br />Ready condition for the Machine they control, like the kubeadm control provider adding ReadinessGates<br />for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc.<br />Another example are external controllers, e.g. responsible to install special software/hardware on the Machines;<br />they can include the status of those components with a new condition and add this condition to ReadinessGates.<br />NOTE: This field is considered only for computing v1beta2 conditions.<br />NOTE: In case readinessGates conditions start with the APIServer, ControllerManager, Scheduler prefix, and all those<br />readiness gates condition are reporting the same message, when computing the Machine's Ready condition those<br />readinessGates will be replaced by a single entry reporting "Control plane components: " + message.<br />This helps to improve readability of conditions bubbling up to the Machine's owner resource / to the Cluster). |  | MaxItems: 32 <br />Optional: \{\} <br /> |
| `nodeDrainTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDrainTimeout is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: NodeDrainTimeout is different from `kubectl drain --timeout` |  | Optional: \{\} <br /> |
| `nodeVolumeDetachTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Optional: \{\} <br /> |
| `nodeDeletionTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDeletionTimeout defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds. |  | Optional: \{\} <br /> |


#### MachineTemplateSpec



MachineTemplateSpec describes the data needed to create a Machine from a template.



_Appears in:_
- [MachineDeploymentSpec](#machinedeploymentspec)
- [MachinePoolSpec](#machinepoolspec)
- [MachineSetSpec](#machinesetspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[MachineSpec](#machinespec)_ | spec is the specification of the desired behavior of the machine.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status |  | Optional: \{\} <br /> |


#### NetworkRanges



NetworkRanges represents ranges of network addresses.



_Appears in:_
- [ClusterNetwork](#clusternetwork)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `cidrBlocks` _string array_ | cidrBlocks is a list of CIDR blocks. |  | MaxItems: 100 <br />items:MaxLength: 43 <br />items:MinLength: 1 <br />Required: \{\} <br /> |


#### ObjectMeta



ObjectMeta is metadata that all persisted resources must have, which includes all objects
users must create. This is a copy of customizable fields from metav1.ObjectMeta.

ObjectMeta is embedded in `Machine.Spec`, `MachineDeployment.Template` and `MachineSet.Template`,
which are not top-level Kubernetes objects. Given that metav1.ObjectMeta has lots of special cases
and read-only fields which end up in the generated CRD validation, having it as a subset simplifies
the API and some issues that can impact user experience.

During the [upgrade to controller-tools@v2](https://github.com/kubernetes-sigs/cluster-api/pull/1054)
for v1alpha2, we noticed a failure would occur running Cluster API test suite against the new CRDs,
specifically `spec.metadata.creationTimestamp in body must be of type string: "null"`.
The investigation showed that `controller-tools@v2` behaves differently than its previous version
when handling types from [metav1](k8s.io/apimachinery/pkg/apis/meta/v1) package.

In more details, we found that embedded (non-top level) types that embedded `metav1.ObjectMeta`
had validation properties, including for `creationTimestamp` (metav1.Time).
The `metav1.Time` type specifies a custom json marshaller that, when IsZero() is true, returns `null`
which breaks validation because the field isn't marked as nullable.

In future versions, controller-tools@v2 might allow overriding the type and validation for embedded
types. When that happens, this hack should be revisited.



_Appears in:_
- [ControlPlaneClass](#controlplaneclass)
- [ControlPlaneTopology](#controlplanetopology)
- [KubeadmConfigTemplateResource](#kubeadmconfigtemplateresource)
- [KubeadmControlPlaneMachineTemplate](#kubeadmcontrolplanemachinetemplate)
- [KubeadmControlPlaneTemplateMachineTemplate](#kubeadmcontrolplanetemplatemachinetemplate)
- [KubeadmControlPlaneTemplateResource](#kubeadmcontrolplanetemplateresource)
- [MachineDeploymentClassTemplate](#machinedeploymentclasstemplate)
- [MachineDeploymentTopology](#machinedeploymenttopology)
- [MachinePoolClassTemplate](#machinepoolclasstemplate)
- [MachinePoolTopology](#machinepooltopology)
- [MachineTemplateSpec](#machinetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `labels` _object (keys:string, values:string)_ | labels is a map of string keys and values that can be used to organize and categorize<br />(scope and select) objects. May match selectors of replication controllers<br />and services.<br />More info: http://kubernetes.io/docs/user-guide/labels |  | Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ | annotations is an unstructured key value map stored with a resource that may be<br />set by external tools to store and retrieve arbitrary metadata. They are not<br />queryable and should be preserved when modifying objects.<br />More info: http://kubernetes.io/docs/user-guide/annotations |  | Optional: \{\} <br /> |


#### PatchDefinition



PatchDefinition defines a patch which is applied to customize the referenced templates.



_Appears in:_
- [ClusterClassPatch](#clusterclasspatch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[PatchSelector](#patchselector)_ | selector defines on which templates the patch should be applied. |  | Required: \{\} <br /> |
| `jsonPatches` _[JSONPatch](#jsonpatch) array_ | jsonPatches defines the patches which should be applied on the templates<br />matching the selector.<br />Note: Patches will be applied in the order of the array. |  | MaxItems: 100 <br />Required: \{\} <br /> |


#### PatchSelector



PatchSelector defines on which templates the patch should be applied.
Note: Matching on APIVersion and Kind is mandatory, to enforce that the patches are
written for the correct version. The version of the references in the ClusterClass may
be automatically updated during reconciliation if there is a newer version for the same contract.
Note: The results of selection based on the individual fields are ANDed.



_Appears in:_
- [PatchDefinition](#patchdefinition)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | apiVersion filters templates by apiVersion. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `kind` _string_ | kind filters templates by kind. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `matchResources` _[PatchSelectorMatch](#patchselectormatch)_ | matchResources selects templates based on where they are referenced. |  | Required: \{\} <br /> |


#### PatchSelectorMatch



PatchSelectorMatch selects templates based on where they are referenced.
Note: The selector must match at least one template.
Note: The results of selection based on the individual fields are ORed.



_Appears in:_
- [PatchSelector](#patchselector)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `controlPlane` _boolean_ | controlPlane selects templates referenced in .spec.ControlPlane.<br />Note: this will match the controlPlane and also the controlPlane<br />machineInfrastructure (depending on the kind and apiVersion). |  | Optional: \{\} <br /> |
| `infrastructureCluster` _boolean_ | infrastructureCluster selects templates referenced in .spec.infrastructure. |  | Optional: \{\} <br /> |
| `machineDeploymentClass` _[PatchSelectorMatchMachineDeploymentClass](#patchselectormatchmachinedeploymentclass)_ | machineDeploymentClass selects templates referenced in specific MachineDeploymentClasses in<br />.spec.workers.machineDeployments. |  | Optional: \{\} <br /> |
| `machinePoolClass` _[PatchSelectorMatchMachinePoolClass](#patchselectormatchmachinepoolclass)_ | machinePoolClass selects templates referenced in specific MachinePoolClasses in<br />.spec.workers.machinePools. |  | Optional: \{\} <br /> |


#### PatchSelectorMatchMachineDeploymentClass



PatchSelectorMatchMachineDeploymentClass selects templates referenced
in specific MachineDeploymentClasses in .spec.workers.machineDeployments.



_Appears in:_
- [PatchSelectorMatch](#patchselectormatch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `names` _string array_ | names selects templates by class names. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### PatchSelectorMatchMachinePoolClass



PatchSelectorMatchMachinePoolClass selects templates referenced
in specific MachinePoolClasses in .spec.workers.machinePools.



_Appears in:_
- [PatchSelectorMatch](#patchselectormatch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `names` _string array_ | names selects templates by class names. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### RemediationStrategy

_Underlying type:_ _[struct{MaxInFlight *k8s.io/apimachinery/pkg/util/intstr.IntOrString "json:\"maxInFlight,omitempty\""}](#struct{maxinflight-*k8sioapimachinerypkgutilintstrintorstring-"json:\"maxinflight,omitempty\""})_

RemediationStrategy allows to define how the MachineSet can control scaling operations.



_Appears in:_
- [MachineDeploymentStrategy](#machinedeploymentstrategy)



#### Topology



Topology encapsulates the information of the managed resources.



_Appears in:_
- [ClusterSpec](#clusterspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `class` _string_ | class is the name of the ClusterClass object to create the topology. |  | MaxLength: 253 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `classNamespace` _string_ | classNamespace is the namespace of the ClusterClass that should be used for the topology.<br />If classNamespace is empty or not set, it is defaulted to the namespace of the Cluster object.<br />classNamespace must be a valid namespace name and because of that be at most 63 characters in length<br />and it must consist only of lower case alphanumeric characters or hyphens (-), and must start<br />and end with an alphanumeric character. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `version` _string_ | version is the Kubernetes version of the cluster. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `controlPlane` _[ControlPlaneTopology](#controlplanetopology)_ | controlPlane describes the cluster control plane. |  | Optional: \{\} <br /> |
| `workers` _[WorkersTopology](#workerstopology)_ | workers encapsulates the different constructs that form the worker nodes<br />for the cluster. |  | Optional: \{\} <br /> |
| `variables` _[ClusterVariable](#clustervariable) array_ | variables can be used to customize the Cluster through<br />patches. They must comply to the corresponding<br />VariableClasses defined in the ClusterClass. |  | MaxItems: 1000 <br />Optional: \{\} <br /> |


#### UnhealthyCondition



UnhealthyCondition represents a Node condition type and value with a timeout
specified as a duration.  When the named condition has been in the given
status for at least the timeout value, a node is considered unhealthy.



_Appears in:_
- [MachineHealthCheckClass](#machinehealthcheckclass)
- [MachineHealthCheckSpec](#machinehealthcheckspec)
- [MachineHealthCheckTopology](#machinehealthchecktopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[NodeConditionType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#nodeconditiontype-v1-core)_ | type of Node condition |  | MinLength: 1 <br />Type: string <br />Required: \{\} <br /> |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | timeout is the duration that a node must be in a given status for,<br />after which the node is considered unhealthy.<br />For example, with a value of "1h", the node must match the status<br />for at least 1 hour before being considered unhealthy. |  | Required: \{\} <br /> |




#### VariableSchema



VariableSchema defines the schema of a variable.



_Appears in:_
- [ClusterClassStatusVariableDefinition](#clusterclassstatusvariabledefinition)
- [ClusterClassVariable](#clusterclassvariable)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `openAPIV3Schema` _[JSONSchemaProps](#jsonschemaprops)_ | openAPIV3Schema defines the schema of a variable via OpenAPI v3<br />schema. The schema is a subset of the schema used in<br />Kubernetes CRDs. |  | Required: \{\} <br /> |




#### WorkersClass



WorkersClass is a collection of deployment classes.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `machineDeployments` _[MachineDeploymentClass](#machinedeploymentclass) array_ | machineDeployments is a list of machine deployment classes that can be used to create<br />a set of worker nodes. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `machinePools` _[MachinePoolClass](#machinepoolclass) array_ | machinePools is a list of machine pool classes that can be used to create<br />a set of worker nodes. |  | MaxItems: 100 <br />Optional: \{\} <br /> |


#### WorkersTopology



WorkersTopology represents the different sets of worker nodes in the cluster.



_Appears in:_
- [Topology](#topology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `machineDeployments` _[MachineDeploymentTopology](#machinedeploymenttopology) array_ | machineDeployments is a list of machine deployments in the cluster. |  | MaxItems: 2000 <br />Optional: \{\} <br /> |
| `machinePools` _[MachinePoolTopology](#machinepooltopology) array_ | machinePools is a list of machine pools in the cluster. |  | MaxItems: 2000 <br />Optional: \{\} <br /> |



## cluster.x-k8s.io/v1beta2

Package v1beta2 contains API Schema definitions for the cluster v1beta2 API group

### Resource Types
- [Cluster](#cluster)
- [ClusterClass](#clusterclass)
- [Machine](#machine)
- [MachineDeployment](#machinedeployment)
- [MachineDrainRule](#machinedrainrule)
- [MachineHealthCheck](#machinehealthcheck)
- [MachinePool](#machinepool)
- [MachineSet](#machineset)



#### APIEndpoint



APIEndpoint represents a reachable Kubernetes API endpoint.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ClusterSpec](#clusterspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `host` _string_ | host is the hostname on which the API server is serving. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the port on which the API server is serving. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |


#### Bootstrap



Bootstrap encapsulates fields to configure the Machine’s bootstrapping mechanism.



_Appears in:_
- [MachineSpec](#machinespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `configRef` _[ContractVersionedObjectReference](#contractversionedobjectreference)_ | configRef is a reference to a bootstrap provider-specific resource<br />that holds configuration details. The reference is optional to<br />allow users/operators to specify Bootstrap.DataSecretName without<br />the need of a controller. |  | Optional: \{\} <br /> |
| `dataSecretName` _string_ | dataSecretName is the name of the secret that stores the bootstrap data script.<br />If nil, the Machine should remain in the Pending state. |  | MaxLength: 253 <br />MinLength: 0 <br />Optional: \{\} <br /> |


#### Cluster



Cluster is the Schema for the clusters API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `Cluster` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[ClusterSpec](#clusterspec)_ | spec is the desired state of Cluster. |  | MinProperties: 1 <br />Required: \{\} <br /> |


#### ClusterAvailabilityGate



ClusterAvailabilityGate contains the type of a Cluster condition to be used as availability gate.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)
- [ClusterSpec](#clusterspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditionType` _string_ | conditionType refers to a condition with matching type in the Cluster's condition list.<br />If the conditions doesn't exist, it will be treated as unknown.<br />Note: Both Cluster API conditions or conditions added by 3rd party controllers can be used as availability gates. |  | MaxLength: 316 <br />MinLength: 1 <br />Pattern: `^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$` <br />Required: \{\} <br /> |
| `polarity` _[ConditionPolarity](#conditionpolarity)_ | polarity of the conditionType specified in this availabilityGate.<br />Valid values are Positive, Negative and omitted.<br />When omitted, the default behaviour will be Positive.<br />A positive polarity means that the condition should report a true status under normal conditions.<br />A negative polarity means that the condition should report a false status under normal conditions. |  | Enum: [Positive Negative] <br />Optional: \{\} <br /> |


#### ClusterClass



ClusterClass is a template which can be used to create managed topologies.
NOTE: This CRD can only be used if the ClusterTopology feature gate is enabled.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `ClusterClass` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[ClusterClassSpec](#clusterclassspec)_ | spec is the desired state of ClusterClass. |  | Required: \{\} <br /> |


#### ClusterClassPatch



ClusterClassPatch defines a patch which is applied to customize the referenced templates.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the patch. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `description` _string_ | description is a human-readable description of this patch. |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `enabledIf` _string_ | enabledIf is a Go template to be used to calculate if a patch should be enabled.<br />It can reference variables defined in .spec.variables and builtin variables.<br />The patch will be enabled if the template evaluates to `true`, otherwise it will<br />be disabled.<br />If EnabledIf is not set, the patch will be enabled per default. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `definitions` _[PatchDefinition](#patchdefinition) array_ | definitions define inline patches.<br />Note: Patches will be applied in the order of the array.<br />Note: Exactly one of Definitions or External must be set. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `external` _[ExternalPatchDefinition](#externalpatchdefinition)_ | external defines an external patch.<br />Note: Exactly one of Definitions or External must be set. |  | Optional: \{\} <br /> |


#### ClusterClassRef



ClusterClassRef is the ref to the ClusterClass that should be used for the topology.



_Appears in:_
- [Topology](#topology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name is the name of the ClusterClass that should be used for the topology.<br />name must be a valid ClusterClass name and because of that be at most 253 characters in length<br />and it must consist only of lower case alphanumeric characters, hyphens (-) and periods (.), and must start<br />and end with an alphanumeric character. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` <br />Required: \{\} <br /> |
| `namespace` _string_ | namespace is the namespace of the ClusterClass that should be used for the topology.<br />If namespace is empty or not set, it is defaulted to the namespace of the Cluster object.<br />namespace must be a valid namespace name and because of that be at most 63 characters in length<br />and it must consist only of lower case alphanumeric characters or hyphens (-), and must start<br />and end with an alphanumeric character. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |


#### ClusterClassSpec



ClusterClassSpec describes the desired state of the ClusterClass.



_Appears in:_
- [ClusterClass](#clusterclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `availabilityGates` _[ClusterAvailabilityGate](#clusteravailabilitygate) array_ | availabilityGates specifies additional conditions to include when evaluating Cluster Available condition.<br />NOTE: If a Cluster is using this ClusterClass, and this Cluster defines a custom list of availabilityGates,<br />such list overrides availabilityGates defined in this field. |  | MaxItems: 32 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `infrastructure` _[InfrastructureClass](#infrastructureclass)_ | infrastructure is a reference to a local struct that holds the details<br />for provisioning the infrastructure cluster for the Cluster. |  | Required: \{\} <br /> |
| `controlPlane` _[ControlPlaneClass](#controlplaneclass)_ | controlPlane is a reference to a local struct that holds the details<br />for provisioning the Control Plane for the Cluster. |  | Required: \{\} <br /> |
| `workers` _[WorkersClass](#workersclass)_ | workers describes the worker nodes for the cluster.<br />It is a collection of node types which can be used to create<br />the worker nodes of the cluster. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `variables` _[ClusterClassVariable](#clusterclassvariable) array_ | variables defines the variables which can be configured<br />in the Cluster topology and are then used in patches. |  | MaxItems: 1000 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `patches` _[ClusterClassPatch](#clusterclasspatch) array_ | patches defines the patches which are applied to customize<br />referenced templates of a ClusterClass.<br />Note: Patches will be applied in the order of the array. |  | MaxItems: 1000 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `upgrade` _[ClusterClassUpgrade](#clusterclassupgrade)_ | upgrade defines the upgrade configuration for clusters using this ClusterClass. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `kubernetesVersions` _string array_ | kubernetesVersions is the list of Kubernetes versions that can be<br />used for clusters using this ClusterClass.<br />The list of version must be ordered from the older to the newer version, and there should be<br />at least one version for every minor in between the first and the last version. |  | MaxItems: 100 <br />MinItems: 1 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |




#### ClusterClassStatusVariableDefinition



ClusterClassStatusVariableDefinition defines a variable which appears in the status of a ClusterClass.



_Appears in:_
- [ClusterClassStatusVariable](#clusterclassstatusvariable)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `from` _string_ | from specifies the origin of the variable definition.<br />This will be `inline` for variables defined in the ClusterClass or the name of a patch defined in the ClusterClass<br />for variables discovered from a DiscoverVariables runtime extensions. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `required` _boolean_ | required specifies if the variable is required.<br />Note: this applies to the variable as a whole and thus the<br />top-level object defined in the schema. If nested fields are<br />required, this will be specified inside the schema. |  | Required: \{\} <br /> |
| `deprecatedV1Beta1Metadata` _[ClusterClassVariableMetadata](#clusterclassvariablemetadata)_ | deprecatedV1Beta1Metadata is the metadata of a variable.<br />It can be used to add additional data for higher level tools to<br />a ClusterClassVariable.<br />Deprecated: This field is deprecated and will be removed when support for v1beta1 will be dropped. Please use XMetadata in JSONSchemaProps instead. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `schema` _[VariableSchema](#variableschema)_ | schema defines the schema of the variable. |  | Required: \{\} <br /> |


#### ClusterClassTemplateReference



ClusterClassTemplateReference is a reference to a ClusterClass template.



_Appears in:_
- [ControlPlaneClass](#controlplaneclass)
- [ControlPlaneClassMachineInfrastructureTemplate](#controlplaneclassmachineinfrastructuretemplate)
- [InfrastructureClass](#infrastructureclass)
- [MachineDeploymentClassBootstrapTemplate](#machinedeploymentclassbootstraptemplate)
- [MachineDeploymentClassInfrastructureTemplate](#machinedeploymentclassinfrastructuretemplate)
- [MachinePoolClassBootstrapTemplate](#machinepoolclassbootstraptemplate)
- [MachinePoolClassInfrastructureTemplate](#machinepoolclassinfrastructuretemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _string_ | kind of the template.<br />kind must consist of alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$` <br />Required: \{\} <br /> |
| `name` _string_ | name of the template.<br />name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` <br />Required: \{\} <br /> |
| `apiVersion` _string_ | apiVersion of the template.<br />apiVersion must be fully qualified domain name followed by / and a version. |  | MaxLength: 317 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/[a-z]([-a-z0-9]*[a-z0-9])?$` <br />Required: \{\} <br /> |


#### ClusterClassUpgrade



ClusterClassUpgrade defines the upgrade configuration for clusters using the ClusterClass.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ClusterClassSpec](#clusterclassspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `external` _[ClusterClassUpgradeExternal](#clusterclassupgradeexternal)_ | external defines external runtime extensions for upgrade operations. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### ClusterClassUpgradeExternal



ClusterClassUpgradeExternal defines external runtime extensions for upgrade operations.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ClusterClassUpgrade](#clusterclassupgrade)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `generateUpgradePlanExtension` _string_ | generateUpgradePlanExtension references an extension which is called to generate upgrade plan. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### ClusterClassVariable



ClusterClassVariable defines a variable which can
be configured in the Cluster topology and used in patches.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the variable. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `required` _boolean_ | required specifies if the variable is required.<br />Note: this applies to the variable as a whole and thus the<br />top-level object defined in the schema. If nested fields are<br />required, this will be specified inside the schema. |  | Required: \{\} <br /> |
| `deprecatedV1Beta1Metadata` _[ClusterClassVariableMetadata](#clusterclassvariablemetadata)_ | deprecatedV1Beta1Metadata is the metadata of a variable.<br />It can be used to add additional data for higher level tools to<br />a ClusterClassVariable.<br />Deprecated: This field is deprecated and will be removed when support for v1beta1 will be dropped. Please use XMetadata in JSONSchemaProps instead. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `schema` _[VariableSchema](#variableschema)_ | schema defines the schema of the variable. |  | Required: \{\} <br /> |


#### ClusterClassVariableMetadata



ClusterClassVariableMetadata is the metadata of a variable.
It can be used to add additional data for higher level tools to
a ClusterClassVariable.

Deprecated: This struct is deprecated and is going to be removed in the next apiVersion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ClusterClassStatusVariableDefinition](#clusterclassstatusvariabledefinition)
- [ClusterClassVariable](#clusterclassvariable)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `labels` _object (keys:string, values:string)_ | labels is a map of string keys and values that can be used to organize and categorize<br />(scope and select) variables. |  | Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ | annotations is an unstructured key value map that can be used to store and<br />retrieve arbitrary metadata.<br />They are not queryable. |  | Optional: \{\} <br /> |


#### ClusterNetwork



ClusterNetwork specifies the different networking
parameters for a cluster.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ClusterSpec](#clusterspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiServerPort` _integer_ | apiServerPort specifies the port the API Server should bind to.<br />Defaults to 6443. |  | Maximum: 65535 <br />Minimum: 1 <br />Optional: \{\} <br /> |
| `services` _[NetworkRanges](#networkranges)_ | services is the network ranges from which service VIPs are allocated. |  | Optional: \{\} <br /> |
| `pods` _[NetworkRanges](#networkranges)_ | pods is the network ranges from which Pod networks are allocated. |  | Optional: \{\} <br /> |
| `serviceDomain` _string_ | serviceDomain is the domain name for services. |  | MaxLength: 253 <br />MinLength: 1 <br />Optional: \{\} <br /> |




#### ClusterSpec



ClusterSpec defines the desired state of Cluster.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Cluster](#cluster)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `paused` _boolean_ | paused can be used to prevent controllers from processing the Cluster and all its associated objects. |  | Optional: \{\} <br /> |
| `clusterNetwork` _[ClusterNetwork](#clusternetwork)_ | clusterNetwork represents the cluster network configuration. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `controlPlaneEndpoint` _[APIEndpoint](#apiendpoint)_ | controlPlaneEndpoint represents the endpoint used to communicate with the control plane. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `controlPlaneRef` _[ContractVersionedObjectReference](#contractversionedobjectreference)_ | controlPlaneRef is an optional reference to a provider-specific resource that holds<br />the details for provisioning the Control Plane for a Cluster. |  | Optional: \{\} <br /> |
| `infrastructureRef` _[ContractVersionedObjectReference](#contractversionedobjectreference)_ | infrastructureRef is a reference to a provider-specific resource that holds the details<br />for provisioning infrastructure for a cluster in said provider. |  | Optional: \{\} <br /> |
| `topology` _[Topology](#topology)_ | topology encapsulates the topology for the cluster.<br />NOTE: It is required to enable the ClusterTopology<br />feature gate flag to activate managed topologies support. |  | Optional: \{\} <br /> |
| `availabilityGates` _[ClusterAvailabilityGate](#clusteravailabilitygate) array_ | availabilityGates specifies additional conditions to include when evaluating Cluster Available condition.<br />If this field is not defined and the Cluster implements a managed topology, availabilityGates<br />from the corresponding ClusterClass will be used, if any. |  | MaxItems: 32 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### ClusterVariable



ClusterVariable can be used to customize the Cluster through patches. Each ClusterVariable is associated with a
Variable definition in the ClusterClass `status` variables.



_Appears in:_
- [ControlPlaneVariables](#controlplanevariables)
- [MachineDeploymentVariables](#machinedeploymentvariables)
- [MachinePoolVariables](#machinepoolvariables)
- [Topology](#topology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the variable. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `value` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#json-v1-apiextensions-k8s-io)_ | value of the variable.<br />Note: the value will be validated against the schema of the corresponding ClusterClassVariable<br />from the ClusterClass.<br />Note: We have to use apiextensionsv1.JSON instead of a custom JSON type, because controller-tools has a<br />hard-coded schema for apiextensionsv1.JSON which cannot be produced by another type via controller-tools,<br />i.e. it is not possible to have no type field.<br />Ref: https://github.com/kubernetes-sigs/controller-tools/blob/d0e03a142d0ecdd5491593e941ee1d6b5d91dba6/pkg/crd/known_types.go#L106-L111 |  | Required: \{\} <br /> |


#### Condition



Condition defines an observation of a Cluster API resource operational state.

Deprecated: This type is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.



_Appears in:_
- [Conditions](#conditions)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[ConditionType](#conditiontype)_ | type of condition in CamelCase or in foo.example.com/CamelCase.<br />Many .condition.type values are consistent across resources like Available, but because arbitrary conditions<br />can be useful (see .node.status.conditions), the ability to deconflict is important. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `severity` _[ConditionSeverity](#conditionseverity)_ | severity provides an explicit classification of Reason code, so the users or machines can immediately<br />understand the current situation and act accordingly.<br />The Severity field MUST be set only when Status=False. |  | MaxLength: 32 <br />Optional: \{\} <br /> |
| `reason` _string_ | reason is the reason for the condition's last transition in CamelCase.<br />The specific API may choose whether or not this field is considered a guaranteed API.<br />This field may be empty. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `message` _string_ | message is a human readable message indicating details about the transition.<br />This field may be empty. |  | MaxLength: 10240 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### ConditionPolarity

_Underlying type:_ _string_

ConditionPolarity defines the polarity for a metav1.Condition.

_Validation:_
- Enum: [Positive Negative]

_Appears in:_
- [ClusterAvailabilityGate](#clusteravailabilitygate)
- [MachineReadinessGate](#machinereadinessgate)

| Field | Description |
| --- | --- |
| `Positive` | PositivePolarityCondition describe a condition with positive polarity, a condition<br />where the normal state is True. e.g. NetworkReady.<br /> |
| `Negative` | NegativePolarityCondition describe a condition with negative polarity, a condition<br />where the normal state is False. e.g. MemoryPressure.<br /> |


#### ConditionSeverity

_Underlying type:_ _string_

ConditionSeverity expresses the severity of a Condition Type failing.

Deprecated: This type is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.

_Validation:_
- MaxLength: 32

_Appears in:_
- [Condition](#condition)

| Field | Description |
| --- | --- |
| `Error` | ConditionSeverityError specifies that a condition with `Status=False` is an error.<br /> |
| `Warning` | ConditionSeverityWarning specifies that a condition with `Status=False` is a warning.<br /> |
| `Info` | ConditionSeverityInfo specifies that a condition with `Status=False` is informative.<br /> |
| `` | ConditionSeverityNone should apply only to conditions with `Status=True`.<br /> |


#### ConditionType

_Underlying type:_ _string_

ConditionType is a valid value for Condition.Type.

Deprecated: This type is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.

_Validation:_
- MaxLength: 256
- MinLength: 1

_Appears in:_
- [Condition](#condition)

| Field | Description |
| --- | --- |
| `Ready` | ReadyV1Beta1Condition defines the Ready condition type that summarizes the operational state of a Cluster API object.<br /> |
| `InfrastructureReady` | InfrastructureReadyV1Beta1Condition reports a summary of current status of the infrastructure object defined for this cluster/machine/machinepool.<br />This condition is mirrored from the Ready condition in the infrastructure ref object, and<br />the absence of this condition might signal problems in the reconcile external loops or the fact that<br />the infrastructure provider does not implement the Ready condition yet.<br /> |
| `VariablesReconciled` | ClusterClassVariablesReconciledV1Beta1Condition reports if the ClusterClass variables, including both inline and external<br />variables, have been successfully reconciled.<br />This signals that the ClusterClass is ready to be used to default and validate variables on Clusters using<br />this ClusterClass.<br /> |
| `ControlPlaneInitialized` | ControlPlaneInitializedV1Beta1Condition reports if the cluster's control plane has been initialized such that the<br />cluster's apiserver is reachable. If no Control Plane provider is in use this condition reports that at least one<br />control plane Machine has a node reference. Once this Condition is marked true, its value is never changed. See<br />the ControlPlaneReady condition for an indication of the current readiness of the cluster's control plane.<br /> |
| `ControlPlaneReady` | ControlPlaneReadyV1Beta1Condition reports the ready condition from the control plane object defined for this cluster.<br />This condition is mirrored from the Ready condition in the control plane ref object, and<br />the absence of this condition might signal problems in the reconcile external loops or the fact that<br />the control plane provider does not implement the Ready condition yet.<br /> |
| `BootstrapReady` | BootstrapReadyV1Beta1Condition reports a summary of current status of the bootstrap object defined for this machine.<br />This condition is mirrored from the Ready condition in the bootstrap ref object, and<br />the absence of this condition might signal problems in the reconcile external loops or the fact that<br />the bootstrap provider does not implement the Ready condition yet.<br /> |
| `DrainingSucceeded` | DrainingSucceededV1Beta1Condition provide evidence of the status of the node drain operation which happens during the machine<br />deletion process.<br /> |
| `PreDrainDeleteHookSucceeded` | PreDrainDeleteHookSucceededV1Beta1Condition reports a machine waiting for a PreDrainDeleteHook before being delete.<br /> |
| `PreTerminateDeleteHookSucceeded` | PreTerminateDeleteHookSucceededV1Beta1Condition reports a machine waiting for a PreDrainDeleteHook before being delete.<br /> |
| `VolumeDetachSucceeded` | VolumeDetachSucceededV1Beta1Condition reports a machine waiting for volumes to be detached.<br /> |
| `HealthCheckSucceeded` | MachineHealthCheckSucceededV1Beta1Condition is set on machines that have passed a healthcheck by the MachineHealthCheck controller.<br />In the event that the health check fails it will be set to False.<br /> |
| `OwnerRemediated` | MachineOwnerRemediatedV1Beta1Condition is set on machines that have failed a healthcheck by the MachineHealthCheck controller.<br />MachineOwnerRemediatedV1Beta1Condition is set to False after a health check fails, but should be changed to True by the owning controller after remediation succeeds.<br /> |
| `ExternalRemediationTemplateAvailable` | ExternalRemediationTemplateAvailableV1Beta1Condition is set on machinehealthchecks when MachineHealthCheck controller uses external remediation.<br />ExternalRemediationTemplateAvailableV1Beta1Condition is set to false if external remediation template is not found.<br /> |
| `ExternalRemediationRequestAvailable` | ExternalRemediationRequestAvailableV1Beta1Condition is set on machinehealthchecks when MachineHealthCheck controller uses external remediation.<br />ExternalRemediationRequestAvailableV1Beta1Condition is set to false if creating external remediation request fails.<br /> |
| `NodeHealthy` | MachineNodeHealthyV1Beta1Condition provides info about the operational state of the Kubernetes node hosted on the machine by summarizing  node conditions.<br />If the conditions defined in a Kubernetes node (i.e., NodeReady, NodeMemoryPressure, NodeDiskPressure and NodePIDPressure) are in a healthy state, it will be set to True.<br /> |
| `RemediationAllowed` | RemediationAllowedV1Beta1Condition is set on MachineHealthChecks to show the status of whether the MachineHealthCheck is<br />allowed to remediate any Machines or whether it is blocked from remediating any further.<br /> |
| `Available` | MachineDeploymentAvailableV1Beta1Condition means the MachineDeployment is available, that is, at least the minimum available<br />machines required (i.e. Spec.Replicas-MaxUnavailable when spec.rollout.strategy.type = RollingUpdate) are up and running for at least minReadySeconds.<br /> |
| `MachineSetReady` | MachineSetReadyV1Beta1Condition reports a summary of current status of the MachineSet owned by the MachineDeployment.<br /> |
| `MachinesCreated` | MachinesCreatedV1Beta1Condition documents that the machines controlled by the MachineSet are created.<br />When this condition is false, it indicates that there was an error when cloning the infrastructure/bootstrap template or<br />when generating the machine object.<br /> |
| `MachinesReady` | MachinesReadyV1Beta1Condition reports an aggregate of current status of the machines controlled by the MachineSet.<br /> |
| `Resized` | ResizedV1Beta1Condition documents a MachineSet is resizing the set of controlled machines.<br /> |
| `TopologyReconciled` | TopologyReconciledV1Beta1Condition provides evidence about the reconciliation of a Cluster topology into<br />the managed objects of the Cluster.<br />Status false means that for any reason, the values defined in Cluster.spec.topology are not yet applied to<br />managed objects on the Cluster; status true means that Cluster.spec.topology have been applied to<br />the objects in the Cluster (but this does not imply those objects are already reconciled to the spec provided).<br /> |
| `RefVersionsUpToDate` | ClusterClassRefVersionsUpToDateV1Beta1Condition documents if the references in the ClusterClass are<br />up-to-date (i.e. they are using the latest apiVersion of the current Cluster API contract from<br />the corresponding CRD).<br /> |
| `ReplicasReady` | ReplicasReadyV1Beta1Condition reports an aggregate of current status of the replicas controlled by the MachinePool.<br /> |




#### ContractVersionedObjectReference



ContractVersionedObjectReference is a reference to a resource for which the version is inferred from contract labels.



_Appears in:_
- [Bootstrap](#bootstrap)
- [ClusterSpec](#clusterspec)
- [KubeadmControlPlaneMachineTemplateSpec](#kubeadmcontrolplanemachinetemplatespec)
- [MachineSpec](#machinespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _string_ | kind of the resource being referenced.<br />kind must consist of alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$` <br />Required: \{\} <br /> |
| `name` _string_ | name of the resource being referenced.<br />name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` <br />Required: \{\} <br /> |
| `apiGroup` _string_ | apiGroup is the group of the resource being referenced.<br />apiGroup must be fully qualified domain name.<br />The corresponding version for this reference will be looked up from the contract<br />labels of the corresponding CRD of the resource being referenced. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` <br />Required: \{\} <br /> |


#### ControlPlaneClass



ControlPlaneClass defines the class for the control plane.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `templateRef` _[ClusterClassTemplateReference](#clusterclasstemplatereference)_ | templateRef contains the reference to a provider-specific control plane template. |  | Required: \{\} <br /> |
| `machineInfrastructure` _[ControlPlaneClassMachineInfrastructureTemplate](#controlplaneclassmachineinfrastructuretemplate)_ | machineInfrastructure defines the metadata and infrastructure information<br />for control plane machines.<br />This field is supported if and only if the control plane provider template<br />referenced above is Machine based and supports setting replicas. |  | Optional: \{\} <br /> |
| `healthCheck` _[ControlPlaneClassHealthCheck](#controlplaneclasshealthcheck)_ | healthCheck defines a MachineHealthCheck for this ControlPlaneClass.<br />This field is supported if and only if the ControlPlane provider template<br />referenced above is Machine based and supports setting replicas. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `naming` _[ControlPlaneClassNamingSpec](#controlplaneclassnamingspec)_ | naming allows changing the naming pattern used when creating the control plane provider object. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `deletion` _[ControlPlaneClassMachineDeletionSpec](#controlplaneclassmachinedeletionspec)_ | deletion contains configuration options for Machine deletion. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition.<br />This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready<br />computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.<br />NOTE: If a Cluster defines a custom list of readinessGates for the control plane,<br />such list overrides readinessGates defined in this field.<br />NOTE: Specific control plane provider implementations might automatically extend the list of readinessGates;<br />e.g. the kubeadm control provider adds ReadinessGates for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc. |  | MaxItems: 32 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### ControlPlaneClassHealthCheck



ControlPlaneClassHealthCheck defines a MachineHealthCheck for control plane machines.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneClass](#controlplaneclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `checks` _[ControlPlaneClassHealthCheckChecks](#controlplaneclasshealthcheckchecks)_ | checks are the checks that are used to evaluate if a Machine is healthy.<br />Independent of this configuration the MachineHealthCheck controller will always<br />flag Machines with `cluster.x-k8s.io/remediate-machine` annotation and<br />Machines with deleted Nodes as unhealthy.<br />Furthermore, if checks.nodeStartupTimeoutSeconds is not set it<br />is defaulted to 10 minutes and evaluated accordingly. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `remediation` _[ControlPlaneClassHealthCheckRemediation](#controlplaneclasshealthcheckremediation)_ | remediation configures if and how remediations are triggered if a Machine is unhealthy.<br />If remediation or remediation.triggerIf is not set,<br />remediation will always be triggered for unhealthy Machines.<br />If remediation or remediation.templateRef is not set,<br />the OwnerRemediated condition will be set on unhealthy Machines to trigger remediation via<br />the owner of the Machines, for example a MachineSet or a KubeadmControlPlane. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### ControlPlaneClassHealthCheckChecks



ControlPlaneClassHealthCheckChecks are the checks that are used to evaluate if a control plane Machine is healthy.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneClassHealthCheck](#controlplaneclasshealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeStartupTimeoutSeconds` _integer_ | nodeStartupTimeoutSeconds allows to set the maximum time for MachineHealthCheck<br />to consider a Machine unhealthy if a corresponding Node isn't associated<br />through a `Spec.ProviderID` field.<br />The duration set in this field is compared to the greatest of:<br />- Cluster's infrastructure ready condition timestamp (if and when available)<br />- Control Plane's initialized condition timestamp (if and when available)<br />- Machine's infrastructure ready condition timestamp (if and when available)<br />- Machine's metadata creation timestamp<br />Defaults to 10 minutes.<br />If you wish to disable this feature, set the value explicitly to 0. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `unhealthyNodeConditions` _[UnhealthyNodeCondition](#unhealthynodecondition) array_ | unhealthyNodeConditions contains a list of conditions that determine<br />whether a node is considered unhealthy. The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the node is unhealthy. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `unhealthyMachineConditions` _[UnhealthyMachineCondition](#unhealthymachinecondition) array_ | unhealthyMachineConditions contains a list of the machine conditions that determine<br />whether a machine is considered unhealthy.  The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the machine is unhealthy. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### ControlPlaneClassHealthCheckRemediation



ControlPlaneClassHealthCheckRemediation configures if and how remediations are triggered if a control plane Machine is unhealthy.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneClassHealthCheck](#controlplaneclasshealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `triggerIf` _[ControlPlaneClassHealthCheckRemediationTriggerIf](#controlplaneclasshealthcheckremediationtriggerif)_ | triggerIf configures if remediations are triggered.<br />If this field is not set, remediations are always triggered. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `templateRef` _[MachineHealthCheckRemediationTemplateReference](#machinehealthcheckremediationtemplatereference)_ | templateRef is a reference to a remediation template<br />provided by an infrastructure provider.<br />This field is completely optional, when filled, the MachineHealthCheck controller<br />creates a new object from the template referenced and hands off remediation of the machine to<br />a controller that lives outside of Cluster API. |  | Optional: \{\} <br /> |


#### ControlPlaneClassHealthCheckRemediationTriggerIf



ControlPlaneClassHealthCheckRemediationTriggerIf configures if remediations are triggered.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneClassHealthCheckRemediation](#controlplaneclasshealthcheckremediation)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `unhealthyLessThanOrEqualTo` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | unhealthyLessThanOrEqualTo specifies that remediations are only triggered if the number of<br />unhealthy Machines is less than or equal to the configured value.<br />unhealthyInRange takes precedence if set. |  | Optional: \{\} <br /> |
| `unhealthyInRange` _string_ | unhealthyInRange specifies that remediations are only triggered if the number of<br />unhealthy Machines is in the configured range.<br />Takes precedence over unhealthyLessThanOrEqualTo.<br />Eg. "[3-5]" - This means that remediation will be allowed only when:<br />(a) there are at least 3 unhealthy Machines (and)<br />(b) there are at most 5 unhealthy Machines |  | MaxLength: 32 <br />MinLength: 1 <br />Pattern: `^\[[0-9]+-[0-9]+\]$` <br />Optional: \{\} <br /> |


#### ControlPlaneClassMachineDeletionSpec



ControlPlaneClassMachineDeletionSpec contains configuration options for Machine deletion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneClass](#controlplaneclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeDrainTimeoutSeconds` _integer_ | nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout`<br />NOTE: This value can be overridden while defining a Cluster.Topology. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeVolumeDetachTimeoutSeconds` _integer_ | nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.<br />NOTE: This value can be overridden while defining a Cluster.Topology. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeDeletionTimeoutSeconds` _integer_ | nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds.<br />NOTE: This value can be overridden while defining a Cluster.Topology. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### ControlPlaneClassMachineInfrastructureTemplate



ControlPlaneClassMachineInfrastructureTemplate defines the template for a MachineInfrastructure of a ControlPlane.



_Appears in:_
- [ControlPlaneClass](#controlplaneclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `templateRef` _[ClusterClassTemplateReference](#clusterclasstemplatereference)_ | templateRef is a required reference to the template for a MachineInfrastructure of a ControlPlane. |  | Required: \{\} <br /> |


#### ControlPlaneClassNamingSpec



ControlPlaneClassNamingSpec defines the naming strategy for control plane objects.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneClass](#controlplaneclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the name of the ControlPlane object.<br />If not defined, it will fallback to `\{\{ .cluster.name \}\}-\{\{ .random \}\}`.<br />If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will<br />get concatenated with a random suffix of length 5.<br />The templating mechanism provides the following arguments:<br />* `.cluster.name`: The name of the cluster object.<br />* `.random`: A random alphanumeric string, without vowels, of length 5. |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### ControlPlaneTopology



ControlPlaneTopology specifies the parameters for the control plane nodes in the cluster.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Topology](#topology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of control plane nodes.<br />If the value is not set, the ControlPlane object is created without the number of Replicas<br />and it's assumed that the control plane controller does not implement support for this field.<br />When specified against a control plane provider that lacks support for this field, this value will be ignored. |  | Optional: \{\} <br /> |
| `healthCheck` _[ControlPlaneTopologyHealthCheck](#controlplanetopologyhealthcheck)_ | healthCheck allows to enable, disable and override control plane health check<br />configuration from the ClusterClass for this control plane. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `deletion` _[ControlPlaneTopologyMachineDeletionSpec](#controlplanetopologymachinedeletionspec)_ | deletion contains configuration options for Machine deletion. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition.<br />This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready<br />computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.<br />If this field is not defined, readinessGates from the corresponding ControlPlaneClass will be used, if any.<br />NOTE: Specific control plane provider implementations might automatically extend the list of readinessGates;<br />e.g. the kubeadm control provider adds ReadinessGates for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc. |  | MaxItems: 32 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `variables` _[ControlPlaneVariables](#controlplanevariables)_ | variables can be used to customize the ControlPlane through patches. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### ControlPlaneTopologyHealthCheck



ControlPlaneTopologyHealthCheck defines a MachineHealthCheck for control plane machines.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneTopology](#controlplanetopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | enabled controls if a MachineHealthCheck should be created for the target machines.<br />If false: No MachineHealthCheck will be created.<br />If not set(default): A MachineHealthCheck will be created if it is defined here or<br /> in the associated ClusterClass. If no MachineHealthCheck is defined then none will be created.<br />If true: A MachineHealthCheck is guaranteed to be created. Cluster validation will<br />block if `enable` is true and no MachineHealthCheck definition is available. |  | Optional: \{\} <br /> |
| `checks` _[ControlPlaneTopologyHealthCheckChecks](#controlplanetopologyhealthcheckchecks)_ | checks are the checks that are used to evaluate if a Machine is healthy.<br />If one of checks and remediation fields are set, the system assumes that an healthCheck override is defined,<br />and as a consequence the checks and remediation fields from Cluster will be used instead of the<br />corresponding fields in ClusterClass.<br />Independent of this configuration the MachineHealthCheck controller will always<br />flag Machines with `cluster.x-k8s.io/remediate-machine` annotation and<br />Machines with deleted Nodes as unhealthy.<br />Furthermore, if checks.nodeStartupTimeoutSeconds is not set it<br />is defaulted to 10 minutes and evaluated accordingly. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `remediation` _[ControlPlaneTopologyHealthCheckRemediation](#controlplanetopologyhealthcheckremediation)_ | remediation configures if and how remediations are triggered if a Machine is unhealthy.<br />If one of checks and remediation fields are set, the system assumes that an healthCheck override is defined,<br />and as a consequence the checks and remediation fields from cluster will be used instead of the<br />corresponding fields in ClusterClass.<br />If an health check override is defined and remediation or remediation.triggerIf is not set,<br />remediation will always be triggered for unhealthy Machines.<br />If an health check override is defined and remediation or remediation.templateRef is not set,<br />the OwnerRemediated condition will be set on unhealthy Machines to trigger remediation via<br />the owner of the Machines, for example a MachineSet or a KubeadmControlPlane. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### ControlPlaneTopologyHealthCheckChecks



ControlPlaneTopologyHealthCheckChecks are the checks that are used to evaluate if a control plane Machine is healthy.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneTopologyHealthCheck](#controlplanetopologyhealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeStartupTimeoutSeconds` _integer_ | nodeStartupTimeoutSeconds allows to set the maximum time for MachineHealthCheck<br />to consider a Machine unhealthy if a corresponding Node isn't associated<br />through a `Spec.ProviderID` field.<br />The duration set in this field is compared to the greatest of:<br />- Cluster's infrastructure ready condition timestamp (if and when available)<br />- Control Plane's initialized condition timestamp (if and when available)<br />- Machine's infrastructure ready condition timestamp (if and when available)<br />- Machine's metadata creation timestamp<br />Defaults to 10 minutes.<br />If you wish to disable this feature, set the value explicitly to 0. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `unhealthyNodeConditions` _[UnhealthyNodeCondition](#unhealthynodecondition) array_ | unhealthyNodeConditions contains a list of conditions that determine<br />whether a node is considered unhealthy. The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the node is unhealthy. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `unhealthyMachineConditions` _[UnhealthyMachineCondition](#unhealthymachinecondition) array_ | unhealthyMachineConditions contains a list of the machine conditions that determine<br />whether a machine is considered unhealthy.  The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the machine is unhealthy. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### ControlPlaneTopologyHealthCheckRemediation



ControlPlaneTopologyHealthCheckRemediation configures if and how remediations are triggered if a control plane Machine is unhealthy.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneTopologyHealthCheck](#controlplanetopologyhealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `triggerIf` _[ControlPlaneTopologyHealthCheckRemediationTriggerIf](#controlplanetopologyhealthcheckremediationtriggerif)_ | triggerIf configures if remediations are triggered.<br />If this field is not set, remediations are always triggered. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `templateRef` _[MachineHealthCheckRemediationTemplateReference](#machinehealthcheckremediationtemplatereference)_ | templateRef is a reference to a remediation template<br />provided by an infrastructure provider.<br />This field is completely optional, when filled, the MachineHealthCheck controller<br />creates a new object from the template referenced and hands off remediation of the machine to<br />a controller that lives outside of Cluster API. |  | Optional: \{\} <br /> |


#### ControlPlaneTopologyHealthCheckRemediationTriggerIf



ControlPlaneTopologyHealthCheckRemediationTriggerIf configures if remediations are triggered.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneTopologyHealthCheckRemediation](#controlplanetopologyhealthcheckremediation)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `unhealthyLessThanOrEqualTo` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | unhealthyLessThanOrEqualTo specifies that remediations are only triggered if the number of<br />unhealthy Machines is less than or equal to the configured value.<br />unhealthyInRange takes precedence if set. |  | Optional: \{\} <br /> |
| `unhealthyInRange` _string_ | unhealthyInRange specifies that remediations are only triggered if the number of<br />unhealthy Machines is in the configured range.<br />Takes precedence over unhealthyLessThanOrEqualTo.<br />Eg. "[3-5]" - This means that remediation will be allowed only when:<br />(a) there are at least 3 unhealthy Machines (and)<br />(b) there are at most 5 unhealthy Machines |  | MaxLength: 32 <br />MinLength: 1 <br />Pattern: `^\[[0-9]+-[0-9]+\]$` <br />Optional: \{\} <br /> |


#### ControlPlaneTopologyMachineDeletionSpec



ControlPlaneTopologyMachineDeletionSpec contains configuration options for Machine deletion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneTopology](#controlplanetopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeDrainTimeoutSeconds` _integer_ | nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout` |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeVolumeDetachTimeoutSeconds` _integer_ | nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeDeletionTimeoutSeconds` _integer_ | nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### ControlPlaneVariables



ControlPlaneVariables can be used to provide variables for the ControlPlane.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneTopology](#controlplanetopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `overrides` _[ClusterVariable](#clustervariable) array_ | overrides can be used to override Cluster level variables. |  | MaxItems: 1000 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### ExternalPatchDefinition



ExternalPatchDefinition defines an external patch.
Note: At least one of GeneratePatchesExtension or ValidateTopologyExtension must be set.



_Appears in:_
- [ClusterClassPatch](#clusterclasspatch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `generatePatchesExtension` _string_ | generatePatchesExtension references an extension which is called to generate patches. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `validateTopologyExtension` _string_ | validateTopologyExtension references an extension which is called to validate the topology. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `discoverVariablesExtension` _string_ | discoverVariablesExtension references an extension which is called to discover variables. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `settings` _object (keys:string, values:string)_ | settings defines key value pairs to be passed to the extensions.<br />Values defined here take precedence over the values defined in the<br />corresponding ExtensionConfig. |  | Optional: \{\} <br /> |




#### FieldValueErrorReason

_Underlying type:_ _string_

FieldValueErrorReason is a machine-readable value providing more detail about why a field failed the validation.



_Appears in:_
- [ValidationRule](#validationrule)

| Field | Description |
| --- | --- |
| `FieldValueRequired` | FieldValueRequired is used to report required values that are not<br />provided (e.g. empty strings, null values, or empty arrays).<br /> |
| `FieldValueDuplicate` | FieldValueDuplicate is used to report collisions of values that must be<br />unique (e.g. unique IDs).<br /> |
| `FieldValueInvalid` | FieldValueInvalid is used to report malformed values (e.g. failed regex<br />match, too long, out of bounds).<br /> |
| `FieldValueForbidden` | FieldValueForbidden is used to report valid (as per formatting rules)<br />values which would be accepted under some conditions, but which are not<br />permitted by the current conditions (such as security policy).<br /> |


#### InfrastructureClass



InfrastructureClass defines the class for the infrastructure cluster.



_Appears in:_
- [ClusterClassSpec](#clusterclassspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `templateRef` _[ClusterClassTemplateReference](#clusterclasstemplatereference)_ | templateRef contains the reference to a provider-specific infrastructure cluster template. |  | Required: \{\} <br /> |
| `naming` _[InfrastructureClassNamingSpec](#infrastructureclassnamingspec)_ | naming allows changing the naming pattern used when creating the infrastructure cluster object. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### InfrastructureClassNamingSpec



InfrastructureClassNamingSpec defines the naming strategy for infrastructure objects.

_Validation:_
- MinProperties: 1

_Appears in:_
- [InfrastructureClass](#infrastructureclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the name of the Infrastructure object.<br />If not defined, it will fallback to `\{\{ .cluster.name \}\}-\{\{ .random \}\}`.<br />If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will<br />get concatenated with a random suffix of length 5.<br />The templating mechanism provides the following arguments:<br />* `.cluster.name`: The name of the cluster object.<br />* `.random`: A random alphanumeric string, without vowels, of length 5. |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### JSONPatch



JSONPatch defines a JSON patch.



_Appears in:_
- [PatchDefinition](#patchdefinition)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `op` _string_ | op defines the operation of the patch.<br />Note: Only `add`, `replace` and `remove` are supported. |  | Enum: [add replace remove] <br />Required: \{\} <br /> |
| `path` _string_ | path defines the path of the patch.<br />Note: Only the spec of a template can be patched, thus the path has to start with /spec/.<br />Note: For now the only allowed array modifications are `append` and `prepend`, i.e.:<br />* for op: `add`: only index 0 (prepend) and - (append) are allowed<br />* for op: `replace` or `remove`: no indexes are allowed |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `value` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#json-v1-apiextensions-k8s-io)_ | value defines the value of the patch.<br />Note: Either Value or ValueFrom is required for add and replace<br />operations. Only one of them is allowed to be set at the same time.<br />Note: We have to use apiextensionsv1.JSON instead of our JSON type,<br />because controller-tools has a hard-coded schema for apiextensionsv1.JSON<br />which cannot be produced by another type (unset type field).<br />Ref: https://github.com/kubernetes-sigs/controller-tools/blob/d0e03a142d0ecdd5491593e941ee1d6b5d91dba6/pkg/crd/known_types.go#L106-L111 |  | Optional: \{\} <br /> |
| `valueFrom` _[JSONPatchValue](#jsonpatchvalue)_ | valueFrom defines the value of the patch.<br />Note: Either Value or ValueFrom is required for add and replace<br />operations. Only one of them is allowed to be set at the same time. |  | Optional: \{\} <br /> |


#### JSONPatchValue



JSONPatchValue defines the value of a patch.
Note: Only one of the fields is allowed to be set at the same time.



_Appears in:_
- [JSONPatch](#jsonpatch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `variable` _string_ | variable is the variable to be used as value.<br />Variable can be one of the variables defined in .spec.variables or a builtin variable. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `template` _string_ | template is the Go template to be used to calculate the value.<br />A template can reference variables defined in .spec.variables and builtin variables.<br />Note: The template must evaluate to a valid YAML or JSON value. |  | MaxLength: 10240 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### JSONSchemaProps



JSONSchemaProps is a JSON-Schema following Specification Draft 4 (http://json-schema.org/).
This struct has been initially copied from apiextensionsv1.JSONSchemaProps, but all fields
which are not supported in CAPI have been removed.

_Validation:_
- MinProperties: 1

_Appears in:_
- [JSONSchemaProps](#jsonschemaprops)
- [VariableSchema](#variableschema)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `description` _string_ | description is a human-readable description of this variable. |  | MaxLength: 4096 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `example` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#json-v1-apiextensions-k8s-io)_ | example is an example for this variable. |  | Optional: \{\} <br /> |
| `type` _string_ | type is the type of the variable.<br />Valid values are: object, array, string, integer, number or boolean. |  | Enum: [object array string integer number boolean] <br />Optional: \{\} <br /> |
| `properties` _object (keys:string, values:[JSONSchemaProps](#jsonschemaprops))_ | properties specifies fields of an object.<br />NOTE: Can only be set if type is object.<br />NOTE: Properties is mutually exclusive with AdditionalProperties.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | Schemaless: \{\} <br />Optional: \{\} <br /> |
| `additionalProperties` _[JSONSchemaProps](#jsonschemaprops)_ | additionalProperties specifies the schema of values in a map (keys are always strings).<br />NOTE: Can only be set if type is object.<br />NOTE: AdditionalProperties is mutually exclusive with Properties.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | MinProperties: 1 <br />Schemaless: \{\} <br />Optional: \{\} <br /> |
| `maxProperties` _integer_ | maxProperties is the maximum amount of entries in a map or properties in an object.<br />NOTE: Can only be set if type is object. |  | Optional: \{\} <br /> |
| `minProperties` _integer_ | minProperties is the minimum amount of entries in a map or properties in an object.<br />NOTE: Can only be set if type is object. |  | Optional: \{\} <br /> |
| `required` _string array_ | required specifies which fields of an object are required.<br />NOTE: Can only be set if type is object. |  | MaxItems: 1000 <br />MinItems: 1 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `items` _[JSONSchemaProps](#jsonschemaprops)_ | items specifies fields of an array.<br />NOTE: Can only be set if type is array.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | MinProperties: 1 <br />Schemaless: \{\} <br />Optional: \{\} <br /> |
| `maxItems` _integer_ | maxItems is the max length of an array variable.<br />NOTE: Can only be set if type is array. |  | Optional: \{\} <br /> |
| `minItems` _integer_ | minItems is the min length of an array variable.<br />NOTE: Can only be set if type is array. |  | Optional: \{\} <br /> |
| `uniqueItems` _boolean_ | uniqueItems specifies if items in an array must be unique.<br />NOTE: Can only be set if type is array. |  | Optional: \{\} <br /> |
| `format` _string_ | format is an OpenAPI v3 format string. Unknown formats are ignored.<br />For a list of supported formats please see: (of the k8s.io/apiextensions-apiserver version we're currently using)<br />https://github.com/kubernetes/apiextensions-apiserver/blob/master/pkg/apiserver/validation/formats.go<br />NOTE: Can only be set if type is string. |  | MaxLength: 32 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `maxLength` _integer_ | maxLength is the max length of a string variable.<br />NOTE: Can only be set if type is string. |  | Optional: \{\} <br /> |
| `minLength` _integer_ | minLength is the min length of a string variable.<br />NOTE: Can only be set if type is string. |  | Optional: \{\} <br /> |
| `pattern` _string_ | pattern is the regex which a string variable must match.<br />NOTE: Can only be set if type is string. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `maximum` _integer_ | maximum is the maximum of an integer or number variable.<br />If ExclusiveMaximum is false, the variable is valid if it is lower than, or equal to, the value of Maximum.<br />If ExclusiveMaximum is true, the variable is valid if it is strictly lower than the value of Maximum.<br />NOTE: Can only be set if type is integer or number. |  | Optional: \{\} <br /> |
| `exclusiveMaximum` _boolean_ | exclusiveMaximum specifies if the Maximum is exclusive.<br />NOTE: Can only be set if type is integer or number. |  | Optional: \{\} <br /> |
| `minimum` _integer_ | minimum is the minimum of an integer or number variable.<br />If ExclusiveMinimum is false, the variable is valid if it is greater than, or equal to, the value of Minimum.<br />If ExclusiveMinimum is true, the variable is valid if it is strictly greater than the value of Minimum.<br />NOTE: Can only be set if type is integer or number. |  | Optional: \{\} <br /> |
| `exclusiveMinimum` _boolean_ | exclusiveMinimum specifies if the Minimum is exclusive.<br />NOTE: Can only be set if type is integer or number. |  | Optional: \{\} <br /> |
| `x-kubernetes-preserve-unknown-fields` _boolean_ | x-kubernetes-preserve-unknown-fields allows setting fields in a variable object<br />which are not defined in the variable schema. This affects fields recursively,<br />except if nested properties or additionalProperties are specified in the schema. |  | Optional: \{\} <br /> |
| `enum` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#json-v1-apiextensions-k8s-io) array_ | enum is the list of valid values of the variable.<br />NOTE: Can be set for all types. |  | MaxItems: 100 <br />Optional: \{\} <br /> |
| `default` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#json-v1-apiextensions-k8s-io)_ | default is the default value of the variable.<br />NOTE: Can be set for all types. |  | Optional: \{\} <br /> |
| `x-kubernetes-validations` _[ValidationRule](#validationrule) array_ | x-kubernetes-validations describes a list of validation rules written in the CEL expression language. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `x-metadata` _[VariableSchemaMetadata](#variableschemametadata)_ | x-metadata is the metadata of a variable or a nested field within a variable.<br />It can be used to add additional data for higher level tools. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `x-kubernetes-int-or-string` _boolean_ | x-kubernetes-int-or-string specifies that this value is<br />either an integer or a string. If this is true, an empty<br />type is allowed and type as child of anyOf is permitted<br />if following one of the following patterns:<br />1) anyOf:<br />   - type: integer<br />   - type: string<br />2) allOf:<br />   - anyOf:<br />     - type: integer<br />     - type: string<br />   - ... zero or more |  | Optional: \{\} <br /> |
| `allOf` _[JSONSchemaProps](#jsonschemaprops) array_ | allOf specifies that the variable must validate against all of the subschemas in the array.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | MinProperties: 1 <br />Schemaless: \{\} <br />Optional: \{\} <br /> |
| `oneOf` _[JSONSchemaProps](#jsonschemaprops) array_ | oneOf specifies that the variable must validate against exactly one of the subschemas in the array.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | MinProperties: 1 <br />Schemaless: \{\} <br />Optional: \{\} <br /> |
| `anyOf` _[JSONSchemaProps](#jsonschemaprops) array_ | anyOf specifies that the variable must validate against one or more of the subschemas in the array.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | MinProperties: 1 <br />Schemaless: \{\} <br />Optional: \{\} <br /> |
| `not` _[JSONSchemaProps](#jsonschemaprops)_ | not specifies that the variable must not validate against the subschema.<br />NOTE: This field uses PreserveUnknownFields and Schemaless,<br />because recursive validation is not possible. |  | MinProperties: 1 <br />Schemaless: \{\} <br />Optional: \{\} <br /> |


#### Machine



Machine is the Schema for the machines API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `Machine` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[MachineSpec](#machinespec)_ | spec is the desired state of Machine. |  | Required: \{\} <br /> |


#### MachineAddress



MachineAddress contains information for the node's address.



_Appears in:_
- [MachineAddresses](#machineaddresses)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[MachineAddressType](#machineaddresstype)_ | type is the machine address type, one of Hostname, ExternalIP, InternalIP, ExternalDNS or InternalDNS. |  | Enum: [Hostname ExternalIP InternalIP ExternalDNS InternalDNS] <br />Required: \{\} <br /> |
| `address` _string_ | address is the machine address. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |


#### MachineAddressType

_Underlying type:_ _string_

MachineAddressType describes a valid MachineAddress type.

_Validation:_
- Enum: [Hostname ExternalIP InternalIP ExternalDNS InternalDNS]

_Appears in:_
- [MachineAddress](#machineaddress)

| Field | Description |
| --- | --- |
| `Hostname` |  |
| `ExternalIP` |  |
| `InternalIP` |  |
| `ExternalDNS` |  |
| `InternalDNS` |  |




#### MachineDeletionSpec



MachineDeletionSpec contains configuration options for Machine deletion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineSpec](#machinespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeDrainTimeoutSeconds` _integer_ | nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout` |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeVolumeDetachTimeoutSeconds` _integer_ | nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeDeletionTimeoutSeconds` _integer_ | nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### MachineDeployment



MachineDeployment is the Schema for the machinedeployments API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `MachineDeployment` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[MachineDeploymentSpec](#machinedeploymentspec)_ | spec is the desired state of MachineDeployment. |  | Required: \{\} <br /> |


#### MachineDeploymentClass



MachineDeploymentClass serves as a template to define a set of worker nodes of the cluster
provisioned using the `ClusterClass`.



_Appears in:_
- [WorkersClass](#workersclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `class` _string_ | class denotes a type of worker node present in the cluster,<br />this name MUST be unique within a ClusterClass and can be referenced<br />in the Cluster to create a managed MachineDeployment. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `bootstrap` _[MachineDeploymentClassBootstrapTemplate](#machinedeploymentclassbootstraptemplate)_ | bootstrap contains the bootstrap template reference to be used<br />for the creation of worker Machines. |  | Required: \{\} <br /> |
| `infrastructure` _[MachineDeploymentClassInfrastructureTemplate](#machinedeploymentclassinfrastructuretemplate)_ | infrastructure contains the infrastructure template reference to be used<br />for the creation of worker Machines. |  | Required: \{\} <br /> |
| `healthCheck` _[MachineDeploymentClassHealthCheck](#machinedeploymentclasshealthcheck)_ | healthCheck defines a MachineHealthCheck for this MachineDeploymentClass. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `failureDomain` _string_ | failureDomain is the failure domain the machines will be created in.<br />Must match the name of a FailureDomain from the Cluster status.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `naming` _[MachineDeploymentClassNamingSpec](#machinedeploymentclassnamingspec)_ | naming allows changing the naming pattern used when creating the MachineDeployment. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `deletion` _[MachineDeploymentClassMachineDeletionSpec](#machinedeploymentclassmachinedeletionspec)_ | deletion contains configuration options for Machine deletion. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a newly created machine should<br />be ready.<br />Defaults to 0 (machine will be considered available as soon as it<br />is ready)<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition.<br />This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready<br />computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.<br />NOTE: If a Cluster defines a custom list of readinessGates for a MachineDeployment using this MachineDeploymentClass,<br />such list overrides readinessGates defined in this field. |  | MaxItems: 32 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `rollout` _[MachineDeploymentClassRolloutSpec](#machinedeploymentclassrolloutspec)_ | rollout allows you to configure the behaviour of rolling updates to the MachineDeployment Machines.<br />It allows you to define the strategy used during rolling replacements. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentClassBootstrapTemplate



MachineDeploymentClassBootstrapTemplate defines the BootstrapTemplate for a MachineDeployment.



_Appears in:_
- [MachineDeploymentClass](#machinedeploymentclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `templateRef` _[ClusterClassTemplateReference](#clusterclasstemplatereference)_ | templateRef is a required reference to the BootstrapTemplate for a MachineDeployment. |  | Required: \{\} <br /> |


#### MachineDeploymentClassHealthCheck



MachineDeploymentClassHealthCheck defines a MachineHealthCheck for MachineDeployment machines.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentClass](#machinedeploymentclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `checks` _[MachineDeploymentClassHealthCheckChecks](#machinedeploymentclasshealthcheckchecks)_ | checks are the checks that are used to evaluate if a Machine is healthy.<br />Independent of this configuration the MachineHealthCheck controller will always<br />flag Machines with `cluster.x-k8s.io/remediate-machine` annotation and<br />Machines with deleted Nodes as unhealthy.<br />Furthermore, if checks.nodeStartupTimeoutSeconds is not set it<br />is defaulted to 10 minutes and evaluated accordingly. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `remediation` _[MachineDeploymentClassHealthCheckRemediation](#machinedeploymentclasshealthcheckremediation)_ | remediation configures if and how remediations are triggered if a Machine is unhealthy.<br />If remediation or remediation.triggerIf is not set,<br />remediation will always be triggered for unhealthy Machines.<br />If remediation or remediation.templateRef is not set,<br />the OwnerRemediated condition will be set on unhealthy Machines to trigger remediation via<br />the owner of the Machines, for example a MachineSet or a KubeadmControlPlane. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentClassHealthCheckChecks



MachineDeploymentClassHealthCheckChecks are the checks that are used to evaluate if a MachineDeployment Machine is healthy.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentClassHealthCheck](#machinedeploymentclasshealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeStartupTimeoutSeconds` _integer_ | nodeStartupTimeoutSeconds allows to set the maximum time for MachineHealthCheck<br />to consider a Machine unhealthy if a corresponding Node isn't associated<br />through a `Spec.ProviderID` field.<br />The duration set in this field is compared to the greatest of:<br />- Cluster's infrastructure ready condition timestamp (if and when available)<br />- Control Plane's initialized condition timestamp (if and when available)<br />- Machine's infrastructure ready condition timestamp (if and when available)<br />- Machine's metadata creation timestamp<br />Defaults to 10 minutes.<br />If you wish to disable this feature, set the value explicitly to 0. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `unhealthyNodeConditions` _[UnhealthyNodeCondition](#unhealthynodecondition) array_ | unhealthyNodeConditions contains a list of conditions that determine<br />whether a node is considered unhealthy. The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the node is unhealthy. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `unhealthyMachineConditions` _[UnhealthyMachineCondition](#unhealthymachinecondition) array_ | unhealthyMachineConditions contains a list of the machine conditions that determine<br />whether a machine is considered unhealthy.  The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the machine is unhealthy. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentClassHealthCheckRemediation



MachineDeploymentClassHealthCheckRemediation configures if and how remediations are triggered if a MachineDeployment Machine is unhealthy.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentClassHealthCheck](#machinedeploymentclasshealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxInFlight` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxInFlight determines how many in flight remediations should happen at the same time.<br />Remediation only happens on the MachineSet with the most current revision, while<br />older MachineSets (usually present during rollout operations) aren't allowed to remediate.<br />Note: In general (independent of remediations), unhealthy machines are always<br />prioritized during scale down operations over healthy ones.<br />MaxInFlight can be set to a fixed number or a percentage.<br />Example: when this is set to 20%, the MachineSet controller deletes at most 20% of<br />the desired replicas.<br />If not set, remediation is limited to all machines (bounded by replicas)<br />under the active MachineSet's management. |  | Optional: \{\} <br /> |
| `triggerIf` _[MachineDeploymentClassHealthCheckRemediationTriggerIf](#machinedeploymentclasshealthcheckremediationtriggerif)_ | triggerIf configures if remediations are triggered.<br />If this field is not set, remediations are always triggered. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `templateRef` _[MachineHealthCheckRemediationTemplateReference](#machinehealthcheckremediationtemplatereference)_ | templateRef is a reference to a remediation template<br />provided by an infrastructure provider.<br />This field is completely optional, when filled, the MachineHealthCheck controller<br />creates a new object from the template referenced and hands off remediation of the machine to<br />a controller that lives outside of Cluster API. |  | Optional: \{\} <br /> |


#### MachineDeploymentClassHealthCheckRemediationTriggerIf



MachineDeploymentClassHealthCheckRemediationTriggerIf configures if remediations are triggered.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentClassHealthCheckRemediation](#machinedeploymentclasshealthcheckremediation)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `unhealthyLessThanOrEqualTo` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | unhealthyLessThanOrEqualTo specifies that remediations are only triggered if the number of<br />unhealthy Machines is less than or equal to the configured value.<br />unhealthyInRange takes precedence if set. |  | Optional: \{\} <br /> |
| `unhealthyInRange` _string_ | unhealthyInRange specifies that remediations are only triggered if the number of<br />unhealthy Machines is in the configured range.<br />Takes precedence over unhealthyLessThanOrEqualTo.<br />Eg. "[3-5]" - This means that remediation will be allowed only when:<br />(a) there are at least 3 unhealthy Machines (and)<br />(b) there are at most 5 unhealthy Machines |  | MaxLength: 32 <br />MinLength: 1 <br />Pattern: `^\[[0-9]+-[0-9]+\]$` <br />Optional: \{\} <br /> |


#### MachineDeploymentClassInfrastructureTemplate



MachineDeploymentClassInfrastructureTemplate defines the InfrastructureTemplate for a MachineDeployment.



_Appears in:_
- [MachineDeploymentClass](#machinedeploymentclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `templateRef` _[ClusterClassTemplateReference](#clusterclasstemplatereference)_ | templateRef is a required reference to the InfrastructureTemplate for a MachineDeployment. |  | Required: \{\} <br /> |


#### MachineDeploymentClassMachineDeletionSpec



MachineDeploymentClassMachineDeletionSpec contains configuration options for Machine deletion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentClass](#machinedeploymentclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `order` _[MachineSetDeletionOrder](#machinesetdeletionorder)_ | order defines the order in which Machines are deleted when downscaling.<br />Defaults to "Random".  Valid values are "Random, "Newest", "Oldest" |  | Enum: [Random Newest Oldest] <br />Optional: \{\} <br /> |
| `nodeDrainTimeoutSeconds` _integer_ | nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout`<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeVolumeDetachTimeoutSeconds` _integer_ | nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeDeletionTimeoutSeconds` _integer_ | nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachineDeploymentClass. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### MachineDeploymentClassNamingSpec



MachineDeploymentClassNamingSpec defines the naming strategy for machine deployment objects.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentClass](#machinedeploymentclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the name of the MachineDeployment object.<br />If not defined, it will fallback to `\{\{ .cluster.name \}\}-\{\{ .machineDeployment.topologyName \}\}-\{\{ .random \}\}`.<br />If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will<br />get concatenated with a random suffix of length 5.<br />The templating mechanism provides the following arguments:<br />* `.cluster.name`: The name of the cluster object.<br />* `.random`: A random alphanumeric string, without vowels, of length 5.<br />* `.machineDeployment.topologyName`: The name of the MachineDeployment topology (Cluster.spec.topology.workers.machineDeployments[].name). |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentClassRolloutSpec



MachineDeploymentClassRolloutSpec defines the rollout behavior.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentClass](#machinedeploymentclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `strategy` _[MachineDeploymentClassRolloutStrategy](#machinedeploymentclassrolloutstrategy)_ | strategy specifies how to roll out control plane Machines. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentClassRolloutStrategy



MachineDeploymentClassRolloutStrategy describes how to replace existing machines
with new ones.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentClassRolloutSpec](#machinedeploymentclassrolloutspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[MachineDeploymentRolloutStrategyType](#machinedeploymentrolloutstrategytype)_ | type of rollout. Allowed values are RollingUpdate and OnDelete.<br />Default is RollingUpdate. |  | Enum: [RollingUpdate OnDelete] <br />Required: \{\} <br /> |
| `rollingUpdate` _[MachineDeploymentClassRolloutStrategyRollingUpdate](#machinedeploymentclassrolloutstrategyrollingupdate)_ | rollingUpdate is the rolling update config params. Present only if<br />type = RollingUpdate. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentClassRolloutStrategyRollingUpdate



MachineDeploymentClassRolloutStrategyRollingUpdate is used to control the desired behavior of rolling update.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentClassRolloutStrategy](#machinedeploymentclassrolloutstrategy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxUnavailable` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxUnavailable is the maximum number of machines that can be unavailable during the update.<br />Value can be an absolute number (ex: 5) or a percentage of desired<br />machines (ex: 10%).<br />Absolute number is calculated from percentage by rounding down.<br />This can not be 0 if MaxSurge is 0.<br />Defaults to 0.<br />Example: when this is set to 30%, the old MachineSet can be scaled<br />down to 70% of desired machines immediately when the rolling update<br />starts. Once new machines are ready, old MachineSet can be scaled<br />down further, followed by scaling up the new MachineSet, ensuring<br />that the total number of machines available at all times<br />during the update is at least 70% of desired machines. |  | Optional: \{\} <br /> |
| `maxSurge` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxSurge is the maximum number of machines that can be scheduled above the<br />desired number of machines.<br />Value can be an absolute number (ex: 5) or a percentage of<br />desired machines (ex: 10%).<br />This can not be 0 if MaxUnavailable is 0.<br />Absolute number is calculated from percentage by rounding up.<br />Defaults to 1.<br />Example: when this is set to 30%, the new MachineSet can be scaled<br />up immediately when the rolling update starts, such that the total<br />number of old and new machines do not exceed 130% of desired<br />machines. Once old machines have been killed, new MachineSet can<br />be scaled up further, ensuring that total number of machines running<br />at any time during the update is at most 130% of desired machines. |  | Optional: \{\} <br /> |


#### MachineDeploymentDeletionSpec



MachineDeploymentDeletionSpec contains configuration options for MachineDeployment deletion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentSpec](#machinedeploymentspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `order` _[MachineSetDeletionOrder](#machinesetdeletionorder)_ | order defines the order in which Machines are deleted when downscaling.<br />Defaults to "Random".  Valid values are "Random, "Newest", "Oldest" |  | Enum: [Random Newest Oldest] <br />Optional: \{\} <br /> |




#### MachineDeploymentRemediationSpec



MachineDeploymentRemediationSpec controls how unhealthy Machines are remediated.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentSpec](#machinedeploymentspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxInFlight` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxInFlight determines how many in flight remediations should happen at the same time.<br />Remediation only happens on the MachineSet with the most current revision, while<br />older MachineSets (usually present during rollout operations) aren't allowed to remediate.<br />Note: In general (independent of remediations), unhealthy machines are always<br />prioritized during scale down operations over healthy ones.<br />MaxInFlight can be set to a fixed number or a percentage.<br />Example: when this is set to 20%, the MachineSet controller deletes at most 20% of<br />the desired replicas.<br />If not set, remediation is limited to all machines (bounded by replicas)<br />under the active MachineSet's management. |  | Optional: \{\} <br /> |


#### MachineDeploymentRolloutSpec



MachineDeploymentRolloutSpec defines the rollout behavior.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentSpec](#machinedeploymentspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `strategy` _[MachineDeploymentRolloutStrategy](#machinedeploymentrolloutstrategy)_ | strategy specifies how to roll out control plane Machines. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentRolloutStrategy



MachineDeploymentRolloutStrategy describes how to replace existing machines
with new ones.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentRolloutSpec](#machinedeploymentrolloutspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[MachineDeploymentRolloutStrategyType](#machinedeploymentrolloutstrategytype)_ | type of rollout. Allowed values are RollingUpdate and OnDelete.<br />Default is RollingUpdate. |  | Enum: [RollingUpdate OnDelete] <br />Required: \{\} <br /> |
| `rollingUpdate` _[MachineDeploymentRolloutStrategyRollingUpdate](#machinedeploymentrolloutstrategyrollingupdate)_ | rollingUpdate is the rolling update config params. Present only if<br />type = RollingUpdate. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentRolloutStrategyRollingUpdate



MachineDeploymentRolloutStrategyRollingUpdate is used to control the desired behavior of rolling update.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentRolloutStrategy](#machinedeploymentrolloutstrategy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxUnavailable` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxUnavailable is the maximum number of machines that can be unavailable during the update.<br />Value can be an absolute number (ex: 5) or a percentage of desired<br />machines (ex: 10%).<br />Absolute number is calculated from percentage by rounding down.<br />This can not be 0 if MaxSurge is 0.<br />Defaults to 0.<br />Example: when this is set to 30%, the old MachineSet can be scaled<br />down to 70% of desired machines immediately when the rolling update<br />starts. Once new machines are ready, old MachineSet can be scaled<br />down further, followed by scaling up the new MachineSet, ensuring<br />that the total number of machines available at all times<br />during the update is at least 70% of desired machines. |  | Optional: \{\} <br /> |
| `maxSurge` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxSurge is the maximum number of machines that can be scheduled above the<br />desired number of machines.<br />Value can be an absolute number (ex: 5) or a percentage of<br />desired machines (ex: 10%).<br />This can not be 0 if MaxUnavailable is 0.<br />Absolute number is calculated from percentage by rounding up.<br />Defaults to 1.<br />Example: when this is set to 30%, the new MachineSet can be scaled<br />up immediately when the rolling update starts, such that the total<br />number of old and new machines do not exceed 130% of desired<br />machines. Once old machines have been killed, new MachineSet can<br />be scaled up further, ensuring that total number of machines running<br />at any time during the update is at most 130% of desired machines. |  | Optional: \{\} <br /> |


#### MachineDeploymentRolloutStrategyType

_Underlying type:_ _string_

MachineDeploymentRolloutStrategyType defines the type of MachineDeployment rollout strategies.

_Validation:_
- Enum: [RollingUpdate OnDelete]

_Appears in:_
- [MachineDeploymentClassRolloutStrategy](#machinedeploymentclassrolloutstrategy)
- [MachineDeploymentRolloutStrategy](#machinedeploymentrolloutstrategy)
- [MachineDeploymentTopologyRolloutStrategy](#machinedeploymenttopologyrolloutstrategy)

| Field | Description |
| --- | --- |
| `RollingUpdate` | RollingUpdateMachineDeploymentStrategyType replaces the old MachineSet by new one using rolling update<br />i.e. gradually scale down the old MachineSet and scale up the new one.<br /> |
| `OnDelete` | OnDeleteMachineDeploymentStrategyType replaces old MachineSets when the deletion of the associated machines are completed.<br /> |


#### MachineDeploymentSpec



MachineDeploymentSpec defines the desired state of MachineDeployment.



_Appears in:_
- [MachineDeployment](#machinedeployment)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of desired machines.<br />This is a pointer to distinguish between explicit zero and not specified.<br />Defaults to:<br />* if the Kubernetes autoscaler min size and max size annotations are set:<br />  - if it's a new MachineDeployment, use min size<br />  - if the replicas field of the old MachineDeployment is < min size, use min size<br />  - if the replicas field of the old MachineDeployment is > max size, use max size<br />  - if the replicas field of the old MachineDeployment is in the (min size, max size) range, keep the value from the oldMD<br />* otherwise use 1<br />Note: Defaulting will be run whenever the replicas field is not set:<br />* A new MachineDeployment is created with replicas not set.<br />* On an existing MachineDeployment the replicas field was first set and is now unset.<br />Those cases are especially relevant for the following Kubernetes autoscaler use cases:<br />* A new MachineDeployment is created and replicas should be managed by the autoscaler<br />* An existing MachineDeployment which initially wasn't controlled by the autoscaler<br />  should be later controlled by the autoscaler |  | Optional: \{\} <br /> |
| `rollout` _[MachineDeploymentRolloutSpec](#machinedeploymentrolloutspec)_ | rollout allows you to configure the behaviour of rolling updates to the MachineDeployment Machines.<br />It allows you to require that all Machines are replaced after a certain time,<br />and allows you to define the strategy used during rolling replacements. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | selector is the label selector for machines. Existing MachineSets whose machines are<br />selected by this will be the ones affected by this deployment.<br />It must match the machine template's labels. |  | Required: \{\} <br /> |
| `template` _[MachineTemplateSpec](#machinetemplatespec)_ | template describes the machines that will be created. |  | Required: \{\} <br /> |
| `machineNaming` _[MachineNamingSpec](#machinenamingspec)_ | machineNaming allows changing the naming pattern used when creating Machines.<br />Note: InfraMachines & BootstrapConfigs will use the same name as the corresponding Machines. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `remediation` _[MachineDeploymentRemediationSpec](#machinedeploymentremediationspec)_ | remediation controls how unhealthy Machines are remediated. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `deletion` _[MachineDeploymentDeletionSpec](#machinedeploymentdeletionspec)_ | deletion contains configuration options for MachineDeployment deletion. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `paused` _boolean_ | paused indicates that the deployment is paused. |  | Optional: \{\} <br /> |


#### MachineDeploymentTopology



MachineDeploymentTopology specifies the different parameters for a set of worker nodes in the topology.
This set of nodes is managed by a MachineDeployment object whose lifecycle is managed by the Cluster controller.



_Appears in:_
- [WorkersTopology](#workerstopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `class` _string_ | class is the name of the MachineDeploymentClass used to create the set of worker nodes.<br />This should match one of the deployment classes defined in the ClusterClass object<br />mentioned in the `Cluster.Spec.Class` field. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `name` _string_ | name is the unique identifier for this MachineDeploymentTopology.<br />The value is used with other unique identifiers to create a MachineDeployment's Name<br />(e.g. cluster's name, etc). In case the name is greater than the allowed maximum length,<br />the values are hashed together. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` <br />Required: \{\} <br /> |
| `failureDomain` _string_ | failureDomain is the failure domain the machines will be created in.<br />Must match a key in the FailureDomains map stored on the cluster object. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of worker nodes belonging to this set.<br />If the value is nil, the MachineDeployment is created without the number of Replicas (defaulting to 1)<br />and it's assumed that an external entity (like cluster autoscaler) is responsible for the management<br />of this value. |  | Optional: \{\} <br /> |
| `healthCheck` _[MachineDeploymentTopologyHealthCheck](#machinedeploymenttopologyhealthcheck)_ | healthCheck allows to enable, disable and override MachineDeployment health check<br />configuration from the ClusterClass for this MachineDeployment. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `deletion` _[MachineDeploymentTopologyMachineDeletionSpec](#machinedeploymenttopologymachinedeletionspec)_ | deletion contains configuration options for Machine deletion. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a newly created machine should<br />be ready.<br />Defaults to 0 (machine will be considered available as soon as it<br />is ready) |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition.<br />This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready<br />computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.<br />If this field is not defined, readinessGates from the corresponding MachineDeploymentClass will be used, if any. |  | MaxItems: 32 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `rollout` _[MachineDeploymentTopologyRolloutSpec](#machinedeploymenttopologyrolloutspec)_ | rollout allows you to configure the behaviour of rolling updates to the MachineDeployment Machines.<br />It allows you to define the strategy used during rolling replacements. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `variables` _[MachineDeploymentVariables](#machinedeploymentvariables)_ | variables can be used to customize the MachineDeployment through patches. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentTopologyHealthCheck



MachineDeploymentTopologyHealthCheck defines a MachineHealthCheck for MachineDeployment machines.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentTopology](#machinedeploymenttopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | enabled controls if a MachineHealthCheck should be created for the target machines.<br />If false: No MachineHealthCheck will be created.<br />If not set(default): A MachineHealthCheck will be created if it is defined here or<br /> in the associated ClusterClass. If no MachineHealthCheck is defined then none will be created.<br />If true: A MachineHealthCheck is guaranteed to be created. Cluster validation will<br />block if `enable` is true and no MachineHealthCheck definition is available. |  | Optional: \{\} <br /> |
| `checks` _[MachineDeploymentTopologyHealthCheckChecks](#machinedeploymenttopologyhealthcheckchecks)_ | checks are the checks that are used to evaluate if a Machine is healthy.<br />If one of checks and remediation fields are set, the system assumes that an healthCheck override is defined,<br />and as a consequence the checks and remediation fields from Cluster will be used instead of the<br />corresponding fields in ClusterClass.<br />Independent of this configuration the MachineHealthCheck controller will always<br />flag Machines with `cluster.x-k8s.io/remediate-machine` annotation and<br />Machines with deleted Nodes as unhealthy.<br />Furthermore, if checks.nodeStartupTimeoutSeconds is not set it<br />is defaulted to 10 minutes and evaluated accordingly. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `remediation` _[MachineDeploymentTopologyHealthCheckRemediation](#machinedeploymenttopologyhealthcheckremediation)_ | remediation configures if and how remediations are triggered if a Machine is unhealthy.<br />If one of checks and remediation fields are set, the system assumes that an healthCheck override is defined,<br />and as a consequence the checks and remediation fields from cluster will be used instead of the<br />corresponding fields in ClusterClass.<br />If an health check override is defined and remediation or remediation.triggerIf is not set,<br />remediation will always be triggered for unhealthy Machines.<br />If an health check override is defined and remediation or remediation.templateRef is not set,<br />the OwnerRemediated condition will be set on unhealthy Machines to trigger remediation via<br />the owner of the Machines, for example a MachineSet or a KubeadmControlPlane. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentTopologyHealthCheckChecks



MachineDeploymentTopologyHealthCheckChecks are the checks that are used to evaluate if a MachineDeployment Machine is healthy.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentTopologyHealthCheck](#machinedeploymenttopologyhealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeStartupTimeoutSeconds` _integer_ | nodeStartupTimeoutSeconds allows to set the maximum time for MachineHealthCheck<br />to consider a Machine unhealthy if a corresponding Node isn't associated<br />through a `Spec.ProviderID` field.<br />The duration set in this field is compared to the greatest of:<br />- Cluster's infrastructure ready condition timestamp (if and when available)<br />- Control Plane's initialized condition timestamp (if and when available)<br />- Machine's infrastructure ready condition timestamp (if and when available)<br />- Machine's metadata creation timestamp<br />Defaults to 10 minutes.<br />If you wish to disable this feature, set the value explicitly to 0. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `unhealthyNodeConditions` _[UnhealthyNodeCondition](#unhealthynodecondition) array_ | unhealthyNodeConditions contains a list of conditions that determine<br />whether a node is considered unhealthy. The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the node is unhealthy. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `unhealthyMachineConditions` _[UnhealthyMachineCondition](#unhealthymachinecondition) array_ | unhealthyMachineConditions contains a list of the machine conditions that determine<br />whether a machine is considered unhealthy.  The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the machine is unhealthy. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentTopologyHealthCheckRemediation



MachineDeploymentTopologyHealthCheckRemediation configures if and how remediations are triggered if a MachineDeployment Machine is unhealthy.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentTopologyHealthCheck](#machinedeploymenttopologyhealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxInFlight` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxInFlight determines how many in flight remediations should happen at the same time.<br />Remediation only happens on the MachineSet with the most current revision, while<br />older MachineSets (usually present during rollout operations) aren't allowed to remediate.<br />Note: In general (independent of remediations), unhealthy machines are always<br />prioritized during scale down operations over healthy ones.<br />MaxInFlight can be set to a fixed number or a percentage.<br />Example: when this is set to 20%, the MachineSet controller deletes at most 20% of<br />the desired replicas.<br />If not set, remediation is limited to all machines (bounded by replicas)<br />under the active MachineSet's management. |  | Optional: \{\} <br /> |
| `triggerIf` _[MachineDeploymentTopologyHealthCheckRemediationTriggerIf](#machinedeploymenttopologyhealthcheckremediationtriggerif)_ | triggerIf configures if remediations are triggered.<br />If this field is not set, remediations are always triggered. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `templateRef` _[MachineHealthCheckRemediationTemplateReference](#machinehealthcheckremediationtemplatereference)_ | templateRef is a reference to a remediation template<br />provided by an infrastructure provider.<br />This field is completely optional, when filled, the MachineHealthCheck controller<br />creates a new object from the template referenced and hands off remediation of the machine to<br />a controller that lives outside of Cluster API. |  | Optional: \{\} <br /> |


#### MachineDeploymentTopologyHealthCheckRemediationTriggerIf



MachineDeploymentTopologyHealthCheckRemediationTriggerIf configures if remediations are triggered.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentTopologyHealthCheckRemediation](#machinedeploymenttopologyhealthcheckremediation)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `unhealthyLessThanOrEqualTo` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | unhealthyLessThanOrEqualTo specifies that remediations are only triggered if the number of<br />unhealthy Machines is less than or equal to the configured value.<br />unhealthyInRange takes precedence if set. |  | Optional: \{\} <br /> |
| `unhealthyInRange` _string_ | unhealthyInRange specifies that remediations are only triggered if the number of<br />unhealthy Machines is in the configured range.<br />Takes precedence over unhealthyLessThanOrEqualTo.<br />Eg. "[3-5]" - This means that remediation will be allowed only when:<br />(a) there are at least 3 unhealthy Machines (and)<br />(b) there are at most 5 unhealthy Machines |  | MaxLength: 32 <br />MinLength: 1 <br />Pattern: `^\[[0-9]+-[0-9]+\]$` <br />Optional: \{\} <br /> |


#### MachineDeploymentTopologyMachineDeletionSpec



MachineDeploymentTopologyMachineDeletionSpec contains configuration options for Machine deletion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentTopology](#machinedeploymenttopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `order` _[MachineSetDeletionOrder](#machinesetdeletionorder)_ | order defines the order in which Machines are deleted when downscaling.<br />Defaults to "Random".  Valid values are "Random, "Newest", "Oldest" |  | Enum: [Random Newest Oldest] <br />Optional: \{\} <br /> |
| `nodeDrainTimeoutSeconds` _integer_ | nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout` |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeVolumeDetachTimeoutSeconds` _integer_ | nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeDeletionTimeoutSeconds` _integer_ | nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### MachineDeploymentTopologyRolloutSpec



MachineDeploymentTopologyRolloutSpec defines the rollout behavior.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentTopology](#machinedeploymenttopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `strategy` _[MachineDeploymentTopologyRolloutStrategy](#machinedeploymenttopologyrolloutstrategy)_ | strategy specifies how to roll out control plane Machines. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentTopologyRolloutStrategy



MachineDeploymentTopologyRolloutStrategy describes how to replace existing machines
with new ones.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentTopologyRolloutSpec](#machinedeploymenttopologyrolloutspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[MachineDeploymentRolloutStrategyType](#machinedeploymentrolloutstrategytype)_ | type of rollout. Allowed values are RollingUpdate and OnDelete.<br />Default is RollingUpdate. |  | Enum: [RollingUpdate OnDelete] <br />Required: \{\} <br /> |
| `rollingUpdate` _[MachineDeploymentTopologyRolloutStrategyRollingUpdate](#machinedeploymenttopologyrolloutstrategyrollingupdate)_ | rollingUpdate is the rolling update config params. Present only if<br />type = RollingUpdate. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineDeploymentTopologyRolloutStrategyRollingUpdate



MachineDeploymentTopologyRolloutStrategyRollingUpdate is used to control the desired behavior of rolling update.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentTopologyRolloutStrategy](#machinedeploymenttopologyrolloutstrategy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxUnavailable` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxUnavailable is the maximum number of machines that can be unavailable during the update.<br />Value can be an absolute number (ex: 5) or a percentage of desired<br />machines (ex: 10%).<br />Absolute number is calculated from percentage by rounding down.<br />This can not be 0 if MaxSurge is 0.<br />Defaults to 0.<br />Example: when this is set to 30%, the old MachineSet can be scaled<br />down to 70% of desired machines immediately when the rolling update<br />starts. Once new machines are ready, old MachineSet can be scaled<br />down further, followed by scaling up the new MachineSet, ensuring<br />that the total number of machines available at all times<br />during the update is at least 70% of desired machines. |  | Optional: \{\} <br /> |
| `maxSurge` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxSurge is the maximum number of machines that can be scheduled above the<br />desired number of machines.<br />Value can be an absolute number (ex: 5) or a percentage of<br />desired machines (ex: 10%).<br />This can not be 0 if MaxUnavailable is 0.<br />Absolute number is calculated from percentage by rounding up.<br />Defaults to 1.<br />Example: when this is set to 30%, the new MachineSet can be scaled<br />up immediately when the rolling update starts, such that the total<br />number of old and new machines do not exceed 130% of desired<br />machines. Once old machines have been killed, new MachineSet can<br />be scaled up further, ensuring that total number of machines running<br />at any time during the update is at most 130% of desired machines. |  | Optional: \{\} <br /> |


#### MachineDeploymentVariables



MachineDeploymentVariables can be used to provide variables for a specific MachineDeployment.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentTopology](#machinedeploymenttopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `overrides` _[ClusterVariable](#clustervariable) array_ | overrides can be used to override Cluster level variables. |  | MaxItems: 1000 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### MachineDrainRule



MachineDrainRule is the Schema for the MachineDrainRule API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `MachineDrainRule` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Required: \{\} <br /> |
| `spec` _[MachineDrainRuleSpec](#machinedrainrulespec)_ | spec defines the spec of a MachineDrainRule. |  | Required: \{\} <br /> |


#### MachineDrainRuleDrainBehavior

_Underlying type:_ _string_

MachineDrainRuleDrainBehavior defines the drain behavior. Can be either "Drain", "Skip", or "WaitCompleted".

_Validation:_
- Enum: [Drain Skip WaitCompleted]

_Appears in:_
- [MachineDrainRuleDrainConfig](#machinedrainruledrainconfig)

| Field | Description |
| --- | --- |
| `Drain` | MachineDrainRuleDrainBehaviorDrain means a Pod should be drained.<br /> |
| `Skip` | MachineDrainRuleDrainBehaviorSkip means the drain for a Pod should be skipped.<br /> |
| `WaitCompleted` | MachineDrainRuleDrainBehaviorWaitCompleted means the Pod should not be evicted,<br />but overall drain should wait until the Pod completes.<br /> |


#### MachineDrainRuleDrainConfig



MachineDrainRuleDrainConfig configures if and how Pods are drained.



_Appears in:_
- [MachineDrainRuleSpec](#machinedrainrulespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `behavior` _[MachineDrainRuleDrainBehavior](#machinedrainruledrainbehavior)_ | behavior defines the drain behavior.<br />Can be either "Drain", "Skip", or "WaitCompleted".<br />"Drain" means that the Pods to which this MachineDrainRule applies will be drained.<br />If behavior is set to "Drain" the order in which Pods are drained can be configured<br />with the order field. When draining Pods of a Node the Pods will be grouped by order<br />and one group after another will be drained (by increasing order). Cluster API will<br />wait until all Pods of a group are terminated / removed from the Node before starting<br />with the next group.<br />"Skip" means that the Pods to which this MachineDrainRule applies will be skipped during drain.<br />"WaitCompleted" means that the pods to which this MachineDrainRule applies will never be evicted<br />and we wait for them to be completed, it is enforced that pods marked with this behavior always have Order=0. |  | Enum: [Drain Skip WaitCompleted] <br />Required: \{\} <br /> |
| `order` _integer_ | order defines the order in which Pods are drained.<br />Pods with higher order are drained after Pods with lower order.<br />order can only be set if behavior is set to "Drain".<br />If order is not set, 0 will be used.<br />Valid values for order are from -2147483648 to 2147483647 (inclusive). |  | Optional: \{\} <br /> |


#### MachineDrainRuleMachineSelector



MachineDrainRuleMachineSelector defines to which Machines this MachineDrainRule should be applied.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDrainRuleSpec](#machinedrainrulespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | selector is a label selector which selects Machines by their labels.<br />This field follows standard label selector semantics; if not present or<br />empty, it selects all Machines.<br />If clusterSelector is also set, then the selector as a whole selects<br />Machines matching selector belonging to Clusters selected by clusterSelector.<br />If clusterSelector is not set, it selects all Machines matching selector in<br />all Clusters. |  | Optional: \{\} <br /> |
| `clusterSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | clusterSelector is a label selector which selects Machines by the labels of<br />their Clusters.<br />This field follows standard label selector semantics; if not present or<br />empty, it selects Machines of all Clusters.<br />If selector is also set, then the selector as a whole selects<br />Machines matching selector belonging to Clusters selected by clusterSelector.<br />If selector is not set, it selects all Machines belonging to Clusters<br />selected by clusterSelector. |  | Optional: \{\} <br /> |


#### MachineDrainRulePodSelector



MachineDrainRulePodSelector defines to which Pods this MachineDrainRule should be applied.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDrainRuleSpec](#machinedrainrulespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | selector is a label selector which selects Pods by their labels.<br />This field follows standard label selector semantics; if not present or<br />empty, it selects all Pods.<br />If namespaceSelector is also set, then the selector as a whole selects<br />Pods matching selector in Namespaces selected by namespaceSelector.<br />If namespaceSelector is not set, it selects all Pods matching selector in<br />all Namespaces. |  | Optional: \{\} <br /> |
| `namespaceSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | namespaceSelector is a label selector which selects Pods by the labels of<br />their Namespaces.<br />This field follows standard label selector semantics; if not present or<br />empty, it selects Pods of all Namespaces.<br />If selector is also set, then the selector as a whole selects<br />Pods matching selector in Namespaces selected by namespaceSelector.<br />If selector is not set, it selects all Pods in Namespaces selected by<br />namespaceSelector. |  | Optional: \{\} <br /> |


#### MachineDrainRuleSpec



MachineDrainRuleSpec defines the spec of a MachineDrainRule.



_Appears in:_
- [MachineDrainRule](#machinedrainrule)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `drain` _[MachineDrainRuleDrainConfig](#machinedrainruledrainconfig)_ | drain configures if and how Pods are drained. |  | Required: \{\} <br /> |
| `machines` _[MachineDrainRuleMachineSelector](#machinedrainrulemachineselector) array_ | machines defines to which Machines this MachineDrainRule should be applied.<br />If machines is not set, the MachineDrainRule applies to all Machines in the Namespace.<br />If machines contains multiple selectors, the results are ORed.<br />Within a single Machine selector the results of selector and clusterSelector are ANDed.<br />Machines will be selected from all Clusters in the Namespace unless otherwise<br />restricted with the clusterSelector.<br />Example: Selects control plane Machines in all Clusters or<br />         Machines with label "os" == "linux" in Clusters with label<br />         "stage" == "production".<br /> - selector:<br />     matchExpressions:<br />     - key: cluster.x-k8s.io/control-plane<br />       operator: Exists<br /> - selector:<br />     matchLabels:<br />       os: linux<br />   clusterSelector:<br />     matchExpressions:<br />     - key: stage<br />       operator: In<br />       values:<br />       - production |  | MaxItems: 32 <br />MinItems: 1 <br />MinProperties: 1 <br />Optional: \{\} <br /> |
| `pods` _[MachineDrainRulePodSelector](#machinedrainrulepodselector) array_ | pods defines to which Pods this MachineDrainRule should be applied.<br />If pods is not set, the MachineDrainRule applies to all Pods in all Namespaces.<br />If pods contains multiple selectors, the results are ORed.<br />Within a single Pod selector the results of selector and namespaceSelector are ANDed.<br />Pods will be selected from all Namespaces unless otherwise<br />restricted with the namespaceSelector.<br />Example: Selects Pods with label "app" == "logging" in all Namespaces or<br />         Pods with label "app" == "prometheus" in the "monitoring"<br />         Namespace.<br /> - selector:<br />     matchExpressions:<br />     - key: app<br />       operator: In<br />       values:<br />       - logging<br /> - selector:<br />     matchLabels:<br />       app: prometheus<br />   namespaceSelector:<br />     matchLabels:<br />       kubernetes.io/metadata.name: monitoring |  | MaxItems: 32 <br />MinItems: 1 <br />MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineHealthCheck



MachineHealthCheck is the Schema for the machinehealthchecks API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `MachineHealthCheck` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[MachineHealthCheckSpec](#machinehealthcheckspec)_ | spec is the specification of machine health check policy |  | Required: \{\} <br /> |


#### MachineHealthCheckChecks



MachineHealthCheckChecks are the checks that are used to evaluate if a Machine is healthy.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineHealthCheckSpec](#machinehealthcheckspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeStartupTimeoutSeconds` _integer_ | nodeStartupTimeoutSeconds allows to set the maximum time for MachineHealthCheck<br />to consider a Machine unhealthy if a corresponding Node isn't associated<br />through a `Spec.ProviderID` field.<br />The duration set in this field is compared to the greatest of:<br />- Cluster's infrastructure ready condition timestamp (if and when available)<br />- Control Plane's initialized condition timestamp (if and when available)<br />- Machine's infrastructure ready condition timestamp (if and when available)<br />- Machine's metadata creation timestamp<br />Defaults to 10 minutes.<br />If you wish to disable this feature, set the value explicitly to 0. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `unhealthyNodeConditions` _[UnhealthyNodeCondition](#unhealthynodecondition) array_ | unhealthyNodeConditions contains a list of conditions that determine<br />whether a node is considered unhealthy. The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the node is unhealthy. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `unhealthyMachineConditions` _[UnhealthyMachineCondition](#unhealthymachinecondition) array_ | unhealthyMachineConditions contains a list of the machine conditions that determine<br />whether a machine is considered unhealthy.  The conditions are combined in a<br />logical OR, i.e. if any of the conditions is met, the machine is unhealthy. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### MachineHealthCheckRemediation



MachineHealthCheckRemediation configures if and how remediations are triggered if a Machine is unhealthy.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineHealthCheckSpec](#machinehealthcheckspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `triggerIf` _[MachineHealthCheckRemediationTriggerIf](#machinehealthcheckremediationtriggerif)_ | triggerIf configures if remediations are triggered.<br />If this field is not set, remediations are always triggered. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `templateRef` _[MachineHealthCheckRemediationTemplateReference](#machinehealthcheckremediationtemplatereference)_ | templateRef is a reference to a remediation template<br />provided by an infrastructure provider.<br />This field is completely optional, when filled, the MachineHealthCheck controller<br />creates a new object from the template referenced and hands off remediation of the machine to<br />a controller that lives outside of Cluster API. |  | Optional: \{\} <br /> |


#### MachineHealthCheckRemediationTemplateReference



MachineHealthCheckRemediationTemplateReference is a reference to a remediation template.



_Appears in:_
- [ControlPlaneClassHealthCheckRemediation](#controlplaneclasshealthcheckremediation)
- [ControlPlaneTopologyHealthCheckRemediation](#controlplanetopologyhealthcheckremediation)
- [MachineDeploymentClassHealthCheckRemediation](#machinedeploymentclasshealthcheckremediation)
- [MachineDeploymentTopologyHealthCheckRemediation](#machinedeploymenttopologyhealthcheckremediation)
- [MachineHealthCheckRemediation](#machinehealthcheckremediation)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _string_ | kind of the remediation template.<br />kind must consist of alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$` <br />Required: \{\} <br /> |
| `name` _string_ | name of the remediation template.<br />name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` <br />Required: \{\} <br /> |
| `apiVersion` _string_ | apiVersion of the remediation template.<br />apiVersion must be fully qualified domain name followed by / and a version.<br />NOTE: This field must be kept in sync with the APIVersion of the remediation template. |  | MaxLength: 317 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/[a-z]([-a-z0-9]*[a-z0-9])?$` <br />Required: \{\} <br /> |


#### MachineHealthCheckRemediationTriggerIf



MachineHealthCheckRemediationTriggerIf configures if remediations are triggered.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineHealthCheckRemediation](#machinehealthcheckremediation)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `unhealthyLessThanOrEqualTo` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | unhealthyLessThanOrEqualTo specifies that remediations are only triggered if the number of<br />unhealthy Machines is less than or equal to the configured value.<br />unhealthyInRange takes precedence if set. |  | Optional: \{\} <br /> |
| `unhealthyInRange` _string_ | unhealthyInRange specifies that remediations are only triggered if the number of<br />unhealthy Machines is in the configured range.<br />Takes precedence over unhealthyLessThanOrEqualTo.<br />Eg. "[3-5]" - This means that remediation will be allowed only when:<br />(a) there are at least 3 unhealthy Machines (and)<br />(b) there are at most 5 unhealthy Machines |  | MaxLength: 32 <br />MinLength: 1 <br />Pattern: `^\[[0-9]+-[0-9]+\]$` <br />Optional: \{\} <br /> |


#### MachineHealthCheckSpec



MachineHealthCheckSpec defines the desired state of MachineHealthCheck.



_Appears in:_
- [MachineHealthCheck](#machinehealthcheck)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | selector is a label selector to match machines whose health will be exercised |  | Required: \{\} <br /> |
| `checks` _[MachineHealthCheckChecks](#machinehealthcheckchecks)_ | checks are the checks that are used to evaluate if a Machine is healthy.<br />Independent of this configuration the MachineHealthCheck controller will always<br />flag Machines with `cluster.x-k8s.io/remediate-machine` annotation and<br />Machines with deleted Nodes as unhealthy.<br />Furthermore, if checks.nodeStartupTimeoutSeconds is not set it<br />is defaulted to 10 minutes and evaluated accordingly. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `remediation` _[MachineHealthCheckRemediation](#machinehealthcheckremediation)_ | remediation configures if and how remediations are triggered if a Machine is unhealthy.<br />If remediation or remediation.triggerIf is not set,<br />remediation will always be triggered for unhealthy Machines.<br />If remediation or remediation.templateRef is not set,<br />the OwnerRemediated condition will be set on unhealthy Machines to trigger remediation via<br />the owner of the Machines, for example a MachineSet or a KubeadmControlPlane. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineNamingSpec



MachineNamingSpec allows changing the naming pattern used when creating
Machines.
Note: InfraMachines & BootstrapConfigs will use the same name as the corresponding Machines.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineDeploymentSpec](#machinedeploymentspec)
- [MachineSetSpec](#machinesetspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the names of the<br />Machine objects.<br />If not defined, it will fallback to `\{\{ .machineSet.name \}\}-\{\{ .random \}\}`.<br />If the generated name string exceeds 63 characters, it will be trimmed to<br />58 characters and will<br />get concatenated with a random suffix of length 5.<br />Length of the template string must not exceed 256 characters.<br />The template allows the following variables `.cluster.name`,<br />`.machineSet.name` and `.random`.<br />The variable `.cluster.name` retrieves the name of the cluster object<br />that owns the Machines being created.<br />The variable `.machineSet.name` retrieves the name of the MachineSet<br />object that owns the Machines being created.<br />The variable `.random` is substituted with random alphanumeric string,<br />without vowels, of length 5. This variable is required part of the<br />template. If not provided, validation will fail. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |






#### MachinePool



MachinePool is the Schema for the machinepools API.
NOTE: This CRD can only be used if the MachinePool feature gate is enabled.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `MachinePool` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[MachinePoolSpec](#machinepoolspec)_ | spec is the desired state of MachinePool. |  | Required: \{\} <br /> |


#### MachinePoolClass



MachinePoolClass serves as a template to define a pool of worker nodes of the cluster
provisioned using `ClusterClass`.



_Appears in:_
- [WorkersClass](#workersclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `class` _string_ | class denotes a type of machine pool present in the cluster,<br />this name MUST be unique within a ClusterClass and can be referenced<br />in the Cluster to create a managed MachinePool. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `bootstrap` _[MachinePoolClassBootstrapTemplate](#machinepoolclassbootstraptemplate)_ | bootstrap contains the bootstrap template reference to be used<br />for the creation of the Machines in the MachinePool. |  | Required: \{\} <br /> |
| `infrastructure` _[MachinePoolClassInfrastructureTemplate](#machinepoolclassinfrastructuretemplate)_ | infrastructure contains the infrastructure template reference to be used<br />for the creation of the MachinePool. |  | Required: \{\} <br /> |
| `failureDomains` _string array_ | failureDomains is the list of failure domains the MachinePool should be attached to.<br />Must match a key in the FailureDomains map stored on the cluster object.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `naming` _[MachinePoolClassNamingSpec](#machinepoolclassnamingspec)_ | naming allows changing the naming pattern used when creating the MachinePool. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `deletion` _[MachinePoolClassMachineDeletionSpec](#machinepoolclassmachinedeletionspec)_ | deletion contains configuration options for Machine deletion. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a newly created machine pool should<br />be ready.<br />Defaults to 0 (machine will be considered available as soon as it<br />is ready)<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### MachinePoolClassBootstrapTemplate



MachinePoolClassBootstrapTemplate defines the BootstrapTemplate for a MachinePool.



_Appears in:_
- [MachinePoolClass](#machinepoolclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `templateRef` _[ClusterClassTemplateReference](#clusterclasstemplatereference)_ | templateRef is a required reference to the BootstrapTemplate for a MachinePool. |  | Required: \{\} <br /> |


#### MachinePoolClassInfrastructureTemplate



MachinePoolClassInfrastructureTemplate defines the InfrastructureTemplate for a MachinePool.



_Appears in:_
- [MachinePoolClass](#machinepoolclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `templateRef` _[ClusterClassTemplateReference](#clusterclasstemplatereference)_ | templateRef is a required reference to the InfrastructureTemplate for a MachinePool. |  | Required: \{\} <br /> |


#### MachinePoolClassMachineDeletionSpec



MachinePoolClassMachineDeletionSpec contains configuration options for Machine deletion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachinePoolClass](#machinepoolclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeDrainTimeoutSeconds` _integer_ | nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout`<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeVolumeDetachTimeoutSeconds` _integer_ | nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeDeletionTimeoutSeconds` _integer_ | nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the Machine<br />hosts after the Machine Pool is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds.<br />NOTE: This value can be overridden while defining a Cluster.Topology using this MachinePoolClass. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### MachinePoolClassNamingSpec



MachinePoolClassNamingSpec defines the naming strategy for MachinePool objects.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachinePoolClass](#machinepoolclass)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the name of the MachinePool object.<br />If not defined, it will fallback to `\{\{ .cluster.name \}\}-\{\{ .machinePool.topologyName \}\}-\{\{ .random \}\}`.<br />If the templated string exceeds 63 characters, it will be trimmed to 58 characters and will<br />get concatenated with a random suffix of length 5.<br />The templating mechanism provides the following arguments:<br />* `.cluster.name`: The name of the cluster object.<br />* `.random`: A random alphanumeric string, without vowels, of length 5.<br />* `.machinePool.topologyName`: The name of the MachinePool topology (Cluster.spec.topology.workers.machinePools[].name). |  | MaxLength: 1024 <br />MinLength: 1 <br />Optional: \{\} <br /> |




#### MachinePoolSpec



MachinePoolSpec defines the desired state of MachinePool.



_Appears in:_
- [MachinePool](#machinepool)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of desired machines. Defaults to 1.<br />This is a pointer to distinguish between explicit zero and not specified. |  | Optional: \{\} <br /> |
| `template` _[MachineTemplateSpec](#machinetemplatespec)_ | template describes the machines that will be created. |  | Required: \{\} <br /> |
| `providerIDList` _string array_ | providerIDList are the identification IDs of machine instances provided by the provider.<br />This field must match the provider IDs as seen on the node objects corresponding to a machine pool's machine instances. |  | MaxItems: 10000 <br />items:MaxLength: 512 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `failureDomains` _string array_ | failureDomains is the list of failure domains this MachinePool should be attached to. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### MachinePoolTopology



MachinePoolTopology specifies the different parameters for a pool of worker nodes in the topology.
This pool of nodes is managed by a MachinePool object whose lifecycle is managed by the Cluster controller.



_Appears in:_
- [WorkersTopology](#workerstopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `class` _string_ | class is the name of the MachinePoolClass used to create the pool of worker nodes.<br />This should match one of the deployment classes defined in the ClusterClass object<br />mentioned in the `Cluster.Spec.Class` field. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `name` _string_ | name is the unique identifier for this MachinePoolTopology.<br />The value is used with other unique identifiers to create a MachinePool's Name<br />(e.g. cluster's name, etc). In case the name is greater than the allowed maximum length,<br />the values are hashed together. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` <br />Required: \{\} <br /> |
| `failureDomains` _string array_ | failureDomains is the list of failure domains the machine pool will be created in.<br />Must match a key in the FailureDomains map stored on the cluster object. |  | MaxItems: 100 <br />MinItems: 1 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |
| `deletion` _[MachinePoolTopologyMachineDeletionSpec](#machinepooltopologymachinedeletionspec)_ | deletion contains configuration options for Machine deletion. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a newly created machine pool should<br />be ready.<br />Defaults to 0 (machine will be considered available as soon as it<br />is ready) |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of nodes belonging to this pool.<br />If the value is nil, the MachinePool is created without the number of Replicas (defaulting to 1)<br />and it's assumed that an external entity (like cluster autoscaler) is responsible for the management<br />of this value. |  | Optional: \{\} <br /> |
| `variables` _[MachinePoolVariables](#machinepoolvariables)_ | variables can be used to customize the MachinePool through patches. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachinePoolTopologyMachineDeletionSpec



MachinePoolTopologyMachineDeletionSpec contains configuration options for Machine deletion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachinePoolTopology](#machinepooltopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeDrainTimeoutSeconds` _integer_ | nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a node.<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout` |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeVolumeDetachTimeoutSeconds` _integer_ | nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeDeletionTimeoutSeconds` _integer_ | nodeDeletionTimeoutSeconds defines how long the controller will attempt to delete the Node that the MachinePool<br />hosts after the MachinePool is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />Defaults to 10 seconds. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### MachinePoolVariables



MachinePoolVariables can be used to provide variables for a specific MachinePool.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachinePoolTopology](#machinepooltopology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `overrides` _[ClusterVariable](#clustervariable) array_ | overrides can be used to override Cluster level variables. |  | MaxItems: 1000 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### MachineReadinessGate



MachineReadinessGate contains the type of a Machine condition to be used as a readiness gate.



_Appears in:_
- [ControlPlaneClass](#controlplaneclass)
- [ControlPlaneTopology](#controlplanetopology)
- [KubeadmControlPlaneMachineTemplateSpec](#kubeadmcontrolplanemachinetemplatespec)
- [MachineDeploymentClass](#machinedeploymentclass)
- [MachineDeploymentTopology](#machinedeploymenttopology)
- [MachineSpec](#machinespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditionType` _string_ | conditionType refers to a condition with matching type in the Machine's condition list.<br />If the conditions doesn't exist, it will be treated as unknown.<br />Note: Both Cluster API conditions or conditions added by 3rd party controllers can be used as readiness gates. |  | MaxLength: 316 <br />MinLength: 1 <br />Pattern: `^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$` <br />Required: \{\} <br /> |
| `polarity` _[ConditionPolarity](#conditionpolarity)_ | polarity of the conditionType specified in this readinessGate.<br />Valid values are Positive, Negative and omitted.<br />When omitted, the default behaviour will be Positive.<br />A positive polarity means that the condition should report a true status under normal conditions.<br />A negative polarity means that the condition should report a false status under normal conditions. |  | Enum: [Positive Negative] <br />Optional: \{\} <br /> |


#### MachineSet



MachineSet is the Schema for the machinesets API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `MachineSet` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[MachineSetSpec](#machinesetspec)_ | spec is the desired state of MachineSet. |  | Required: \{\} <br /> |


#### MachineSetDeletionOrder

_Underlying type:_ _string_

MachineSetDeletionOrder defines how priority is assigned to nodes to delete when
downscaling a MachineSet. Defaults to "Random".

_Validation:_
- Enum: [Random Newest Oldest]

_Appears in:_
- [MachineDeploymentClassMachineDeletionSpec](#machinedeploymentclassmachinedeletionspec)
- [MachineDeploymentDeletionSpec](#machinedeploymentdeletionspec)
- [MachineDeploymentTopologyMachineDeletionSpec](#machinedeploymenttopologymachinedeletionspec)
- [MachineSetDeletionSpec](#machinesetdeletionspec)

| Field | Description |
| --- | --- |
| `Random` | RandomMachineSetDeletionOrder prioritizes both Machines that have the annotation<br />"cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy<br />(Status.FailureReason or Status.FailureMessage are set to a non-empty value<br />or NodeHealthy type of Status.Conditions is not true).<br />Finally, it picks Machines at random to delete.<br /> |
| `Newest` | NewestMachineSetDeletionOrder prioritizes both Machines that have the annotation<br />"cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy<br />(Status.FailureReason or Status.FailureMessage are set to a non-empty value<br />or NodeHealthy type of Status.Conditions is not true).<br />It then prioritizes the newest Machines for deletion based on the Machine's CreationTimestamp.<br /> |
| `Oldest` | OldestMachineSetDeletionOrder prioritizes both Machines that have the annotation<br />"cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy<br />(Status.FailureReason or Status.FailureMessage are set to a non-empty value<br />or NodeHealthy type of Status.Conditions is not true).<br />It then prioritizes the oldest Machines for deletion based on the Machine's CreationTimestamp.<br /> |


#### MachineSetDeletionSpec



MachineSetDeletionSpec contains configuration options for MachineSet deletion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [MachineSetSpec](#machinesetspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `order` _[MachineSetDeletionOrder](#machinesetdeletionorder)_ | order defines the order in which Machines are deleted when downscaling.<br />Defaults to "Random".  Valid values are "Random, "Newest", "Oldest" |  | Enum: [Random Newest Oldest] <br />Optional: \{\} <br /> |




#### MachineSetSpec



MachineSetSpec defines the desired state of MachineSet.



_Appears in:_
- [MachineSet](#machineset)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `replicas` _integer_ | replicas is the number of desired replicas.<br />This is a pointer to distinguish between explicit zero and unspecified.<br />Defaults to:<br />* if the Kubernetes autoscaler min size and max size annotations are set:<br />  - if it's a new MachineSet, use min size<br />  - if the replicas field of the old MachineSet is < min size, use min size<br />  - if the replicas field of the old MachineSet is > max size, use max size<br />  - if the replicas field of the old MachineSet is in the (min size, max size) range, keep the value from the oldMS<br />* otherwise use 1<br />Note: Defaulting will be run whenever the replicas field is not set:<br />* A new MachineSet is created with replicas not set.<br />* On an existing MachineSet the replicas field was first set and is now unset.<br />Those cases are especially relevant for the following Kubernetes autoscaler use cases:<br />* A new MachineSet is created and replicas should be managed by the autoscaler<br />* An existing MachineSet which initially wasn't controlled by the autoscaler<br />  should be later controlled by the autoscaler |  | Optional: \{\} <br /> |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | selector is a label query over machines that should match the replica count.<br />Label keys and values that must match in order to be controlled by this MachineSet.<br />It must match the machine template's labels.<br />More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors |  | Required: \{\} <br /> |
| `template` _[MachineTemplateSpec](#machinetemplatespec)_ | template is the object that describes the machine that will be created if<br />insufficient replicas are detected.<br />Object references to custom resources are treated as templates. |  | Required: \{\} <br /> |
| `machineNaming` _[MachineNamingSpec](#machinenamingspec)_ | machineNaming allows changing the naming pattern used when creating Machines.<br />Note: InfraMachines & BootstrapConfigs will use the same name as the corresponding Machines. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `deletion` _[MachineSetDeletionSpec](#machinesetdeletionspec)_ | deletion contains configuration options for MachineSet deletion. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### MachineSpec



MachineSpec defines the desired state of Machine.



_Appears in:_
- [Machine](#machine)
- [MachineTemplateSpec](#machinetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `bootstrap` _[Bootstrap](#bootstrap)_ | bootstrap is a reference to a local struct which encapsulates<br />fields to configure the Machine’s bootstrapping mechanism. |  | Required: \{\} <br /> |
| `infrastructureRef` _[ContractVersionedObjectReference](#contractversionedobjectreference)_ | infrastructureRef is a required reference to a custom resource<br />offered by an infrastructure provider. |  | Required: \{\} <br /> |
| `version` _string_ | version defines the desired Kubernetes version.<br />This field is meant to be optionally used by bootstrap providers. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `providerID` _string_ | providerID is the identification ID of the machine provided by the provider.<br />This field must match the provider ID as seen on the node object corresponding to this machine.<br />This field is required by higher level consumers of cluster-api. Example use case is cluster autoscaler<br />with cluster-api as provider. Clean-up logic in the autoscaler compares machines to nodes to find out<br />machines at provider which could not get registered as Kubernetes nodes. With cluster-api as a<br />generic out-of-tree provider for autoscaler, this field is required by autoscaler to be<br />able to have a provider view of the list of machines. Another list of nodes is queried from the k8s apiserver<br />and then a comparison is done to find out unregistered machines and are marked for delete.<br />This field will be set by the actuators and consumed by higher level entities like autoscaler that will<br />be interfacing with cluster-api as generic provider. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `failureDomain` _string_ | failureDomain is the failure domain the machine will be created in.<br />Must match the name of a FailureDomain from the Cluster status. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `minReadySeconds` _integer_ | minReadySeconds is the minimum number of seconds for which a Machine should be ready before considering it available.<br />Defaults to 0 (Machine will be considered available as soon as the Machine is ready) |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition.<br />This field can be used e.g. by Cluster API control plane providers to extend the semantic of the<br />Ready condition for the Machine they control, like the kubeadm control provider adding ReadinessGates<br />for the APIServerPodHealthy, SchedulerPodHealthy conditions, etc.<br />Another example are external controllers, e.g. responsible to install special software/hardware on the Machines;<br />they can include the status of those components with a new condition and add this condition to ReadinessGates.<br />NOTE: In case readinessGates conditions start with the APIServer, ControllerManager, Scheduler prefix, and all those<br />readiness gates condition are reporting the same message, when computing the Machine's Ready condition those<br />readinessGates will be replaced by a single entry reporting "Control plane components: " + message.<br />This helps to improve readability of conditions bubbling up to the Machine's owner resource / to the Cluster). |  | MaxItems: 32 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `deletion` _[MachineDeletionSpec](#machinedeletionspec)_ | deletion contains configuration options for Machine deletion. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `taints` _[MachineTaint](#machinetaint) array_ | taints are the node taints that Cluster API will manage.<br />This list is not necessarily complete: other Kubernetes components may add or remove other taints from nodes,<br />e.g. the node controller might add the node.kubernetes.io/not-ready taint.<br />Only those taints defined in this list will be added or removed by core Cluster API controllers.<br />There can be at most 64 taints.<br />A pod would have to tolerate all existing taints to run on the corresponding node.<br />NOTE: This list is implemented as a "map" type, meaning that individual elements can be managed by different owners. |  | MaxItems: 64 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### MachineTaint



MachineTaint defines a taint equivalent to corev1.Taint, but additionally having a propagation field.



_Appears in:_
- [KubeadmControlPlaneMachineTemplateSpec](#kubeadmcontrolplanemachinetemplatespec)
- [KubeadmControlPlaneTemplateMachineTemplateSpec](#kubeadmcontrolplanetemplatemachinetemplatespec)
- [MachineSpec](#machinespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `key` _string_ | key is the taint key to be applied to a node.<br />Must be a valid qualified name of maximum size 63 characters<br />with an optional subdomain prefix of maximum size 253 characters,<br />separated by a `/`. |  | MaxLength: 317 <br />MinLength: 1 <br />Pattern: `^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/)?([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$` <br />Required: \{\} <br /> |
| `value` _string_ | value is the taint value corresponding to the taint key.<br />It must be a valid label value of maximum size 63 characters. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$` <br />Optional: \{\} <br /> |
| `effect` _[TaintEffect](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#tainteffect-v1-core)_ | effect is the effect for the taint. Valid values are NoSchedule, PreferNoSchedule and NoExecute. |  | Enum: [NoSchedule PreferNoSchedule NoExecute] <br />Required: \{\} <br /> |
| `propagation` _[MachineTaintPropagation](#machinetaintpropagation)_ | propagation defines how this taint should be propagated to nodes.<br />Valid values are 'Always' and 'OnInitialization'.<br />Always: The taint will be continuously reconciled. If it is not set for a node, it will be added during reconciliation.<br />OnInitialization: The taint will be added during node initialization. If it gets removed from the node later on it will not get added again. |  | Enum: [Always OnInitialization] <br />Required: \{\} <br /> |


#### MachineTaintPropagation

_Underlying type:_ _string_

MachineTaintPropagation defines when a taint should be propagated to nodes.

_Validation:_
- Enum: [Always OnInitialization]

_Appears in:_
- [MachineTaint](#machinetaint)

| Field | Description |
| --- | --- |
| `Always` | MachineTaintPropagationAlways means the taint should be continuously reconciled and kept on the node.<br />- If an Always taint is added to the Machine, the taint will be added to the node.<br />- If an Always taint is removed from the Machine, the taint will be removed from the node.<br />- If an OnInitialization taint is changed to Always, the Machine controller will ensure the taint is set on the node.<br />- If an Always taint is removed from the node, it will be re-added during reconciliation.<br /> |
| `OnInitialization` | MachineTaintPropagationOnInitialization means the taint should be set once during initialization and then<br />left alone.<br />- If an OnInitialization taint is added to the Machine, the taint will only be added to the node on initialization.<br />- If an OnInitialization taint is removed from the Machine nothing will be changed on the node.<br />- If an Always taint is changed to OnInitialization, the taint will only be added to the node on initialization.<br />- If an OnInitialization taint is removed from the node, it will not be re-added during reconciliation.<br /> |


#### MachineTemplateSpec



MachineTemplateSpec describes the data needed to create a Machine from a template.



_Appears in:_
- [MachineDeploymentSpec](#machinedeploymentspec)
- [MachinePoolSpec](#machinepoolspec)
- [MachineSetSpec](#machinesetspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[MachineSpec](#machinespec)_ | spec is the specification of the desired behavior of the machine.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status |  | Required: \{\} <br /> |


#### NetworkRanges



NetworkRanges represents ranges of network addresses.



_Appears in:_
- [ClusterNetwork](#clusternetwork)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `cidrBlocks` _string array_ | cidrBlocks is a list of CIDR blocks. |  | MaxItems: 100 <br />MinItems: 1 <br />items:MaxLength: 43 <br />items:MinLength: 1 <br />Required: \{\} <br /> |


#### ObjectMeta



ObjectMeta is metadata that all persisted resources must have, which includes all objects
users must create. This is a copy of customizable fields from metav1.ObjectMeta.

ObjectMeta is embedded in `Machine.Spec`, `MachineDeployment.Template` and `MachineSet.Template`,
which are not top-level Kubernetes objects. Given that metav1.ObjectMeta has lots of special cases
and read-only fields which end up in the generated CRD validation, having it as a subset simplifies
the API and some issues that can impact user experience.

During the [upgrade to controller-tools@v2](https://github.com/kubernetes-sigs/cluster-api/pull/1054)
for v1alpha2, we noticed a failure would occur running Cluster API test suite against the new CRDs,
specifically `spec.metadata.creationTimestamp in body must be of type string: "null"`.
The investigation showed that `controller-tools@v2` behaves differently than its previous version
when handling types from [metav1](k8s.io/apimachinery/pkg/apis/meta/v1) package.

In more details, we found that embedded (non-top level) types that embedded `metav1.ObjectMeta`
had validation properties, including for `creationTimestamp` (metav1.Time).
The `metav1.Time` type specifies a custom json marshaller that, when IsZero() is true, returns `null`
which breaks validation because the field isn't marked as nullable.

In future versions, controller-tools@v2 might allow overriding the type and validation for embedded
types. When that happens, this hack should be revisited.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ControlPlaneClass](#controlplaneclass)
- [ControlPlaneTopology](#controlplanetopology)
- [KubeadmConfigTemplateResource](#kubeadmconfigtemplateresource)
- [KubeadmControlPlaneMachineTemplate](#kubeadmcontrolplanemachinetemplate)
- [KubeadmControlPlaneTemplateMachineTemplate](#kubeadmcontrolplanetemplatemachinetemplate)
- [KubeadmControlPlaneTemplateResource](#kubeadmcontrolplanetemplateresource)
- [MachineDeploymentClass](#machinedeploymentclass)
- [MachineDeploymentTopology](#machinedeploymenttopology)
- [MachinePoolClass](#machinepoolclass)
- [MachinePoolTopology](#machinepooltopology)
- [MachineTemplateSpec](#machinetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `labels` _object (keys:string, values:string)_ | labels is a map of string keys and values that can be used to organize and categorize<br />(scope and select) objects. May match selectors of replication controllers<br />and services.<br />More info: http://kubernetes.io/docs/user-guide/labels |  | Optional: \{\} <br /> |
| `annotations` _object (keys:string, values:string)_ | annotations is an unstructured key value map stored with a resource that may be<br />set by external tools to store and retrieve arbitrary metadata. They are not<br />queryable and should be preserved when modifying objects.<br />More info: http://kubernetes.io/docs/user-guide/annotations |  | Optional: \{\} <br /> |


#### PatchDefinition



PatchDefinition defines a patch which is applied to customize the referenced templates.



_Appears in:_
- [ClusterClassPatch](#clusterclasspatch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[PatchSelector](#patchselector)_ | selector defines on which templates the patch should be applied. |  | Required: \{\} <br /> |
| `jsonPatches` _[JSONPatch](#jsonpatch) array_ | jsonPatches defines the patches which should be applied on the templates<br />matching the selector.<br />Note: Patches will be applied in the order of the array. |  | MaxItems: 100 <br />MinItems: 1 <br />Required: \{\} <br /> |


#### PatchSelector



PatchSelector defines on which templates the patch should be applied.
Note: Matching on APIVersion and Kind is mandatory, to enforce that the patches are
written for the correct version. The version of the references in the ClusterClass may
be automatically updated during reconciliation if there is a newer version for the same contract.
Note: The results of selection based on the individual fields are ANDed.



_Appears in:_
- [PatchDefinition](#patchdefinition)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | apiVersion filters templates by apiVersion.<br />apiVersion must be fully qualified domain name followed by / and a version. |  | MaxLength: 317 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/[a-z]([-a-z0-9]*[a-z0-9])?$` <br />Required: \{\} <br /> |
| `kind` _string_ | kind filters templates by kind.<br />kind must consist of alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$` <br />Required: \{\} <br /> |
| `matchResources` _[PatchSelectorMatch](#patchselectormatch)_ | matchResources selects templates based on where they are referenced. |  | MinProperties: 1 <br />Required: \{\} <br /> |


#### PatchSelectorMatch



PatchSelectorMatch selects templates based on where they are referenced.
Note: The selector must match at least one template.
Note: The results of selection based on the individual fields are ORed.

_Validation:_
- MinProperties: 1

_Appears in:_
- [PatchSelector](#patchselector)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `controlPlane` _boolean_ | controlPlane selects templates referenced in .spec.ControlPlane.<br />Note: this will match the controlPlane and also the controlPlane<br />machineInfrastructure (depending on the kind and apiVersion). |  | Optional: \{\} <br /> |
| `infrastructureCluster` _boolean_ | infrastructureCluster selects templates referenced in .spec.infrastructure. |  | Optional: \{\} <br /> |
| `machineDeploymentClass` _[PatchSelectorMatchMachineDeploymentClass](#patchselectormatchmachinedeploymentclass)_ | machineDeploymentClass selects templates referenced in specific MachineDeploymentClasses in<br />.spec.workers.machineDeployments. |  | Optional: \{\} <br /> |
| `machinePoolClass` _[PatchSelectorMatchMachinePoolClass](#patchselectormatchmachinepoolclass)_ | machinePoolClass selects templates referenced in specific MachinePoolClasses in<br />.spec.workers.machinePools. |  | Optional: \{\} <br /> |


#### PatchSelectorMatchMachineDeploymentClass



PatchSelectorMatchMachineDeploymentClass selects templates referenced
in specific MachineDeploymentClasses in .spec.workers.machineDeployments.



_Appears in:_
- [PatchSelectorMatch](#patchselectormatch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `names` _string array_ | names selects templates by class names. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### PatchSelectorMatchMachinePoolClass



PatchSelectorMatchMachinePoolClass selects templates referenced
in specific MachinePoolClasses in .spec.workers.machinePools.



_Appears in:_
- [PatchSelectorMatch](#patchselectormatch)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `names` _string array_ | names selects templates by class names. |  | MaxItems: 100 <br />items:MaxLength: 256 <br />items:MinLength: 1 <br />Optional: \{\} <br /> |


#### Topology



Topology encapsulates the information of the managed resources.



_Appears in:_
- [ClusterSpec](#clusterspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `classRef` _[ClusterClassRef](#clusterclassref)_ | classRef is the ref to the ClusterClass that should be used for the topology. |  | Required: \{\} <br /> |
| `version` _string_ | version is the Kubernetes version of the cluster. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `controlPlane` _[ControlPlaneTopology](#controlplanetopology)_ | controlPlane describes the cluster control plane. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `workers` _[WorkersTopology](#workerstopology)_ | workers encapsulates the different constructs that form the worker nodes<br />for the cluster. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `variables` _[ClusterVariable](#clustervariable) array_ | variables can be used to customize the Cluster through<br />patches. They must comply to the corresponding<br />VariableClasses defined in the ClusterClass. |  | MaxItems: 1000 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### UnhealthyMachineCondition



UnhealthyMachineCondition represents a Machine condition type and value with a timeout
specified as a duration.  When the named condition has been in the given
status for at least the timeout value, a machine is considered unhealthy.



_Appears in:_
- [ControlPlaneClassHealthCheckChecks](#controlplaneclasshealthcheckchecks)
- [ControlPlaneTopologyHealthCheckChecks](#controlplanetopologyhealthcheckchecks)
- [MachineDeploymentClassHealthCheckChecks](#machinedeploymentclasshealthcheckchecks)
- [MachineDeploymentTopologyHealthCheckChecks](#machinedeploymenttopologyhealthcheckchecks)
- [MachineHealthCheckChecks](#machinehealthcheckchecks)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | type of Machine condition |  | MaxLength: 316 <br />MinLength: 1 <br />Pattern: `^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$` <br />Required: \{\} <br /> |
| `timeoutSeconds` _integer_ | timeoutSeconds is the duration that a machine must be in a given status for,<br />after which the machine is considered unhealthy.<br />For example, with a value of "3600", the machine must match the status<br />for at least 1 hour before being considered unhealthy. |  | Minimum: 0 <br />Required: \{\} <br /> |


#### UnhealthyNodeCondition



UnhealthyNodeCondition represents a Node condition type and value with a timeout
specified as a duration.  When the named condition has been in the given
status for at least the timeout value, a node is considered unhealthy.



_Appears in:_
- [ControlPlaneClassHealthCheckChecks](#controlplaneclasshealthcheckchecks)
- [ControlPlaneTopologyHealthCheckChecks](#controlplanetopologyhealthcheckchecks)
- [MachineDeploymentClassHealthCheckChecks](#machinedeploymentclasshealthcheckchecks)
- [MachineDeploymentTopologyHealthCheckChecks](#machinedeploymenttopologyhealthcheckchecks)
- [MachineHealthCheckChecks](#machinehealthcheckchecks)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[NodeConditionType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#nodeconditiontype-v1-core)_ | type of Node condition |  | MinLength: 1 <br />Type: string <br />Required: \{\} <br /> |
| `timeoutSeconds` _integer_ | timeoutSeconds is the duration that a node must be in a given status for,<br />after which the node is considered unhealthy.<br />For example, with a value of "3600", the node must match the status<br />for at least 1 hour before being considered unhealthy. |  | Minimum: 0 <br />Required: \{\} <br /> |




#### VariableSchema



VariableSchema defines the schema of a variable.



_Appears in:_
- [ClusterClassStatusVariableDefinition](#clusterclassstatusvariabledefinition)
- [ClusterClassVariable](#clusterclassvariable)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `openAPIV3Schema` _[JSONSchemaProps](#jsonschemaprops)_ | openAPIV3Schema defines the schema of a variable via OpenAPI v3<br />schema. The schema is a subset of the schema used in<br />Kubernetes CRDs. |  | MinProperties: 1 <br />Required: \{\} <br /> |


#### VariableSchemaMetadata

_Underlying type:_ _[struct{Labels map[string]string "json:\"labels,omitempty\""; Annotations map[string]string "json:\"annotations,omitempty\""}](#struct{labels-map[string]string-"json:\"labels,omitempty\"";-annotations-map[string]string-"json:\"annotations,omitempty\""})_

VariableSchemaMetadata is the metadata of a variable or a nested field within a variable.
It can be used to add additional data for higher level tools.

_Validation:_
- MinProperties: 1

_Appears in:_
- [JSONSchemaProps](#jsonschemaprops)



#### WorkersClass



WorkersClass is a collection of deployment classes.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ClusterClassSpec](#clusterclassspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `machineDeployments` _[MachineDeploymentClass](#machinedeploymentclass) array_ | machineDeployments is a list of machine deployment classes that can be used to create<br />a set of worker nodes. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `machinePools` _[MachinePoolClass](#machinepoolclass) array_ | machinePools is a list of machine pool classes that can be used to create<br />a set of worker nodes. |  | MaxItems: 100 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### WorkersTopology



WorkersTopology represents the different sets of worker nodes in the cluster.

_Validation:_
- MinProperties: 1

_Appears in:_
- [Topology](#topology)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `machineDeployments` _[MachineDeploymentTopology](#machinedeploymenttopology) array_ | machineDeployments is a list of machine deployments in the cluster. |  | MaxItems: 2000 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `machinePools` _[MachinePoolTopology](#machinepooltopology) array_ | machinePools is a list of machine pools in the cluster. |  | MaxItems: 2000 <br />MinItems: 1 <br />Optional: \{\} <br /> |



## controlplane.cluster.x-k8s.io/v1beta1

Package v1beta1 contains API Schema definitions for the kubeadm v1beta1 API group,

Deprecated: This package is deprecated and is going to be removed when support for v1beta1 will be dropped.

### Resource Types
- [KubeadmControlPlane](#kubeadmcontrolplane)
- [KubeadmControlPlaneTemplate](#kubeadmcontrolplanetemplate)



#### KubeadmControlPlane



KubeadmControlPlane is the Schema for the KubeadmControlPlane API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `controlplane.cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `KubeadmControlPlane` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)_ | spec is the desired state of KubeadmControlPlane. |  | Optional: \{\} <br /> |


#### KubeadmControlPlaneMachineTemplate



KubeadmControlPlaneMachineTemplate defines the template for Machines
in a KubeadmControlPlane object.



_Appears in:_
- [KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `infrastructureRef` _[ObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectreference-v1-core)_ | infrastructureRef is a required reference to a custom resource<br />offered by an infrastructure provider. |  | Required: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition;<br />KubeadmControlPlane will always add readinessGates for the condition it is setting on the Machine:<br />APIServerPodHealthy, SchedulerPodHealthy, ControllerManagerPodHealthy, and if etcd is managed by CKP also<br />EtcdPodHealthy, EtcdMemberHealthy.<br />This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready<br />computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine.<br />NOTE: This field is considered only for computing v1beta2 conditions. |  | MaxItems: 32 <br />Optional: \{\} <br /> |
| `nodeDrainTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDrainTimeout is the total amount of time that the controller will spend on draining a controlplane node<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: NodeDrainTimeout is different from `kubectl drain --timeout` |  | Optional: \{\} <br /> |
| `nodeVolumeDetachTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Optional: \{\} <br /> |
| `nodeDeletionTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDeletionTimeout defines how long the machine controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />If no value is provided, the default value for this property of the Machine resource will be used. |  | Optional: \{\} <br /> |


#### KubeadmControlPlaneSpec



KubeadmControlPlaneSpec defines the desired state of KubeadmControlPlane.



_Appears in:_
- [KubeadmControlPlane](#kubeadmcontrolplane)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `replicas` _integer_ | replicas is the number of desired machines. Defaults to 1. When stacked etcd is used only<br />odd numbers are permitted, as per [etcd best practice](https://etcd.io/docs/v3.3.12/faq/#why-an-odd-number-of-cluster-members).<br />This is a pointer to distinguish between explicit zero and not specified. |  | Optional: \{\} <br /> |
| `version` _string_ | version defines the desired Kubernetes version.<br />Please note that if kubeadmConfigSpec.ClusterConfiguration.imageRepository is not set<br />we don't allow upgrades to versions >= v1.22.0 for which kubeadm uses the old registry (k8s.gcr.io).<br />Please use a newer patch version with the new registry instead. The default registries of kubeadm are:<br />  * registry.k8s.io (new registry): >= v1.22.17, >= v1.23.15, >= v1.24.9, >= v1.25.0<br />  * k8s.gcr.io (old registry): all older versions |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `machineTemplate` _[KubeadmControlPlaneMachineTemplate](#kubeadmcontrolplanemachinetemplate)_ | machineTemplate contains information about how machines<br />should be shaped when creating or updating a control plane. |  | Required: \{\} <br /> |
| `kubeadmConfigSpec` _[KubeadmConfigSpec](#kubeadmconfigspec)_ | kubeadmConfigSpec is a KubeadmConfigSpec<br />to use for initializing and joining machines to the control plane. |  | Required: \{\} <br /> |
| `rolloutBefore` _[RolloutBefore](#rolloutbefore)_ | rolloutBefore is a field to indicate a rollout should be performed<br />if the specified criteria is met. |  | Optional: \{\} <br /> |
| `rolloutStrategy` _[RolloutStrategy](#rolloutstrategy)_ | rolloutStrategy is the RolloutStrategy to use to replace control plane machines with<br />new ones. | \{ rollingUpdate:map[maxSurge:1] type:RollingUpdate \} | Optional: \{\} <br /> |
| `remediationStrategy` _[RemediationStrategy](#remediationstrategy)_ | remediationStrategy is the RemediationStrategy that controls how control plane machine remediation happens. |  | Optional: \{\} <br /> |
| `machineNamingStrategy` _[MachineNamingStrategy](#machinenamingstrategy)_ | machineNamingStrategy allows changing the naming pattern used when creating Machines.<br />InfraMachines & KubeadmConfigs will use the same name as the corresponding Machines. |  | Optional: \{\} <br /> |


#### KubeadmControlPlaneTemplate



KubeadmControlPlaneTemplate is the Schema for the kubeadmcontrolplanetemplates API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `controlplane.cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `KubeadmControlPlaneTemplate` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmControlPlaneTemplateSpec](#kubeadmcontrolplanetemplatespec)_ | spec is the desired state of KubeadmControlPlaneTemplate. |  | Optional: \{\} <br /> |


#### KubeadmControlPlaneTemplateMachineTemplate



KubeadmControlPlaneTemplateMachineTemplate defines the template for Machines
in a KubeadmControlPlaneTemplate object.
NOTE: KubeadmControlPlaneTemplateMachineTemplate is similar to KubeadmControlPlaneMachineTemplate but
omits ObjectMeta and InfrastructureRef fields. These fields do not make sense on the KubeadmControlPlaneTemplate,
because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
be configured on the KubeadmControlPlaneTemplate.



_Appears in:_
- [KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `nodeDrainTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDrainTimeout is the total amount of time that the controller will spend on draining a controlplane node<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: NodeDrainTimeout is different from `kubectl drain --timeout` |  | Optional: \{\} <br /> |
| `nodeVolumeDetachTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Optional: \{\} <br /> |
| `nodeDeletionTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | nodeDeletionTimeout defines how long the machine controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />If no value is provided, the default value for this property of the Machine resource will be used. |  | Optional: \{\} <br /> |


#### KubeadmControlPlaneTemplateResource



KubeadmControlPlaneTemplateResource describes the data needed to create a KubeadmControlPlane from a template.



_Appears in:_
- [KubeadmControlPlaneTemplateSpec](#kubeadmcontrolplanetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)_ | spec is the desired state of KubeadmControlPlaneTemplateResource. |  | Required: \{\} <br /> |


#### KubeadmControlPlaneTemplateResourceSpec



KubeadmControlPlaneTemplateResourceSpec defines the desired state of KubeadmControlPlane.
NOTE: KubeadmControlPlaneTemplateResourceSpec is similar to KubeadmControlPlaneSpec but
omits Replicas and Version fields. These fields do not make sense on the KubeadmControlPlaneTemplate,
because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
be configured on the KubeadmControlPlaneTemplate.



_Appears in:_
- [KubeadmControlPlaneTemplateResource](#kubeadmcontrolplanetemplateresource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `machineTemplate` _[KubeadmControlPlaneTemplateMachineTemplate](#kubeadmcontrolplanetemplatemachinetemplate)_ | machineTemplate contains information about how machines<br />should be shaped when creating or updating a control plane. |  | Optional: \{\} <br /> |
| `kubeadmConfigSpec` _[KubeadmConfigSpec](#kubeadmconfigspec)_ | kubeadmConfigSpec is a KubeadmConfigSpec<br />to use for initializing and joining machines to the control plane. |  | Required: \{\} <br /> |
| `rolloutBefore` _[RolloutBefore](#rolloutbefore)_ | rolloutBefore is a field to indicate a rollout should be performed<br />if the specified criteria is met. |  | Optional: \{\} <br /> |
| `rolloutStrategy` _[RolloutStrategy](#rolloutstrategy)_ | rolloutStrategy is the RolloutStrategy to use to replace control plane machines with<br />new ones. | \{ rollingUpdate:map[maxSurge:1] type:RollingUpdate \} | Optional: \{\} <br /> |
| `remediationStrategy` _[RemediationStrategy](#remediationstrategy)_ | remediationStrategy is the RemediationStrategy that controls how control plane machine remediation happens. |  | Optional: \{\} <br /> |
| `machineNamingStrategy` _[MachineNamingStrategy](#machinenamingstrategy)_ | machineNamingStrategy allows changing the naming pattern used when creating Machines.<br />InfraMachines & KubeadmConfigs will use the same name as the corresponding Machines. |  | Optional: \{\} <br /> |


#### KubeadmControlPlaneTemplateSpec



KubeadmControlPlaneTemplateSpec defines the desired state of KubeadmControlPlaneTemplate.



_Appears in:_
- [KubeadmControlPlaneTemplate](#kubeadmcontrolplanetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _[KubeadmControlPlaneTemplateResource](#kubeadmcontrolplanetemplateresource)_ | template defines the desired state of KubeadmControlPlaneTemplate. |  | Required: \{\} <br /> |


#### MachineNamingStrategy



MachineNamingStrategy allows changing the naming pattern used when creating Machines.
InfraMachines & KubeadmConfigs will use the same name as the corresponding Machines.



_Appears in:_
- [KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)
- [KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the names of the Machine objects.<br />If not defined, it will fallback to `\{\{ .kubeadmControlPlane.name \}\}-\{\{ .random \}\}`.<br />If the generated name string exceeds 63 characters, it will be trimmed to 58 characters and will<br />get concatenated with a random suffix of length 5.<br />Length of the template string must not exceed 256 characters.<br />The template allows the following variables `.cluster.name`, `.kubeadmControlPlane.name` and `.random`.<br />The variable `.cluster.name` retrieves the name of the cluster object that owns the Machines being created.<br />The variable `.kubeadmControlPlane.name` retrieves the name of the KubeadmControlPlane object that owns the Machines being created.<br />The variable `.random` is substituted with random alphanumeric string, without vowels, of length 5. This variable is required<br />part of the template. If not provided, validation will fail. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### RemediationStrategy



RemediationStrategy allows to define how control plane machine remediation happens.



_Appears in:_
- [KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)
- [KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxRetry` _integer_ | maxRetry is the Max number of retries while attempting to remediate an unhealthy machine.<br />A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.<br />For example, given a control plane with three machines M1, M2, M3:<br />	M1 become unhealthy; remediation happens, and M1-1 is created as a replacement.<br />	If M1-1 (replacement of M1) has problems while bootstrapping it will become unhealthy, and then be<br />	remediated; such operation is considered a retry, remediation-retry #1.<br />	If M1-2 (replacement of M1-1) becomes unhealthy, remediation-retry #2 will happen, etc.<br />A retry could happen only after RetryPeriod from the previous retry.<br />If a machine is marked as unhealthy after MinHealthyPeriod from the previous remediation expired,<br />this is not considered a retry anymore because the new issue is assumed unrelated from the previous one.<br />If not set, the remedation will be retried infinitely. |  | Optional: \{\} <br /> |
| `retryPeriod` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | retryPeriod is the duration that KCP should wait before remediating a machine being created as a replacement<br />for an unhealthy machine (a retry).<br />If not set, a retry will happen immediately. |  | Optional: \{\} <br /> |
| `minHealthyPeriod` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#duration-v1-meta)_ | minHealthyPeriod defines the duration after which KCP will consider any failure to a machine unrelated<br />from the previous one. In this case the remediation is not considered a retry anymore, and thus the retry<br />counter restarts from 0. For example, assuming MinHealthyPeriod is set to 1h (default)<br />	M1 become unhealthy; remediation happens, and M1-1 is created as a replacement.<br />	If M1-1 (replacement of M1) has problems within the 1hr after the creation, also<br />	this machine will be remediated and this operation is considered a retry - a problem related<br />	to the original issue happened to M1 -.<br />	If instead the problem on M1-1 is happening after MinHealthyPeriod expired, e.g. four days after<br />	m1-1 has been created as a remediation of M1, the problem on M1-1 is considered unrelated to<br />	the original issue happened to M1.<br />If not set, this value is defaulted to 1h. |  | Optional: \{\} <br /> |


#### RollingUpdate



RollingUpdate is used to control the desired behavior of rolling update.



_Appears in:_
- [RolloutStrategy](#rolloutstrategy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxSurge` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxSurge is the maximum number of control planes that can be scheduled above or under the<br />desired number of control planes.<br />Value can be an absolute number 1 or 0.<br />Defaults to 1.<br />Example: when this is set to 1, the control plane can be scaled<br />up immediately when the rolling update starts. |  | Optional: \{\} <br /> |


#### RolloutBefore



RolloutBefore describes when a rollout should be performed on the KCP machines.



_Appears in:_
- [KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)
- [KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `certificatesExpiryDays` _integer_ | certificatesExpiryDays indicates a rollout needs to be performed if the<br />certificates of the machine will expire within the specified days. |  | Optional: \{\} <br /> |


#### RolloutStrategy



RolloutStrategy describes how to replace existing machines
with new ones.



_Appears in:_
- [KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)
- [KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[RolloutStrategyType](#rolloutstrategytype)_ | type of rollout. Currently the only supported strategy is<br />"RollingUpdate".<br />Default is RollingUpdate. |  | Enum: [RollingUpdate] <br />Optional: \{\} <br /> |
| `rollingUpdate` _[RollingUpdate](#rollingupdate)_ | rollingUpdate is the rolling update config params. Present only if<br />RolloutStrategyType = RollingUpdate. |  | Optional: \{\} <br /> |


#### RolloutStrategyType

_Underlying type:_ _string_

RolloutStrategyType defines the rollout strategies for a KubeadmControlPlane.

_Validation:_
- Enum: [RollingUpdate]

_Appears in:_
- [RolloutStrategy](#rolloutstrategy)

| Field | Description |
| --- | --- |
| `RollingUpdate` | RollingUpdateStrategyType replaces the old control planes by new one using rolling update<br />i.e. gradually scale up or down the old control planes and scale up or down the new one.<br /> |



## controlplane.cluster.x-k8s.io/v1beta2

Package v1beta2 contains API Schema definitions for the kubeadm v1beta2 API group.

### Resource Types
- [KubeadmControlPlane](#kubeadmcontrolplane)
- [KubeadmControlPlaneTemplate](#kubeadmcontrolplanetemplate)



#### KubeadmControlPlane



KubeadmControlPlane is the Schema for the KubeadmControlPlane API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `controlplane.cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `KubeadmControlPlane` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)_ | spec is the desired state of KubeadmControlPlane. |  | Required: \{\} <br /> |


#### KubeadmControlPlaneMachineTemplate



KubeadmControlPlaneMachineTemplate defines the template for Machines
in a KubeadmControlPlane object.



_Appears in:_
- [KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmControlPlaneMachineTemplateSpec](#kubeadmcontrolplanemachinetemplatespec)_ | spec defines the spec for Machines<br />in a KubeadmControlPlane object. |  | Required: \{\} <br /> |


#### KubeadmControlPlaneMachineTemplateDeletionSpec



KubeadmControlPlaneMachineTemplateDeletionSpec contains configuration options for Machine deletion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneMachineTemplateSpec](#kubeadmcontrolplanemachinetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeDrainTimeoutSeconds` _integer_ | nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a controlplane node<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout` |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeVolumeDetachTimeoutSeconds` _integer_ | nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeDeletionTimeoutSeconds` _integer_ | nodeDeletionTimeoutSeconds defines how long the machine controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />If no value is provided, the default value for this property of the Machine resource will be used. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneMachineTemplateSpec



KubeadmControlPlaneMachineTemplateSpec defines the spec for Machines
in a KubeadmControlPlane object.



_Appears in:_
- [KubeadmControlPlaneMachineTemplate](#kubeadmcontrolplanemachinetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `infrastructureRef` _[ContractVersionedObjectReference](#contractversionedobjectreference)_ | infrastructureRef is a required reference to a custom resource<br />offered by an infrastructure provider. |  | Required: \{\} <br /> |
| `readinessGates` _[MachineReadinessGate](#machinereadinessgate) array_ | readinessGates specifies additional conditions to include when evaluating Machine Ready condition;<br />KubeadmControlPlane will always add readinessGates for the condition it is setting on the Machine:<br />APIServerPodHealthy, SchedulerPodHealthy, ControllerManagerPodHealthy, and if etcd is managed by CKP also<br />EtcdPodHealthy, EtcdMemberHealthy.<br />This field can be used e.g. to instruct the machine controller to include in the computation for Machine's ready<br />computation a condition, managed by an external controllers, reporting the status of special software/hardware installed on the Machine. |  | MaxItems: 32 <br />MinItems: 1 <br />Optional: \{\} <br /> |
| `deletion` _[KubeadmControlPlaneMachineTemplateDeletionSpec](#kubeadmcontrolplanemachinetemplatedeletionspec)_ | deletion contains configuration options for Machine deletion. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `taints` _[MachineTaint](#machinetaint) array_ | taints are the node taints that Cluster API will manage.<br />This list is not necessarily complete: other Kubernetes components may add or remove other taints from nodes,<br />e.g. the node controller might add the node.kubernetes.io/not-ready taint.<br />Only those taints defined in this list will be added or removed by core Cluster API controllers.<br />There can be at most 64 taints.<br />A pod would have to tolerate all existing taints to run on the corresponding node.<br />NOTE: This list is implemented as a "map" type, meaning that individual elements can be managed by different owners. |  | MaxItems: 64 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneRemediationSpec



KubeadmControlPlaneRemediationSpec controls how unhealthy control plane Machines are remediated.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)
- [KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxRetry` _integer_ | maxRetry is the Max number of retries while attempting to remediate an unhealthy machine.<br />A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.<br />For example, given a control plane with three machines M1, M2, M3:<br />	M1 become unhealthy; remediation happens, and M1-1 is created as a replacement.<br />	If M1-1 (replacement of M1) has problems while bootstrapping it will become unhealthy, and then be<br />	remediated; such operation is considered a retry, remediation-retry #1.<br />	If M1-2 (replacement of M1-1) becomes unhealthy, remediation-retry #2 will happen, etc.<br />A retry could happen only after retryPeriodSeconds from the previous retry.<br />If a machine is marked as unhealthy after minHealthyPeriodSeconds from the previous remediation expired,<br />this is not considered a retry anymore because the new issue is assumed unrelated from the previous one.<br />If not set, the remedation will be retried infinitely. |  | Optional: \{\} <br /> |
| `retryPeriodSeconds` _integer_ | retryPeriodSeconds is the duration that KCP should wait before remediating a machine being created as a replacement<br />for an unhealthy machine (a retry).<br />If not set, a retry will happen immediately. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `minHealthyPeriodSeconds` _integer_ | minHealthyPeriodSeconds defines the duration after which KCP will consider any failure to a machine unrelated<br />from the previous one. In this case the remediation is not considered a retry anymore, and thus the retry<br />counter restarts from 0. For example, assuming minHealthyPeriodSeconds is set to 1h (default)<br />	M1 become unhealthy; remediation happens, and M1-1 is created as a replacement.<br />	If M1-1 (replacement of M1) has problems within the 1hr after the creation, also<br />	this machine will be remediated and this operation is considered a retry - a problem related<br />	to the original issue happened to M1 -.<br />	If instead the problem on M1-1 is happening after minHealthyPeriodSeconds expired, e.g. four days after<br />	m1-1 has been created as a remediation of M1, the problem on M1-1 is considered unrelated to<br />	the original issue happened to M1.<br />If not set, this value is defaulted to 1h. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneRolloutBeforeSpec



KubeadmControlPlaneRolloutBeforeSpec describes when a rollout should be performed on the KCP machines.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneRolloutSpec](#kubeadmcontrolplanerolloutspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `certificatesExpiryDays` _integer_ | certificatesExpiryDays indicates a rollout needs to be performed if the<br />certificates of the machine will expire within the specified days.<br />The minimum for this field is 7. |  | Minimum: 7 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneRolloutSpec



KubeadmControlPlaneRolloutSpec allows you to configure the behaviour of rolling updates to the control plane Machines.
It allows you to require that all Machines are replaced before or after a certain time,
and allows you to define the strategy used during rolling replacements.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)
- [KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `before` _[KubeadmControlPlaneRolloutBeforeSpec](#kubeadmcontrolplanerolloutbeforespec)_ | before is a field to indicate a rollout should be performed<br />if the specified criteria is met. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `strategy` _[KubeadmControlPlaneRolloutStrategy](#kubeadmcontrolplanerolloutstrategy)_ | strategy specifies how to roll out control plane Machines. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneRolloutStrategy



KubeadmControlPlaneRolloutStrategy describes how to replace existing machines
with new ones.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneRolloutSpec](#kubeadmcontrolplanerolloutspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[KubeadmControlPlaneRolloutStrategyType](#kubeadmcontrolplanerolloutstrategytype)_ | type of rollout. Currently the only supported strategy is<br />"RollingUpdate".<br />Default is RollingUpdate. |  | Enum: [RollingUpdate] <br />Required: \{\} <br /> |
| `rollingUpdate` _[KubeadmControlPlaneRolloutStrategyRollingUpdate](#kubeadmcontrolplanerolloutstrategyrollingupdate)_ | rollingUpdate is the rolling update config params. Present only if<br />type = RollingUpdate. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneRolloutStrategyRollingUpdate



KubeadmControlPlaneRolloutStrategyRollingUpdate is used to control the desired behavior of rolling update.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneRolloutStrategy](#kubeadmcontrolplanerolloutstrategy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxSurge` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#intorstring-intstr-util)_ | maxSurge is the maximum number of control planes that can be scheduled above or under the<br />desired number of control planes.<br />Value can be an absolute number 1 or 0.<br />Defaults to 1.<br />Example: when this is set to 1, the control plane can be scaled<br />up immediately when the rolling update starts. |  | Optional: \{\} <br /> |


#### KubeadmControlPlaneRolloutStrategyType

_Underlying type:_ _string_

KubeadmControlPlaneRolloutStrategyType defines the rollout strategies for a KubeadmControlPlane.

_Validation:_
- Enum: [RollingUpdate]

_Appears in:_
- [KubeadmControlPlaneRolloutStrategy](#kubeadmcontrolplanerolloutstrategy)

| Field | Description |
| --- | --- |
| `RollingUpdate` | RollingUpdateStrategyType replaces the old control planes by new one using rolling update<br />i.e. gradually scale up or down the old control planes and scale up or down the new one.<br /> |


#### KubeadmControlPlaneSpec



KubeadmControlPlaneSpec defines the desired state of KubeadmControlPlane.



_Appears in:_
- [KubeadmControlPlane](#kubeadmcontrolplane)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `replicas` _integer_ | replicas is the number of desired machines. Defaults to 1. When stacked etcd is used only<br />odd numbers are permitted, as per [etcd best practice](https://etcd.io/docs/v3.3.12/faq/#why-an-odd-number-of-cluster-members).<br />This is a pointer to distinguish between explicit zero and not specified. |  | Optional: \{\} <br /> |
| `version` _string_ | version defines the desired Kubernetes version. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `machineTemplate` _[KubeadmControlPlaneMachineTemplate](#kubeadmcontrolplanemachinetemplate)_ | machineTemplate contains information about how machines<br />should be shaped when creating or updating a control plane. |  | Required: \{\} <br /> |
| `kubeadmConfigSpec` _[KubeadmConfigSpec](#kubeadmconfigspec)_ | kubeadmConfigSpec is a KubeadmConfigSpec<br />to use for initializing and joining machines to the control plane. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `rollout` _[KubeadmControlPlaneRolloutSpec](#kubeadmcontrolplanerolloutspec)_ | rollout allows you to configure the behaviour of rolling updates to the control plane Machines.<br />It allows you to require that all Machines are replaced before or after a certain time,<br />and allows you to define the strategy used during rolling replacements. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `remediation` _[KubeadmControlPlaneRemediationSpec](#kubeadmcontrolplaneremediationspec)_ | remediation controls how unhealthy Machines are remediated. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `machineNaming` _[MachineNamingSpec](#machinenamingspec)_ | machineNaming allows changing the naming pattern used when creating Machines.<br />InfraMachines & KubeadmConfigs will use the same name as the corresponding Machines. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneTemplate



KubeadmControlPlaneTemplate is the Schema for the kubeadmcontrolplanetemplates API.
NOTE: This CRD can only be used if the ClusterTopology feature gate is enabled.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `controlplane.cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `KubeadmControlPlaneTemplate` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmControlPlaneTemplateSpec](#kubeadmcontrolplanetemplatespec)_ | spec is the desired state of KubeadmControlPlaneTemplate. |  | Optional: \{\} <br /> |


#### KubeadmControlPlaneTemplateMachineTemplate



KubeadmControlPlaneTemplateMachineTemplate defines the template for Machines
in a KubeadmControlPlaneTemplate object.
NOTE: KubeadmControlPlaneTemplateMachineTemplate is similar to KubeadmControlPlaneMachineTemplate but
omits ObjectMeta and InfrastructureRef fields. These fields do not make sense on the KubeadmControlPlaneTemplate,
because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
be configured on the KubeadmControlPlaneTemplate.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmControlPlaneTemplateMachineTemplateSpec](#kubeadmcontrolplanetemplatemachinetemplatespec)_ | spec defines the spec for Machines<br />in a KubeadmControlPlane object. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneTemplateMachineTemplateDeletionSpec



KubeadmControlPlaneTemplateMachineTemplateDeletionSpec contains configuration options for Machine deletion.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneTemplateMachineTemplateSpec](#kubeadmcontrolplanetemplatemachinetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `nodeDrainTimeoutSeconds` _integer_ | nodeDrainTimeoutSeconds is the total amount of time that the controller will spend on draining a controlplane node<br />The default value is 0, meaning that the node can be drained without any time limitations.<br />NOTE: nodeDrainTimeoutSeconds is different from `kubectl drain --timeout` |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeVolumeDetachTimeoutSeconds` _integer_ | nodeVolumeDetachTimeoutSeconds is the total amount of time that the controller will spend on waiting for all volumes<br />to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations. |  | Minimum: 0 <br />Optional: \{\} <br /> |
| `nodeDeletionTimeoutSeconds` _integer_ | nodeDeletionTimeoutSeconds defines how long the machine controller will attempt to delete the Node that the Machine<br />hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.<br />If no value is provided, the default value for this property of the Machine resource will be used. |  | Minimum: 0 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneTemplateMachineTemplateSpec



KubeadmControlPlaneTemplateMachineTemplateSpec defines the spec for Machines
in a KubeadmControlPlane object.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneTemplateMachineTemplate](#kubeadmcontrolplanetemplatemachinetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `deletion` _[KubeadmControlPlaneTemplateMachineTemplateDeletionSpec](#kubeadmcontrolplanetemplatemachinetemplatedeletionspec)_ | deletion contains configuration options for Machine deletion. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `taints` _[MachineTaint](#machinetaint) array_ | taints are the node taints that Cluster API will manage.<br />This list is not necessarily complete: other Kubernetes components may add or remove other taints from nodes,<br />e.g. the node controller might add the node.kubernetes.io/not-ready taint.<br />Only those taints defined in this list will be added or removed by core Cluster API controllers.<br />There can be at most 64 taints.<br />A pod would have to tolerate all existing taints to run on the corresponding node.<br />NOTE: This list is implemented as a "map" type, meaning that individual elements can be managed by different owners. |  | MaxItems: 64 <br />MinItems: 1 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneTemplateResource



KubeadmControlPlaneTemplateResource describes the data needed to create a KubeadmControlPlane from a template.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneTemplateSpec](#kubeadmcontrolplanetemplatespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `metadata` _[ObjectMeta](#objectmeta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)_ | spec is the desired state of KubeadmControlPlaneTemplateResource. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneTemplateResourceSpec



KubeadmControlPlaneTemplateResourceSpec defines the desired state of KubeadmControlPlane.
NOTE: KubeadmControlPlaneTemplateResourceSpec is similar to KubeadmControlPlaneSpec but
omits Replicas and Version fields. These fields do not make sense on the KubeadmControlPlaneTemplate,
because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
be configured on the KubeadmControlPlaneTemplate.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneTemplateResource](#kubeadmcontrolplanetemplateresource)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `machineTemplate` _[KubeadmControlPlaneTemplateMachineTemplate](#kubeadmcontrolplanetemplatemachinetemplate)_ | machineTemplate contains information about how machines<br />should be shaped when creating or updating a control plane. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `kubeadmConfigSpec` _[KubeadmConfigSpec](#kubeadmconfigspec)_ | kubeadmConfigSpec is a KubeadmConfigSpec<br />to use for initializing and joining machines to the control plane. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `rollout` _[KubeadmControlPlaneRolloutSpec](#kubeadmcontrolplanerolloutspec)_ | rollout allows you to configure the behaviour of rolling updates to the control plane Machines.<br />It allows you to require that all Machines are replaced before or after a certain time,<br />and allows you to define the strategy used during rolling replacements. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `remediation` _[KubeadmControlPlaneRemediationSpec](#kubeadmcontrolplaneremediationspec)_ | remediation controls how unhealthy Machines are remediated. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `machineNaming` _[MachineNamingSpec](#machinenamingspec)_ | machineNaming allows changing the naming pattern used when creating Machines.<br />InfraMachines & KubeadmConfigs will use the same name as the corresponding Machines. |  | MinProperties: 1 <br />Optional: \{\} <br /> |


#### KubeadmControlPlaneTemplateSpec



KubeadmControlPlaneTemplateSpec defines the desired state of KubeadmControlPlaneTemplate.



_Appears in:_
- [KubeadmControlPlaneTemplate](#kubeadmcontrolplanetemplate)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _[KubeadmControlPlaneTemplateResource](#kubeadmcontrolplanetemplateresource)_ | template defines the desired state of KubeadmControlPlaneTemplate. |  | MinProperties: 1 <br />Required: \{\} <br /> |


#### MachineNamingSpec



MachineNamingSpec allows changing the naming pattern used when creating Machines.
InfraMachines & KubeadmConfigs will use the same name as the corresponding Machines.

_Validation:_
- MinProperties: 1

_Appears in:_
- [KubeadmControlPlaneSpec](#kubeadmcontrolplanespec)
- [KubeadmControlPlaneTemplateResourceSpec](#kubeadmcontrolplanetemplateresourcespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `template` _string_ | template defines the template to use for generating the names of the Machine objects.<br />If not defined, it will fallback to `\{\{ .kubeadmControlPlane.name \}\}-\{\{ .random \}\}`.<br />If the generated name string exceeds 63 characters, it will be trimmed to 58 characters and will<br />get concatenated with a random suffix of length 5.<br />Length of the template string must not exceed 256 characters.<br />The template allows the following variables `.cluster.name`, `.kubeadmControlPlane.name` and `.random`.<br />The variable `.cluster.name` retrieves the name of the cluster object that owns the Machines being created.<br />The variable `.kubeadmControlPlane.name` retrieves the name of the KubeadmControlPlane object that owns the Machines being created.<br />The variable `.random` is substituted with random alphanumeric string, without vowels, of length 5. This variable is required<br />part of the template. If not provided, validation will fail. |  | MaxLength: 256 <br />MinLength: 1 <br />Optional: \{\} <br /> |



## ipam.cluster.x-k8s.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the exp v1alpha1 IPAM API.

Deprecated: This package is deprecated and is going to be removed when support for v1beta1 will be dropped.

### Resource Types
- [IPAddress](#ipaddress)
- [IPAddressClaim](#ipaddressclaim)



#### IPAddress



IPAddress is the Schema for the ipaddress API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `ipam.cluster.x-k8s.io/v1alpha1` | | |
| `kind` _string_ | `IPAddress` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[IPAddressSpec](#ipaddressspec)_ | spec is the desired state of IPAddress. |  | Optional: \{\} <br /> |


#### IPAddressClaim



IPAddressClaim is the Schema for the ipaddressclaim API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `ipam.cluster.x-k8s.io/v1alpha1` | | |
| `kind` _string_ | `IPAddressClaim` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[IPAddressClaimSpec](#ipaddressclaimspec)_ | spec is the desired state of IPAddressClaim. |  | Optional: \{\} <br /> |


#### IPAddressClaimSpec



IPAddressClaimSpec is the desired state of an IPAddressClaim.



_Appears in:_
- [IPAddressClaim](#ipaddressclaim)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `poolRef` _[TypedLocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#typedlocalobjectreference-v1-core)_ | poolRef is a reference to the pool from which an IP address should be created. |  | Required: \{\} <br /> |


#### IPAddressSpec



IPAddressSpec is the desired state of an IPAddress.



_Appears in:_
- [IPAddress](#ipaddress)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `claimRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | claimRef is a reference to the claim this IPAddress was created for. |  | Required: \{\} <br /> |
| `poolRef` _[TypedLocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#typedlocalobjectreference-v1-core)_ | poolRef is a reference to the pool that this IPAddress was created from. |  | Required: \{\} <br /> |
| `address` _string_ | address is the IP address. |  | MaxLength: 39 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `prefix` _integer_ | prefix is the prefix of the address. |  | Required: \{\} <br /> |
| `gateway` _string_ | gateway is the network gateway of the network the address is from. |  | MaxLength: 39 <br />MinLength: 1 <br />Optional: \{\} <br /> |



## ipam.cluster.x-k8s.io/v1beta1

Package v1beta1 contains API Schema definitions for the v1beta1 IPAM API.

Deprecated: This package is deprecated and is going to be removed when support for v1beta1 will be dropped.

### Resource Types
- [IPAddress](#ipaddress)
- [IPAddressClaim](#ipaddressclaim)



#### IPAddress



IPAddress is the Schema for the ipaddress API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `ipam.cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `IPAddress` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[IPAddressSpec](#ipaddressspec)_ | spec is the desired state of IPAddress. |  | Optional: \{\} <br /> |


#### IPAddressClaim



IPAddressClaim is the Schema for the ipaddressclaim API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `ipam.cluster.x-k8s.io/v1beta1` | | |
| `kind` _string_ | `IPAddressClaim` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[IPAddressClaimSpec](#ipaddressclaimspec)_ | spec is the desired state of IPAddressClaim. |  | Optional: \{\} <br /> |


#### IPAddressClaimSpec



IPAddressClaimSpec is the desired state of an IPAddressClaim.



_Appears in:_
- [IPAddressClaim](#ipaddressclaim)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `poolRef` _[TypedLocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#typedlocalobjectreference-v1-core)_ | poolRef is a reference to the pool from which an IP address should be created. |  | Required: \{\} <br /> |


#### IPAddressSpec



IPAddressSpec is the desired state of an IPAddress.



_Appears in:_
- [IPAddress](#ipaddress)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `claimRef` _[LocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#localobjectreference-v1-core)_ | claimRef is a reference to the claim this IPAddress was created for. |  | Required: \{\} <br /> |
| `poolRef` _[TypedLocalObjectReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#typedlocalobjectreference-v1-core)_ | poolRef is a reference to the pool that this IPAddress was created from. |  | Required: \{\} <br /> |
| `address` _string_ | address is the IP address. |  | MaxLength: 39 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `prefix` _integer_ | prefix is the prefix of the address. |  | Required: \{\} <br /> |
| `gateway` _string_ | gateway is the network gateway of the network the address is from. |  | MaxLength: 39 <br />MinLength: 1 <br />Optional: \{\} <br /> |



## ipam.cluster.x-k8s.io/v1beta2

Package v1beta2 contains API Schema definitions for the v1beta2 IPAM API.

### Resource Types
- [IPAddress](#ipaddress)
- [IPAddressClaim](#ipaddressclaim)



#### IPAddress



IPAddress is the Schema for the ipaddress API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `ipam.cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `IPAddress` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[IPAddressSpec](#ipaddressspec)_ | spec is the desired state of IPAddress. |  | Required: \{\} <br /> |


#### IPAddressClaim



IPAddressClaim is the Schema for the ipaddressclaim API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `ipam.cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `IPAddressClaim` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[IPAddressClaimSpec](#ipaddressclaimspec)_ | spec is the desired state of IPAddressClaim. |  | Required: \{\} <br /> |


#### IPAddressClaimReference



IPAddressClaimReference is a reference to an IPAddressClaim.



_Appears in:_
- [IPAddressSpec](#ipaddressspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the IPAddressClaim.<br />name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` <br />Required: \{\} <br /> |


#### IPAddressClaimSpec



IPAddressClaimSpec is the desired state of an IPAddressClaim.



_Appears in:_
- [IPAddressClaim](#ipaddressclaim)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clusterName` _string_ | clusterName is the name of the Cluster this object belongs to. |  | MaxLength: 63 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `poolRef` _[IPPoolReference](#ippoolreference)_ | poolRef is a reference to the pool from which an IP address should be created. |  | Required: \{\} <br /> |




#### IPAddressSpec



IPAddressSpec is the desired state of an IPAddress.



_Appears in:_
- [IPAddress](#ipaddress)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `claimRef` _[IPAddressClaimReference](#ipaddressclaimreference)_ | claimRef is a reference to the claim this IPAddress was created for. |  | Required: \{\} <br /> |
| `poolRef` _[IPPoolReference](#ippoolreference)_ | poolRef is a reference to the pool that this IPAddress was created from. |  | Required: \{\} <br /> |
| `address` _string_ | address is the IP address. |  | MaxLength: 39 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `prefix` _integer_ | prefix is the prefix of the address. |  | Maximum: 128 <br />Minimum: 0 <br />Required: \{\} <br /> |
| `gateway` _string_ | gateway is the network gateway of the network the address is from. |  | MaxLength: 39 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### IPPoolReference



IPPoolReference is a reference to an IPPool.



_Appears in:_
- [IPAddressClaimSpec](#ipaddressclaimspec)
- [IPAddressSpec](#ipaddressspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | name of the IPPool.<br />name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` <br />Required: \{\} <br /> |
| `kind` _string_ | kind of the IPPool.<br />kind must consist of alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character. |  | MaxLength: 63 <br />MinLength: 1 <br />Pattern: `^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$` <br />Required: \{\} <br /> |
| `apiGroup` _string_ | apiGroup of the IPPool.<br />apiGroup must be fully qualified domain name. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` <br />Required: \{\} <br /> |



## runtime.cluster.x-k8s.io/v1alpha1

Package v1alpha1 contains the v1alpha1 implementation of ExtensionConfig.

Deprecated: This package is deprecated and is going to be removed when support for v1beta1 will be dropped.

### Resource Types
- [ExtensionConfig](#extensionconfig)



#### ClientConfig



ClientConfig contains the information to make a client
connection with an Extension server.



_Appears in:_
- [ExtensionConfigSpec](#extensionconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | url gives the location of the Extension server, in standard URL form<br />(`scheme://host:port/path`).<br />Note: Exactly one of `url` or `service` must be specified.<br />The scheme must be "https".<br />The `host` should not refer to a service running in the cluster; use<br />the `service` field instead.<br />A path is optional, and if present may be any string permissible in<br />a URL. If a path is set it will be used as prefix to the hook-specific path.<br />Attempting to use a user or basic auth e.g. "user:password@" is not<br />allowed. Fragments ("#...") and query parameters ("?...") are not<br />allowed either. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `service` _[ServiceReference](#servicereference)_ | service is a reference to the Kubernetes service for the Extension server.<br />Note: Exactly one of `url` or `service` must be specified.<br />If the Extension server is running within a cluster, then you should use `service`. |  | Optional: \{\} <br /> |
| `caBundle` _integer array_ | caBundle is a PEM encoded CA bundle which will be used to validate the Extension server's server certificate. |  | MaxLength: 51200 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### ExtensionConfig



ExtensionConfig is the Schema for the ExtensionConfig API.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `runtime.cluster.x-k8s.io/v1alpha1` | | |
| `kind` _string_ | `ExtensionConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[ExtensionConfigSpec](#extensionconfigspec)_ | spec is the desired state of the ExtensionConfig. |  | Optional: \{\} <br /> |


#### ExtensionConfigSpec



ExtensionConfigSpec defines the desired state of ExtensionConfig.



_Appears in:_
- [ExtensionConfig](#extensionconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clientConfig` _[ClientConfig](#clientconfig)_ | clientConfig defines how to communicate with the Extension server. |  | Required: \{\} <br /> |
| `namespaceSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | namespaceSelector decides whether to call the hook for an object based<br />on whether the namespace for that object matches the selector.<br />Defaults to the empty LabelSelector, which matches all objects. |  | Optional: \{\} <br /> |
| `settings` _object (keys:string, values:string)_ | settings defines key value pairs to be passed to all calls<br />to all supported RuntimeExtensions.<br />Note: Settings can be overridden on the ClusterClass. |  | Optional: \{\} <br /> |




#### FailurePolicy

_Underlying type:_ _string_

FailurePolicy specifies how unrecognized errors when calling the ExtensionHandler are handled.
FailurePolicy helps with extensions not working consistently, e.g. due to an intermittent network issue.
The following type of errors are never ignored by FailurePolicy Ignore:
- Misconfigurations (e.g. incompatible types)
- Extension explicitly returns a Status Failure.



_Appears in:_
- [ExtensionHandler](#extensionhandler)

| Field | Description |
| --- | --- |
| `Ignore` | FailurePolicyIgnore means that an error when calling the extension is ignored.<br /> |
| `Fail` | FailurePolicyFail means that an error when calling the extension is propagated as an error.<br /> |


#### GroupVersionHook



GroupVersionHook defines the runtime hook when the ExtensionHandler is called.



_Appears in:_
- [ExtensionHandler](#extensionhandler)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | apiVersion is the group and version of the Hook. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `hook` _string_ | hook is the name of the hook. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |


#### ServiceReference



ServiceReference holds a reference to a Kubernetes Service of an Extension server.



_Appears in:_
- [ClientConfig](#clientconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `namespace` _string_ | namespace is the namespace of the service. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `name` _string_ | name is the name of the service. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `path` _string_ | path is an optional URL path and if present may be any string permissible in<br />a URL. If a path is set it will be used as prefix to the hook-specific path. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the port on the service that's hosting the Extension server.<br />Defaults to 443.<br />Port should be a valid port number (1-65535, inclusive). |  | Optional: \{\} <br /> |



## runtime.cluster.x-k8s.io/v1beta2

Package v1beta2 contains the v1beta2 implementation of ExtensionConfig.

### Resource Types
- [ExtensionConfig](#extensionconfig)



#### ClientConfig



ClientConfig contains the information to make a client
connection with an Extension server.

_Validation:_
- MinProperties: 1

_Appears in:_
- [ExtensionConfigSpec](#extensionconfigspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | url gives the location of the Extension server, in standard URL form<br />(`scheme://host:port/path`).<br />Note: Exactly one of `url` or `service` must be specified.<br />The scheme must be "https".<br />The `host` should not refer to a service running in the cluster; use<br />the `service` field instead.<br />A path is optional, and if present may be any string permissible in<br />a URL. If a path is set it will be used as prefix to the hook-specific path.<br />Attempting to use a user or basic auth e.g. "user:password@" is not<br />allowed. Fragments ("#...") and query parameters ("?...") are not<br />allowed either. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `service` _[ServiceReference](#servicereference)_ | service is a reference to the Kubernetes service for the Extension server.<br />Note: Exactly one of `url` or `service` must be specified.<br />If the Extension server is running within a cluster, then you should use `service`. |  | Optional: \{\} <br /> |
| `caBundle` _integer array_ | caBundle is a PEM encoded CA bundle which will be used to validate the Extension server's server certificate. |  | MaxLength: 51200 <br />MinLength: 1 <br />Optional: \{\} <br /> |


#### ExtensionConfig



ExtensionConfig is the Schema for the ExtensionConfig API.
NOTE: This CRD can only be used if the RuntimeSDK feature gate is enabled.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `runtime.cluster.x-k8s.io/v1beta2` | | |
| `kind` _string_ | `ExtensionConfig` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | MinProperties: 1 <br />Optional: \{\} <br /> |
| `spec` _[ExtensionConfigSpec](#extensionconfigspec)_ | spec is the desired state of the ExtensionConfig. |  | Required: \{\} <br /> |


#### ExtensionConfigSpec



ExtensionConfigSpec defines the desired state of ExtensionConfig.



_Appears in:_
- [ExtensionConfig](#extensionconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `clientConfig` _[ClientConfig](#clientconfig)_ | clientConfig defines how to communicate with the Extension server. |  | MinProperties: 1 <br />Required: \{\} <br /> |
| `namespaceSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#labelselector-v1-meta)_ | namespaceSelector decides whether to call the hook for an object based<br />on whether the namespace for that object matches the selector.<br />Defaults to the empty LabelSelector, which matches all objects. |  | Optional: \{\} <br /> |
| `settings` _object (keys:string, values:string)_ | settings defines key value pairs to be passed to all calls<br />to all supported RuntimeExtensions.<br />Note: Settings can be overridden on the ClusterClass. |  | Optional: \{\} <br /> |




#### FailurePolicy

_Underlying type:_ _string_

FailurePolicy specifies how unrecognized errors when calling the ExtensionHandler are handled.
FailurePolicy helps with extensions not working consistently, e.g. due to an intermittent network issue.
The following type of errors are never ignored by FailurePolicy Ignore:
- Misconfigurations (e.g. incompatible types)
- Extension explicitly returns a Status Failure.

_Validation:_
- Enum: [Ignore Fail]

_Appears in:_
- [ExtensionHandler](#extensionhandler)

| Field | Description |
| --- | --- |
| `Ignore` | FailurePolicyIgnore means that an error when calling the extension is ignored.<br /> |
| `Fail` | FailurePolicyFail means that an error when calling the extension is propagated as an error.<br /> |


#### GroupVersionHook



GroupVersionHook defines the runtime hook when the ExtensionHandler is called.



_Appears in:_
- [ExtensionHandler](#extensionhandler)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | apiVersion is the group and version of the Hook. |  | MaxLength: 512 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `hook` _string_ | hook is the name of the hook. |  | MaxLength: 256 <br />MinLength: 1 <br />Required: \{\} <br /> |


#### ServiceReference



ServiceReference holds a reference to a Kubernetes Service of an Extension server.



_Appears in:_
- [ClientConfig](#clientconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `namespace` _string_ | namespace is the namespace of the service. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `name` _string_ | name is the name of the service. |  | MaxLength: 63 <br />MinLength: 1 <br />Required: \{\} <br /> |
| `path` _string_ | path is an optional URL path and if present may be any string permissible in<br />a URL. If a path is set it will be used as prefix to the hook-specific path. |  | MaxLength: 512 <br />MinLength: 1 <br />Optional: \{\} <br /> |
| `port` _integer_ | port is the port on the service that's hosting the Extension server.<br />Defaults to 443.<br />Port should be a valid port number (1-65535, inclusive). |  | Optional: \{\} <br /> |


