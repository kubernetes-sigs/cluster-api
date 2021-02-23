---
title: Cluster API OS Independent Machine Bootstrapper
authors:
  - "@randomvariable"
  - "@yastij"
reviewers:
  - "@fpandini"
creation-date: 2021-02-22
last-updated: 2021-02-22
status: provisional
see-also:
  - "/docs/proposals/2021022-kubelet-authentication-plugin.md"
  - "/docs/proposals/2021022-artifacts-management.md"
---

# Cluster API Machine Bootstrapper

## Table of Contents

- [Cluster API Machine Bootstrapper](#cluster-api-machine-bootstrapper)
  - [Table of Contents](#table-of-contents)
  - [Glossary](#glossary)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
    - [Requirements Specification](#requirements-specification)
    - [Implementation details](#implementation-details)
      - [Deployment Model](#deployment-model)
        - [Core plugins](#core-plugins)
    - [Phased development](#phased-development)
    - [Plugin architecture](#plugin-architecture)
    - [Security Model](#security-model)
      - [Userdata storage](#userdata-storage)
    - [Types](#types)
      - [Additions to core Bootstrap type used in Machine* types](#additions-to-core-bootstrap-type-used-in-machine-types)
      - [Bootstrap template ConfigMap/Secret format:](#bootstrap-template-configmapsecret-format)
        - [Variables](#variables)
        - [Custom supported functions](#custom-supported-functions)
      - [MachineConfig types](#machineconfig-types)
        - [Common types](#common-types)
      - [Serialized data format](#serialized-data-format)
        - [Remote configuration](#remote-configuration)
        - [Encrypted configuration](#encrypted-configuration)
        - [Example of combining](#example-of-combining)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Alternatives](#alternatives)
    - [Single MachineConfiguration type](#single-machineconfiguration-type)
    - [MachineConfiguration as a separate non-bootstrap resource](#machineconfiguration-as-a-separate-non-bootstrap-resource)
  - [Upgrade Strategy](#upgrade-strategy)
  - [Additional Details](#additional-details)
    - [Test Plan [optional]](#test-plan-optional)
    - [Graduation Criteria [optional]](#graduation-criteria-optional)
    - [Version Skew Strategy [optional]](#version-skew-strategy-optional)
  - [Implementation History](#implementation-history)

## Glossary

* OS Distribution: An OS Distribution refers to a packaged version of an Operating System, in order to primarily distinguish between different Linux distributions such as CentOS vs. Ubuntu vs. Talos, as opposed to differences between Operating Systems as a whole (e.g. Linux and Windows).
* Machineadm : Is a binary CLI that is executed on machines to perform Kubernetes Cluster API bootstrap.
* CABPK: Cluster API Bootstrap Provider Kubeadm is the bootstrap controller that exists from v1alpha2 onwards and generates cloud-init bootstrap data to execute kubeadm for a machine provisioned by Cluster API
* Cloud-Init: Is a first-boot bootstrapper written by Canonical and is widely used across Ubuntu, Amazon Linux 2, and VMware PhotonOS
* Cloudbase-Init: A Cloud-Init API compatible bootstrapper for the Windows operating system.
* First-boot bootstrapper: Run when a machine is booted for the first time, and retrieve configuration information from an infrastructure provider specified endpoint, e.g. IMDS or Guestinfo.


## Summary

Cluster API through v1alpha3 has used a combination of kubeadm, shell scripts and cloud-init to provision nodes. This proposal is for a node bootstrapper to combine these functions into a component that configures a machine and runs kubeadm, and be able to be consumed by multiple bootstrap providers such as cloud-init, Ignition and Talos.

## Motivation

Cluster API’s reliance on cloud-init has frequently caused problems: changes in patch releases have caused breaking changes for Cluster API providers, such as vSphere and AWS. It has also made it difficult for other vendors, not using cloud-init, to easily use the core
Cluster API providers, examples include OpenShift and FlatCar Linux which both use Ignition, and Talos with their own system.

Furthermore, certain providers, such as Cluster API Provider AWS are utilising time-limited hacks within cloud-init to secure bootstrap userdata, and this is not sustainable for the health of the project over time.

Use of an agnostic bootstrapper (machineadm) benefits end users in that they won’t need to closely monitor changes within each system that may have negative side effects on Cluster API. In addition, separating out the processes required to bootstrap a Kubernetes node from the bootstrap mechanism allows for Cluster API Kubeadm Bootstrap Provider (CABPK) to become an independent component.

### Goals

* To produce a minimal on-machine bootstrapping mechanism to run kubeadm, and configure cluster related components.
* To produce interfaces to plug new bootstrapping mechanisms.
* To define new super-types (bootstrapper agnostic) for cluster and control plane configuration, which are not directly coupled to the kubeadm types.
* To work closely with other efforts within Cluster API for the introduction of the types, like the one outlined in #2769.
* To have Kubeadm Control Plane Provider and Kubeadm Bootstrap Provider adopt these types in v1alpha4 (release blocking).


### Non-Goals/Future Work

- To support any on the fly mutability of components or resources at the present time. This proposal will be amended in the future to cover mutability use cases after the initial implementations are completed.

## Proposal

### User Stories
<table>
<thead>
<tr>
<th>ID</th><th>Title</th><th>Description</th></tr>
</thead>
<tbody>
<tr>
<td>U1</td><td>Non cloud-init bootstrap processes</td>
<td>
Ignition is a user-data processing Linux bootstrapping system used by Flat Car Linux, RHEL Atomic Host and Fedora CoreOS. (cluster-api/3761)
</td>
</tr>

<tr>
<td>U2</td><td>System preparation</td>
<td>
Although Flatcar Container Linux is being added to Image Builder, Flatcar is intended to also be used as an immutable distribution, with all additions being done at first boot. Flatcar users should be able to use standard Flatcar images with Cluster API.
</td>
</tr>

<tr>
<td>U3</td><td>Active Directory</td>
<td>
As a platform operator of a Windows environment, I may require their Kubernetes nodes to be domain joined such that the application workloads operate with appropriate Kerberos credentials to connect to services in the infrastructure.

For Windows or Linux hosts joining an Active Directory, they must effectively be given a set of bootstrap credentials to join the directory and persist a Kerberos keytab for the host.
</td>
</tr>

<tr>
<td>U4</td><td>CIS Benchmark Compliance</td>
<td>
As a platform operator, I require Kubernetes clusters to pass the CIS Benchmark in order to meet organisational level security compliance requirements.
</td>
</tr>

<tr>
<td>U5</td><td>DISA STIG Compliance</td>
<td>
As a platform operator in a US, UK, Canadian, Australian or New Zealand secure government environment, I require my Kubernetes clusters to be compliant with the DISA STIG.
</td>
</tr>

<tr>
<td>U6</td><td>Kubeadm UX</td>
<td>
As a cluster operator, I would like the bootstrap configuration of clusters or machines to be shielded from changes happening in kubeadm (e.g. v1beta1 and v1beta2 type migration)</td>
</tr>

<tr>
<td>U7</td><td>Existing Clusters</td>
<td>
As a cluster operator with existing clusters, I would like to be able to, after enabling the necessary flags or feature gates, to create new clusters or machines using nodeadm.
</td>
</tr>

<tr>
<td>U8</td><td>Air-gapped</td>
<td>
As a cluster operator, I need Cluster API to operate independently of an internet connection in order to be able to provision clusters in an air-gapped environment, i.e. where the data center is not connected to the public internet.
</td>
</tr>

<tr>
<td>U9</td><td>Advanced control plane configuration files
</td>
<td>
As a cluster operator, I need to configure components of my control plane, such as audit logging policies, KMS encryption, authentication webhooks to meet organisational requirements.
</td>
</tr>

<tr>
<td>U10</td><td>ContainerD Configuration</td>
<td>
Options such as proxy configuration, registry mirrors, custom certs, cgroup hierachy (image-builder/471) need to often be customised, and it isn’t always suitable to do at an image level. Cluster operators in an organisation often resort to prekubeadmcommand bash scripts to configure containerd and restart the service.
</td>
</tr>

<tr>
<td>U11</td><td>API Server Auth Reconfiguration</td>
<td>
As a cluster operator, I need to reconfigure the API server such that I can deploy a new static pod for authentication and insert an updated API server configuration.
</td>
</tr>

<tr>
<td>U12</td><td>Improving bootstrap reporting</td>
<td>
SRE teams often need to diagnose failed nodes, and having better information about why a node may have failed to join, or better indication of success would be helpful. (cluster-api/3716)
</td>
</tr>

</tbody>
</table>


### Requirements Specification
We define three modalities of the node bootstrapper:

<table>
<thead>
<tr><th>Mode</th><th>Description</th>
</thead>
<tbody>

<tr>
<td>Provisioning</td>
<td>
Expected to run as part of machine bootstrapping e.g. (part of cloud-* SystemD units or Windows OOBE). Only supported when used with Cluster API bootstrapping. Typically executes cluster creation or node join procedures, configuring kubelet etc...
</td>
</tr>

<tr>
<td>Preparation</td>
<td>
Could be run as part of machine bootstrapping prior to “provisioning”, and “prepares” a machine for use with Kubernetes. We largely keep this out of scope for the initial implementation unless there is a trivial implementation.
</td>
</tr>

<tr>
<td>Post</td>
<td>
Parts of the use cases above require ongoing management of a host. We list these as requirements, but are largely not in scope for the machine bootstrapper and should be dealt with by external systems.
</td>
</tr>
</tbody>
</table>

<table>
<thead>
<tr><th>ID</th><th>Requirement</th><th>Mode</th><th>Related Stories</th>
</thead>
<tbody>

<tr>
<td>R1</td>
<td>
The machine bootstrapper MUST be able to execute kubeadm and report its outcome.
Provisioning
</td>
<td>Provisioning</td><td>U1</td>
</tr>

<tr>
<td>R2</td>
<td>
The machine bootstrapper MUST allow the configuration of Linux sysctl parameters
</td>
<td>Preparation</td><td>U2,U4</td>
</tr>

<tr>
<td>R3</td>
<td>
The machine bootstrapper COULD allow the application of custom static pods on the control plane
</td>
<td>Provisioning</td><td>U4,U9</td>
</tr>

<tr>
<td>R4</td>
<td>
The machine bootstrapper MUST not directly expose the kubeadm API to the end user
</td>
<td>Provisioning</td><td>U6</td>
</tr>

<tr>
<td>R5</td>
<td>
The machine bootstrapper MUST be able to be used in conjunction with an OS provided bootstrapping tool, not limited to Cloud-Init, Ignition, Talos and Windows Answer File.
</td>
<td>Provisioning</td><td>U1</td>
</tr>

<tr>
<td>R6</td>
<td>
The machine bootstrapper/authenticator binary MUST provide cryptographic verification in situations where it is downloaded post-boot.
</td>
<td>Preparation</td><td>U2</td>
</tr>

<tr>
<td>R7</td>
<td>
The machine bootstrapper MUST not be reliant on the use of static pods to operate</td>
<td>All</td><td>U5</td>
</tr>

<tr>
<td>R8</td>
<td>
The machine bootstrapper MUST enable a Windows node to be domain joined. The machine bootstrapper WILL NOT manage the group membership of a Windows node in order to enable Group Managed Service Accounts
</td>
<td>Provisioning</td><td>U3</td>
</tr>

<tr>
<td>R9</td>
<td>
The node bootstrapping system MUST be opt-in and not affect the operation of existing clusters when Cluster API is upgraded.
</td>
<td>Provisioning</td><td>U7</td>
</tr>

<tr>
<td>R10</td>
<td>
The machine bootstrapper system SHOULD allow the agent to be downloaded from the management cluster
</td>
<td>Preparation</td><td>U8</td>
</tr>

<tr>
<td>R11</td>
<td>
The machine bootstrapper MUST be able to operate without connectivity to the internet (using proper configuration parameters), or to the management cluster.
</td>
<td>Provisioning</td><td>U7</td>
</tr>

<tr>
<td>R12</td>
<td>
When the machine bootstrapper is downloaded on boot the location MUST be configurable</td>
<td>Preparation</td><td>U8</td>
</tr>

<tr>
<td>R13</td>
<td>
When the machine bootstrapper is downloaded from the public internet, it MUST be downloadable from a location not subject to frequent rate limiting (e.g. a GCS bucket).</td>
<td>Preparation</td><td>U9</td>
</tr>

<tr>
<td>R14</td>
<td>
The machine bootstrapper MUST be able to configure containerd given a structured configuration input..</td>
<td>Provisioning</td><td>U10</td>
</tr>

<tr>
<td>R15</td>
<td>
The machine bootstrapper MUST publish a documented contract for operating system maintainers to integrate with the machine bootstrapper.
</td>
<td>All</td><td>U1</td>
</tr>


</tbody>


</table>




### Implementation details

#### Deployment Model

<table>
<thead>
<tr><th>Component</th><th>Location</th><th>Description</th>
</thead>
<tbody>

<tr>
<td>machineadm-bootstrap-controller</td><td>cluster-api repo</td>
<td>
The core machine bootstrapper contains the controllerst hat reconciles MachineConfig to produce machineadm yaml
</td>

<tr>
<td>machineadm-bootstrap-controller</td><td>cluster-api repo</td>
<td>
The core machine bootstrapper contains the controllerst hat reconciles MachineConfig to produce machineadm yaml
</td>
</tr>

<tr>
<td>machineadm cli</td><td>cluster-api repo</td>
<td>
The core CLI that reads machineadm.yaml and orchestrates the bootstrapping workflow
</td>
</tr>

<tr>
<td>machineadm core plugins</td><td>cluster-api repo</td>
<td>
A controller that ships with the infrastructure provider that can reconcile the storage of machineadm configurations with infrastructure APIs (e.g. Amazon S3/Minio, GCS, custom server etc...).
</td>
</tr>

<tr>
<td>machineadm remote configuration plugin</td><td>infrastructure provider repo</td>
<td>
An infrastructure plugin for the node bootstrapper will be created within infrastructure providers that would be responsible for infrastructure-specific logic to pull files machineadm files from a given location and upload
bootstrap data.
</td>
</tr>

<tr>
<td>machineadm encryption plugin</td><td>infrastructure provider repo</td>
<td>
An infrastructure plugin for the node bootstrapper will be created within infrastructure providers that would be responsible for infrastructure-specific logic to use an external key management service.
</td>
</tr>

<tr>
<td>machineadm infrastructure controller</td><td>infrastructure provider repo</td>
<td>
A controller that ships with the infrastructure provider that can reconcile the storage of machineadm configurations with infrastructure APIs (e.g. Amazon S3/Minio, GCS, custom server etc...).
</td>
</tr>

</tbody>
</table>

##### Core plugins
<table>
<thead>
<tr><th>Component</th><th>Description</th>
</thead>
<tbody>

<tr>
<td>machineadm-container-runtime-plugin-containerd</td>
<td>
A plugin that configures containerd runtimes.
</td>

<tr>
<td>machineadm-timesync-plugin-systemd</td>
<td>
A plugin that configures systemd-timesyncd.
</td>

<tr>
<td>machineadm-timesync-plugin-ntpd</td>
<td>
A plugin that configures ntpd.
</td>

<tr>
<td>machineadm-timesync-plugin-chronyd</td>
<td>
A plugin that configures chronyd.
</td>

</tbody>
</table>

### Phased development

With reference to the modalities described in the requirements specification, we propose 3 phases of development:

| Phase             | What gets implemented           |
| ----------------- | ------------------------------- |
| Phase 1 - Alpha 1 | Provisioning Modality           |
| Phase 2 - Alpha 2 | Preparation and Post Modalities |
| Phase 3 & GA      | API Stabilisation               |

### Plugin architecture

Machineadm will use the [go-plugin][go-plugin] architecture used by Hashicorp. Machineadm will expect plugins to
use consistent filename formats depending on what type of plugin it is (and will be documented in the type).

Plugins can either be "on path", in a directory supported in the PATH variable, or:
* Linux: stored in `/usr/libexec/machineadm` or `/usr/local/libexec/machineadm`
* Windows: stored in `"C:\Program Files\machineadm\plugins"`

### Security Model

#### Userdata storage

Machineadm's security model is intended such that secrets associated with machine configuration are stored on the
management Kubernetes cluster and are secured to the level that the backing API server and etcd are secured.

For machine bootstrapping, secrets are intended to be delivered by an infrastructure provider to a suitable secure
location, and to also provide a mechanism for the machine to report back status. Examples include:

* Amazon S3 or S3-compatible API such as Minio
* AWS Secrets Manager and AWS Systems Manager Parameter Store

When using external storage, we assume security is configured appropriately (e.g. Amazon Identity and Access Management),
and is outside of the system boundary of machineadm bootstrap controller.

In addition, some providers may require a secondary mechanism to encrypt data. For example, a deployment of an S3 compatible
API may not support the same levels of fine-grained IAM access supported by EC2. A provider may then want to further encrypt
the data. In this case  we will additionally support a data block for in-band binary data (see [#encrypted-configuration](#encrypted-configuration)) that can be decrypted with a key derivation algorithm and a passphrase from an external source. We do not assume a KMS can
directly encrypt the entire machine configuration as most systems have byte limits of around 64KB. Instead we assume
the use of envelope encryption where a KMS encrypts a random string that is itself used as a key for encryption/data
of data.

We furthermore protect the KMS issued random string by deriving a key from it using SHA-512 PBKDF2-HMAC hashed to 50,000 rounds, with the result used for AES-256-GCM. This provides FIPS compliance with a suitable level of encryption given the
resultant data will be applied as plain text to the machine configuration.

The passphrase may either be stored at a remote location only the machine can access, or itself may be encrypted by an external Key Management Service, in which case machineadm is using envelope encryption. For example. the AWS provider could call `kms::GenerateDataKey` that generates a random string and associated ciphertext for the string encrypted by the Customer Managed Key. The provider could then use the random string as the passphrase for key derivation and throw away the plaintext
passphrase, encoding the KMS encrypted value in a URI understood by the machineadm plugin, e.g. `kms://ZW5jcnlwdGVk@arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab`, where the base64 value before
@ represents the encrypted passphrase to be handed to kms::Decrypt.

In terms of why we provide support for both external data and in-band data with encryption is that some infrastructure
APIs have limited userdata storage: For example, AWS and Azure are both limited to 64-bytes for userdata. When users
want to configure things such as system certificates, these are generally incompressible and may exceed the capacity of
that infrastructure. In these scenarios, external data stores are required to allow high levels of customization.



### Types

#### Additions to core Bootstrap type used in Machine* types

```go
// Bootstrap capsulates fields to configure the Machine’s bootstrapping mechanism.
type Bootstrap struct {
	// NOTE: NO CHANGES TO EXISTING FIELDS

	// NEW FIELD:
	// InfrastructureRef is a reference to an infrastructure provide specific resource
	// that holds details of how to store and retrieve bootstrap data securely,
	// and how a bootstrapper can report status.
	InfrastructureRef *corev1.ObjectReference `json:"infrastructureRef,omitempty"`
}
```
#### Bootstrap template ConfigMap/Secret format:

Bootstrap templates will be simple go template strings that will form the final
user data block. For the purposes of machineadm, the OS bootstrapper must write
the file to disk somewhere, and execute machineadm with administrative credentials
against the provided file.

The following variables and functions are supported:

##### Variables

* `machine_config`: The contents of the final infrastructure customized
  bootstrap configuration document.

##### Custom supported functions
* `base64`: Base64 encodes the input.
* `gzipBase64`: gzip compresses the input and then base64 encodes it.

An example using cloud-init is presented below:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cloud-init-machineadm-bootstrap-template
data:
  template: |
    #cloud-config
    write_files:
    - encoding: gz+b64
      content:  {{ machine_config | gzipBase64 }}.
      owner: root:root
      path: /tmp/machineadm_config.yaml
      permissions: '0644'
    runcmd:
      - /usr/bin/machineadm --bootstrap --path /tmp/machineadm_config.yaml
```
#### MachineConfig types

It is important to note that we have in effect two representations of the MachineConfig: First, we have the
MachineBootstrap Kubernetes resource that may reference other Kubernetes resources such as secrets for inclusion
in the machine configuration. Secondly, we have the "serialized" MachineConfiguration which is the "fully realized"
configuration, that inlines all of the referenced data and is stored as its own Kubernetes secret for handoff to
the infrastructure provider for secure storage.

##### Common types

```go
// MachineBootstrapConfig is a resource that defines the configuration
// of a machine
type MachineBootstrapConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineBootstrapConfigSpec   `json:"spec,omitempty"`
	Status MachineBootstrapConfigStatus `json:"status,omitempty"`
}

// MachineBootstrapConfigSpec defines the desired state of a MachineBootstrapConfig
type MachineBootstrapConfigSpec struct {
	PluginRefs []corev1.ObjectReference `json:"pluginRefs"`
}

// MachineBootstrapConfigTemplate is a resource that defines the configuration
// of a machine
type MachineBootstrapConfigTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MachineBootstrapConfigTemplateSpec `json:"spec,omitempty"`
}

// MachineBootstrapConfigTemplateSpec defines the desired state of a MachineBootstrapConfigTemplate
type MachineBootstrapConfigTemplateSpec struct {
	MachineBootstrapConfigSpec `json:",inline"`
	// PluginTemplateRefs is a list of machine bootstrap plugin template refs
	// which will be instantiated for each machine.
	PluginTemplateRefs []corev1.ObjectReference `json:"pluginTemplateRefs,omitempty"`
}

// MachineBootstrapConfigStatus defines the observed state of KubeadmConfig
type MachineBootstrapConfigStatus struct {
	// Ready indicates the BootstrapData field is ready to be consumed
	Ready bool `json:"ready,omitempty"`

	// DataSecretName is the name of the secret that stores the bootstrap data script.
	// +optional
	DataSecretName *string `json:"dataSecretName,omitempty"`

	// FailureReason will be set on non-retryable errors
	// +optional
	FailureReason string `json:"failureReason,omitempty"`

	// FailureMessage will be set on non-retryable errors
	// +optional
	FailureMessage string `json:"failureMessage,omitempty"`

	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions defines current service state of the KubeadmConfig.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// PluginStatus defines the status of a machineadm plugin
type PluginStatus struct {
	// FailureReason will be set on non-retryable errors
	// +optional
	FailureReason string `json:"failureReason,omitempty"`
	// FailureMessage will be set on non-retryable errors
	// +optional
	FailureMessage string `json:"failureMessage,omitempty"`
	// Conditions defines current service state of the cluster.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`
	// Ready defines if the control plane is ready.
	// +optional
	Ready         bool     `json:"ready,omitempty"`
	SecretName    string   `json:"secretName,omitempty"`
	BeforePlugins []string `json:"beforePlugins,omitempty"`
	AfterPlugins  []string `json:"afterPlugins,omitempty"`
}
```

#### Serialized data format

`machineadm_config.yaml` will be a multi-part YAML docment supporting multiple
data types, read and processed in order.

##### Remote configuration

```go
// RemoteConfig is a resource for fetching a remote machine configuration
type RemoteConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RemoteConfigSpec `json:"spec,omitempty"`
}

type RemoteConfigSpec struct {
	// Provider is the infrastructure provider plugin that fetches the remote config.
	// This must be present on the host as
	// machineadm-remote-configuration-provider-x
	// +kubebuilder:validation:Required
	Provider string `json:"provider"`
	// URI is the location of the provider-specific remote config for retrieval
	// s3://us-west-2.cluster-api.contoso.com/bootstrap/test-cluster-machine-ufjdhad.yaml
	// +kubebuilder:validation:Required
	URI string `json:"uri"`
}
```

##### Encrypted configuration

In-line serialized encrypted data for machineadm is a fully versioned type.
The reason for this is to allow changes due to security reviews, CVEs, support
for different types of encryption, such that we are not locked in to
a particular implementation, as is currently the case with Kubernetes API Server
KMS encryption providers.


```go
// EncryptedConfig is a resource representing in-line encrypted
// data that can be decrypted with an infrastructure plugin
type EncryptedConfig struct {
	metav1.TypeMeta   `json:",inline"`
	Spec EncryptedDataSpec `json:"spec,omitempty"`
}

// EncryptedConfigSpec stores an encrypted block of data as Ciphertext decryptable with a given key derivation algorithm
// and passphrase.
type EncryptedConfigSpec struct {
	// Provider is the infrastructure provider plugin that fetches the passphrase. This must be present on the host as
	// machineadm-plugin-encryption-provider-x
	// +kubebuilder:validation:Required
	Provider string `json:"provider"`
	// PassphraseURI is the location of the provider-specific passphrase
	// for retrieval, e.g. kms://ZW5jcnlwdGVk@arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab
	// +kubebuilder:validation:Required
	PassphraseURI string `json:"passphraseURI"`
	// Ciphertext is the base64 encoded encrypted ciphertext
	Ciphertext string `json:"ciphertext"`
	// +kubebuilder:validation:Required
	// Salt is base64 encoded random data added to the hashing algorithm to safeguard the
	// passphrase
	Salt string `json:"salt"`
	// IV, or Initialization Vector is the base64 encoded fixed-size input to a cryptographic algorithm
	// providing the random seed that was used for encryption.
	IV string `json:"iv"`
	// CipherAlgorithm is the encryption algorithm used for the ciphertext
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=aes-256-gcm
	CipherAlgorithm string `json:"cipherAlgorithm"`
	// DigestAlgorithm is the digest algorithm used for key derivation.
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=sha-512
	DigestAlgorithm string `json:"digestAlgorithm"`
	// Iterations is the number of hashing iterations that was used for key derivation.
	// +kubebuilder:default:=50000
	Iterations string `json:"iterations"`
	// KeyDerivationAlgorithm states the key derivation algorithm used to derive the encryption key
	// from the passphrase
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=pbkdf2
	KeyDerivationAlgorithm string `json:"keyDerivationAlgorithm"`
}
```

##### Example of combining

### Risks and Mitigations

- What are the risks of this proposal and how do we mitigate? Think broadly.
- How will UX be reviewed and by whom?
- How will security be reviewed and by whom?
- Consider including folks that also work outside the SIG or subproject.

## Alternatives

### Single MachineConfiguration type

We could alternatively have a single MachineConfiguration type that includes
all of the common use cases within it.  The problem with this is that it leads
to a large versioned API surface that would need to be supported for a long time.

Breaking out separate configuration components into smaller types allows for
extensibility, and faster revisioning of types where needed.

### MachineConfiguration as a separate non-bootstrap resource

Another option that was considered was to preserve KubeadmBootstrapConfig, and have the
MachineConfiguration resources wholly attached to the Machine object.

This would theoretically allow the continued usage of KubeadmControlPlane, but changes
the Machine contract, and would need to be considered in version upgrades for Machine resources.


## Upgrade Strategy
TODO

If applicable, how will the component be upgraded? Make sure this is in the test plan.

Consider the following in developing an upgrade strategy for this enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing cluster required to make on upgrade in order to make use of the enhancement?

## Additional Details

### Test Plan [optional]

TODO

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy.
Anything that would count as tricky in the implementation and anything particularly challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage expectations).
Please adhere to the [Kubernetes testing guidelines][testing-guidelines] when drafting this test plan.

[testing-guidelines]: https://git.k8s.io/community/contributors/devel/sig-testing/testing.md

### Graduation Criteria [optional]

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal should keep
this high-level with a focus on what signals will be looked at to determine graduation.

Consider the following in developing the graduation criteria for this enhancement:
- [Maturity levels (`alpha`, `beta`, `stable`)][maturity-levels]
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

### Version Skew Strategy [optional]

If applicable, how will the component handle version skew with other components? What are the guarantees? Make sure
this is in the test plan.

Consider the following in developing a version skew strategy for this enhancement:
- Does this enhancement involve coordinating behavior in the control plane and in the kubelet? How does an n-2 kubelet without this feature available behave when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI or CNI may require updating that component before the kubelet.

## Implementation History

- [ ] MM/DD/YYYY: Proposed idea in an issue or [community meeting]
- [ ] MM/DD/YYYY: Compile a Google Doc following the CAEP template (link here)
- [ ] MM/DD/YYYY: First round of feedback from community
- [ ] MM/DD/YYYY: Present proposal at a [community meeting]
- [ ] MM/DD/YYYY: Open proposal PR

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1Ys-DOR5UsgbMEeciuG0HOgDQc8kZsaWIWJeKJ1-UfbY
[go-plugin]: https://github.com/hashicorp/go-plugin
[kmip]: https://en.wikipedia.org/wiki/Key_Management_Interoperability_Protocol
