---
title: Publish Provider Manifests as OCI Artifacts
authors:
  - "@richardcase"
reviewers:
  - "@fabriziopandini"
  - "@sbueringer"
  - "@erikgb"
  - "@phoban01"
creation-date: 2026-08-04
last-updated: 2026-08-04
status: provisional
see-also:
  - "/docs/proposals/20191016-clusterctl-redesign.md"
  - "/docs/proposals/20201020-capi-provider-operator.md"
---

# Publish Provider Manifests as OCI Artifacts

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Glossary](#glossary)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals/Future Work](#non-goalsfuture-work)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
    - [Story 1: Air-gapped installation with the CAPI Operator](#story-1-air-gapped-installation-with-the-capi-operator)
    - [Story 2: GitOps-driven provider management](#story-2-gitops-driven-provider-management)
    - [Story 3: clusterctl init from a private registry mirror](#story-3-clusterctl-init-from-a-private-registry-mirror)
    - [Story 4: Provider maintainer adopting OCI distribution](#story-4-provider-maintainer-adopting-oci-distribution)
    - [Story 5: Reducing CI flakiness](#story-5-reducing-ci-flakiness)
  - [Requirements (Optional)](#requirements-optional)
    - [Functional Requirements](#functional-requirements)
      - [FR1](#fr1)
      - [FR2](#fr2)
      - [FR3](#fr3)
      - [FR4](#fr4)
      - [FR5](#fr5)
      - [FR6](#fr6)
      - [FR7](#fr7)
    - [Non-Functional Requirements](#non-functional-requirements)
      - [NFR1](#nfr1)
      - [NFR2](#nfr2)
      - [NFR3](#nfr3)
      - [NFR4](#nfr4)
  - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [OCI artifact format](#oci-artifact-format)
    - [Naming and tagging conventions](#naming-and-tagging-conventions)
    - [Templating and variables](#templating-and-variables)
    - [Publishing pipeline](#publishing-pipeline)
    - [clusterctl support](#clusterctl-support)
    - [CAPI Operator compatibility](#capi-operator-compatibility)
    - [Provider contract](#provider-contract)
  - [Security Model](#security-model)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
- [Upgrade Strategy](#upgrade-strategy)
- [Additional Details](#additional-details)
  - [Test Plan [optional]](#test-plan-optional)
  - [Graduation Criteria [optional]](#graduation-criteria-optional)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

Additional terms used in this proposal:

- **OCI artifact**: Content stored in an OCI registry that is not a runnable container image. OCI artifacts reuse the OCI image manifest and distribution specifications to store arbitrary files (here: provider manifests) addressable by name, tag, and digest.
- **ORAS**: [OCI Registry As Storage](https://oras.land/) — a project (and Go library, `oras-go`) for pushing and pulling OCI artifacts to/from OCI registries.
- **Provider manifests**: The YAML files a provider attaches to its release today: `metadata.yaml`, the `*-components.yaml` file(s), and (for infrastructure providers) cluster template files.
- **Digest**: The content-addressable identifier of an OCI manifest (`sha256:...`). Pulling by digest guarantees the content is exactly what was published.
- **CAPI Operator**: The [Cluster API Operator](https://github.com/kubernetes-sigs/cluster-api-operator), a declarative alternative to clusterctl for managing the lifecycle of providers in a management cluster.

## Summary

As it stands today, installing or upgrading Cluster API providers with `clusterctl` or the CAPI Operator requires access to github.com (or GitLab), because provider manifests are generally distributed as GitHub (or GitLab) release assets. This is a recurring pain point in air-gapped and regulated environments, and it makes both user installations and CAPI's own CI dependent on GitHub availability and rate limits.

This proposal introduces OCI artifacts as a packaging and way to distribute provider manifests. Cluster API will publish, for every provider it releases and for every new minor/patch release going forward, an OCI artifact containing the same manifest files that are attached to the GitHub release. The artifacts will be published to `registry.k8s.io` alongside the existing controller images, using the same staging and image-promotion process.

The proposal covers two ways of consuming the manifest OCIs:

1. **`clusterctl`** will have a new `oci://` URL scheme, implemented as a new repository client behind the existing repository abstraction. Built-in providers become resolvable from OCI in a deterministic, version-aware way.
2. **GitOps via the CAPI Operator**: users manage provider CRs (e.g. `CoreProvider`) with Flux/Argo CD, and the operator fetches the manifests from the OCI artifact using its existing `fetchConfig.oci` support. The artifact layout defined here is compatible with the operator as it exists today.

OCI is used here only as a packaging and distribution mechanism. The manifest content, the variable substitution mechanism, and the provider metadata format are all unchanged.

## Motivation

This proposal is a result of [issue #13729](https://github.com/kubernetes-sigs/cluster-api/issues/13729) and a discussion on the #cluster-api Kubernetes Slack channel.

The scenario shared in the Slack discussion was a user is running a semi-air-gapped management cluster and they had installed the CAPI Operator successfully (controller images were mirrored and pullable), but installing the core provider failed because the operator could not reach github.com to list releases and download manifests. The documented air-gapped workaround — manually downloading release assets and wrapping them in a ConfigMap — failed on the 1MiB ConfigMap size limit, and even with the supported compressed-ConfigMap annotation it remains a manual process that has to be repeated for every provider and every version and this is a pain.

The underlying problem is an asymmetry: controller images are already distributed through OCI registries (`registry.k8s.io`), which organizations can mirror with standard tooling, while the manifests that reference those images are only available from github.com. Distributing manifests as OCI artifacts removes the last GitHub dependency from the installation path and lets organizations use one mirroring workflow for everything.

There are two further motivations:

- **Ecosystem alignment**: distributing configuration as OCI artifacts is now well-established practice — Helm charts ([OCI registries](https://helm.sh/docs/topics/registries/)), Flux ([OCIRepository](https://fluxcd.io/flux/components/source/ocirepositories/)), and the CAPI Operator's existing [OCI support](https://cluster-api-operator.sigs.k8s.io/topics/configuration/air-gapped-environtment#using-oci-artifact) all consume OCI artifacts.
- **CI reliability**: CAPI's E2E and periodic jobs currently download provider manifests from github.com, which can cause flakiness (rate limits, transient failures). Fetching from registries (or from a local registry populated during the test run) removes those HTTP calls.

### Goals

- Publish, as part of the Cluster API release process, one OCI artifact per provider per version containing exactly the manifest files attached to the corresponding GitHub release.
- Define the artifact layout, naming, tagging, and annotation conventions as a documented contract that any provider can adopt.
- Ensure the artifacts are consumable, without modification, by the CAPI Operator's existing `fetchConfig.oci` mechanism, so that GitOps-managed provider CRs work in air-gapped environments.
- Add first-class OCI support to clusterctl via an `oci://` provider repository URL scheme, covering `init`, `upgrade`, and `generate provider`, including the built-in (hard-coded) providers.
- Provide the publishing tooling Cluster API uses in a reusable form so providers can publish conformant artifacts with minimal effort.
- Define how HTTP(S)/GitHub and OCI coexist for providers whose older versions predate OCI publication.

### Non-Goals/Future Work

- **Redesigning the templating mechanism.** Manifests keep their current `${VAR}` variables (see [Templating and variables](#templating-and-variables)). Replacing envsubst with something richer has its benefits, but it is orthogonal to how manifests are packaged, and coupling the two would delay getting the benefits of OCI. Revisiting templating is future work.
- **Direct consumption of the artifacts by Flux/Argo CD** (i.e. a Flux `OCIRepository` + `Kustomization` applying the components YAML without clusterctl or the CAPI Operator in the path). Because the manifests contain variables, the supported GitOps path in this proposal goes through the CAPI Operator. One of the stated montivations of the CAPI operator was to enable GitOps management of CAPI and providers.  Direct consumption will be considered in the future once the templating future work lands.
- **Changes to the CAPI Operator.** The artifact format is designed to be consumable by the operator as it exists today and as such this proposal doesn't cover any changes to the operator (see [CAPI Operator compatibility](#capi-operator-compatibility)).
- **Client-side signature verification** (e.g. cosign verification in clusterctl). See [Security Model](#security-model) for what this proposal does provide, and Future Work for verification.
- **Distributing the cert-manager manifest via OCI.** `clusterctl init` also downloads the cert-manager manifest from GitHub. cert-manager is a third-party project, and clusterctl already supports overriding its manifest URL and version via configuration, which is the existing air-gap escape hatch. This residual GitHub dependency is explicitly acknowledged and out of scope.
- **Backfilling older releases.** OCI artifacts are published from the first minor release that ships this feature onwards. Older release branches and already-published tags are not backfilled (see [Upgrade Strategy](#upgrade-strategy)).
- **A global registry-rewrite configuration** in clusterctl (a single knob redirecting all `oci://` URLs to a mirror). Per-provider URL overrides cover the air-gapped use case today; a global convenience knob can be added later if demand shows up.

## Proposal

### User Stories

#### Story 1: Air-gapped installation with the CAPI Operator

As a platform engineer in a (semi) air-gapped environment, 
I want to install and upgrade Cluster API providers without any component reaching github.com,
So that installation works within my organization's network policy.

I mirror `registry.k8s.io/cluster-api/manifests/*` into my internal registry with the same tooling I already use for controller images (e.g. `oras copy`, `crane copy`, or a pull-through cache). I then apply:

```yaml
apiVersion: operator.cluster.x-k8s.io/v1alpha2
kind: CoreProvider
metadata:
  name: cluster-api
  namespace: capi-system
spec:
  version: v1.15.0
  fetchConfig:
    oci: "registry.internal.example.com/cluster-api/manifests/core-cluster-api:v1.15.0"
```

This removes the need for the ConfigMap workaround, along with its size limits and per-version manual steps.

#### Story 2: GitOps-driven provider management

As a platform engineer using GitOps,
I want the full definition of my management cluster (including which providers are installed at which versions) to live in Git and be reconciled by Flux or Argo CD,
So that i have a single source of truth for my management cluster.

I commit `CoreProvider`/`BootstrapProvider`/`ControlPlaneProvider`/`InfrastructureProvider` resources (as in Story 1) to Git. Flux/Argo applies them; the CAPI Operator pulls the referenced OCI artifact, performs variable substitution, and installs the provider. Upgrading a provider is a one-line change to `spec.version` (and the artifact tag) in Git.

#### Story 3: clusterctl init from a private registry mirror

As a cluster operator,
I want `clusterctl init` and `clusterctl upgrade` to work using only my private registry,
So that i can adhere to my companies policy of using internal registries only.

For providers whose versions are published as OCI artifacts, clusterctl resolves versions and fetches manifests from the registry. If my registry requires authentication, my existing `docker login` credentials are used. In a fully air-gapped setup I override the provider URLs in `clusterctl` configuration to point at my mirror:

```yaml
providers:
  - name: "cluster-api"
    url: "oci://registry.internal.example.com/cluster-api/manifests/core-cluster-api"
    type: "CoreProvider"
```

#### Story 4: Provider maintainer adopting OCI distribution

As a provider maintainer, 
I want to offer OCI distribution of my manifests without inventing anything,
So that consumers can take advantage of its benefits like private registrues

I follow the documented contract (artifact layout, naming, tagging, annotations), reuse the publishing tool Cluster API itself uses in my release pipeline, and update my provider's repository URL in the clusterctl providers list to `oci://...` when I am ready to cut over.

#### Story 5: Reducing CI flakiness

As a Cluster API maintainer,
I want CI jobs to stop calling github.com for release manifests (the calls can be rate-limited and flaky),
So that there is less risk of CI failures.

E2E tests can consume providers from OCI artifacts pushed to a local registry during the test run, and periodic jobs testing released versions can pull from `registry.k8s.io`.

### Requirements (Optional)

#### Functional Requirements

##### FR1

For every provider released from the cluster-api repository (core, bootstrap-kubeadm, control-plane-kubeadm), the release process MUST publish an OCI artifact per provider per version to `registry.k8s.io`, containing exactly the manifest files attached to the corresponding GitHub release.

##### FR2

The published artifacts MUST be consumable by the CAPI Operator's existing `fetchConfig.oci` implementation without changes to the operator.

##### FR3

clusterctl MUST support an `oci://` scheme for provider repository URLs, usable in the built-in provider list, user-defined providers in `clusterctl` configuration, and everywhere an HTTP(S) repository URL is accepted today.

##### FR4

clusterctl MUST resolve provider versions from OCI registry tags, and MUST support fetching a provider at a pinned digest (`oci://...@sha256:...`).

##### FR5

For built-in providers, clusterctl MUST deterministically select between the GitHub repository and the OCI repository based on the requested provider version (see [clusterctl support](#clusterctl-support)). No network probing or fallback logic is permitted.

##### FR6

clusterctl MUST authenticate to OCI registries using the standard Docker credential chain (`~/.docker/config.json`, credential helpers, `DOCKER_CONFIG`).

##### FR7

Cluster API MUST provide the publishing tooling it uses (building and pushing conformant OCI images) in a form reusable by provider maintainers.

#### Non-Functional Requirements

##### NFR1

The OCI path MUST NOT change the content of the manifests, iles fetched from an OCI artifact are byte-for-byte identical to the corresponding GitHub release assets.

##### NFR2

The artifacts MUST be mirrorable with standard OCI tooling (`oras copy`, `crane copy`, registry pull-through caches) without any CAPI-specific tooling.

##### NFR3

The publishing step MUST NOT significantly extend the release process, and a publishing failure MUST be surfaced (release job failure) and be not silent.

##### NFR4

E2E coverage of the OCI path MUST be hermetic — it must not depend on external registries at test runtime.

### Implementation Details/Notes/Constraints

#### OCI artifact format

**One artifact per provider per version.** Each provider gets its own OCI repository; each release of that provider is one tag. For the providers released from the cluster-api repository:

| Provider | Artifact repository | Contents |
|---|---|---|
| core `cluster-api` | `registry.k8s.io/cluster-api/manifests/core-cluster-api` | `metadata.yaml`, `core-components.yaml` |
| bootstrap `kubeadm` | `registry.k8s.io/cluster-api/manifests/bootstrap-kubeadm` | `metadata.yaml`, `bootstrap-components.yaml` |
| control plane `kubeadm` | `registry.k8s.io/cluster-api/manifests/control-plane-kubeadm` | `metadata.yaml`, `control-plane-components.yaml` |

> NOTE: Infrastructure providers could additionally include their cluster template files (`cluster-template*.yaml`). This is TBD at this stage. There is an argument that because the templates are attached to the release that they should be included. 

The artifact itself is a standard OCI image manifest with:

- A single layer: a gzipped tarball (`application/vnd.oci.image.layer.v1.tar+gzip`) containing the files above at the root. No custom layer media types and no custom `artifactType` are used. This keeps the artifact consumable by the widest range of registries and tools (ORAS, Flux, the CAPI Operator, `crane`), and mirrors what generic-content publishers do today. (Custom media types were considered; see [Alternatives](#alternatives).)
- Required **manifest annotations**, which carry the identification that a custom media type would otherwise provide:

| Annotation | Example | Meaning |
|---|---|---|
| `org.opencontainers.image.title` | `core-cluster-api` | Artifact name (`{type}-{name}`) |
| `org.opencontainers.image.version` | `v1.15.0` | Provider version (== tag) |
| `org.opencontainers.image.source` | `https://github.com/kubernetes-sigs/cluster-api` | Source repository |
| `cluster-api.sigs.k8s.io/provider-type` | `core` | Provider type (`core`, `bootstrap`, `controlPlane`, `infrastructure`, `ipam`, `runtimeextension`, `addon`) |
| `cluster-api.sigs.k8s.io/provider-name` | `cluster-api` | Provider name as known to clusterctl |

Consumers (clusterctl) validate the annotations after pulling and return an error when the artifact does not conform to the contract. Together, the file-name conventions and the annotations form the normative artifact contract.

#### Naming and tagging conventions

- **Repository path**: `{registry}/{project}/manifests/{type}-{name}`, type-qualified because provider names alone are ambiguous (both kubeadm providers are named `kubeadm`). The `manifests/` path segment separates these artifacts from controller images, following the path-based organization preferred in the issue discussion (over a `-manifests` name suffix, which is recorded in [Alternatives](#alternatives)).
- **Tag**: exactly the provider release version, including the `v` prefix (`v1.15.0`, `v1.16.0-beta.0`). One tag per release; tags are never mutated after publication. There is no `latest` tag.  The latest version is resolved by listing tags and applying the same semver ordering clusterctl uses for GitHub releases today.
- **Digest references**: consumers can pin `@sha256:...` wherever a tag is accepted.

Third-party providers follow the same pattern under their own registry namespace, e.g. `registry.k8s.io/cluster-api-aws/manifests/infrastructure-aws:v2.9.0`.

#### Templating and variables

The manifests inside the artifact are identical to today's release assets and therefore contain envsubst `${VAR}` variables. Nothing about variable handling changes:

- **clusterctl** performs variable substitution after fetching, just as it does for GitHub-sourced manifests.
- **The CAPI Operator** performs substitution using variables from the provider's `configSecret`, as it does today.

Both consumers use the same substitution engine: [drone/envsubst](https://github.com/drone/envsubst)-style processing as wrapped by clusterctl's yamlprocessor (supporting defaults and the additional functions beyond POSIX envsubst). Because both supported consumption paths (clusterctl, operator) substitute variables themselves, publishing manifests containing variables is safe.

A question raised in the issue discussion was whether Flux/Argo would need to run envsubst over the manifests. The short answer is they don't. In this proposal Flux/Argo never apply the components YAML directly. Instead they apply the CAPI Operator's provider CRs (Story 2), and the CAPI Operator handles substitution. Applying the artifact contents directly with Flux/Argo is explicitly deferred to a later date when discussions on templating have occured.

#### Publishing pipeline

A small Go tool, built on `oras-go` and living in the cluster-api repository (e.g. `hack/tools/oci-publish`). When running it you supply

- a directory of release assets (the same staging directory used to attach assets to the GitHub release),
- provider type, name, and version,
- a target reference,

It then produces and pushes the conformant artifact (file selection, tar+gzip layer, annotations). Since the same tool is offered to providers ([FR7](#fr7)), the contract has a single implementation to maintain.

The tool is wired into the release process as follows:

- A new make target (e.g. `make release-oci-artifacts`) invokes the tool once per provider.
- A new step in `cloudbuild.yaml` publishes the artifacts to the staging registry (`gcr.io/k8s-staging-cluster-api/manifests/...`) on release builds, alongside the existing image builds.
- Promotion to `registry.k8s.io` uses the standard Kubernetes image-promotion process (the promoter operates on OCI manifests by digest and handles artifacts the same way it handles images).
- Docs: the book gains a page describing the artifact format and how to consume/mirror it.

#### clusterctl support

**`oci://` URL scheme.** Provider repository URLs may now use the `oci://` scheme:

```
oci://registry.k8s.io/cluster-api/manifests/core-cluster-api
```

A new repository client (alongside the existing GitHub, GitLab, and local implementations in `cmd/clusterctl/client/repository`) implements the existing `Repository` interface:

- `GetVersions()` — lists registry tags and filters them to valid semver, reusing clusterctl's existing version handling.
- `GetFile(version, path)` — pulls the artifact for the version (by tag, or digest when the URL pins one), validates annotations, extracts the requested file from the layer.
- Default components path etc. behave the same as for other repository types.

Because the abstraction boundary is the existing `Repository` interface, everything above it (`init`, `upgrade plan/apply`, `generate provider`, overrides layer, local caching) works unchanged.

**Authentication** uses the standard Docker credential chain, so `docker login registry.internal.example.com` (or a credential helper) is all a user needs. Thus matches the behavior users already rely on for `docker`/`oras`/`helm`.

**Built-in providers and HTTP/OCI coexistence.** OCI artifacts only exist from a given version onwards, so the built-in provider table (in code, not in the user-facing configuration schema) carries, per provider:

- the existing GitHub repository URL,
- the OCI repository URL,
- the first version available via OCI (the "OCI floor" — the first minor release published with this feature).

Resolution needs to be deterministic. If the requested version is at or above the floor, clusterctl uses the OCI repository, otherwise it uses GitHub. There is no probing and no fallback. Version listing merges both sources' version sets for `upgrade plan` purposes. In practice, GitHub remains authoritative for pre-floor versions and OCI for post-floor versions.

The **user-facing configuration schema is unchanged.** A provider entry still has exactly one `url`, which may now be `oci://`. A user override replaces the built-in dual-source logic entirely, users get precisely the repository they configured, which keeps air-gapped behavior predictable. Third-party providers adopt by switching the single URL in their providers-list entry when ready (older versions of such a provider must then also be available in that OCI repository, or users needing them can override back to the GitHub URL).

**Rollout.** In the first release (N) with this feature, built-in providers keep GitHub as the default source and OCI is opt-in via configuration override; CAPI's own CI switches to the OCI path to exercise it continuously. In release N+1, after a full release cycle of stable publishing, the built-in defaults flip to OCI for versions >= the floor. This staging avoids making an unproven publishing pipeline a single point of failure for every `clusterctl init` on day one.

#### CAPI Operator compatibility

This proposal deliberately makes **no changes** to the CAPI Operator as the artifact format is designed to be consumable by the operator's [existing OCI support](https://cluster-api-operator.sigs.k8s.io/topics/configuration/air-gapped-environtment#using-oci-artifact):

- The operator accepts `fetchConfig.oci: "<registry>/<repository>:<tag>"` (or without a tag, defaulting the tag to `spec.version`) — both work with the naming scheme above.
- The operator looks for `metadata.yaml` (default metadata file name) and `{type}-components.yaml` (typed component file name) inside the artifact — the same file names this proposal publishes.
- The operator authenticates via `OCI_USERNAME`/`OCI_PASSWORD`/`OCI_ACCESS_TOKEN`/`OCI_REFRESH_TOKEN` keys in the provider's `configSecret` — unchanged by this proposal.

The compatibility statement in this CAEP is, any change to the artifact contract defined here MUST remain consumable by the operator's released `fetchConfig.oci` implementation, and the E2E suite includes a test asserting the operator can install the core provider from a published artifact. Evolution of the operator itself (e.g. richer artifact validation using the annotations) stays in the cluster-api-operator project.

Note: the operator's air-gapped ConfigMap mode remains available but is no longer necessary once artifacts can be pulled from a mirrored registry. No ConfigMap-shaped variant of the artifact is published (see [Alternatives](#alternatives)).

#### Provider contract

A new optional section is added to the clusterctl provider contract documentation. A provider offering OCI distribution MUST:

1. Publish one OCI artifact per provider per release version, with the layout, file names, and annotations defined in [OCI artifact format](#oci-artifact-format).
2. Use the repository path convention `.../manifests/{type}-{name}` and tag == release version.
3. Never mutate a published tag.
4. Update its entry in the clusterctl providers list to the `oci://` URL when cutting over (or document the URL for user configuration).

Providers SHOULD use the publishing tool provided by Cluster API ([Publishing pipeline](#publishing-pipeline)) to guarantee conformance. Nothing else changes for providers: manifest content, `metadata.yaml`, and versioning semantics are identical to the GitHub release contract.

### Security Model

- **No new privileges in the cluster.** The artifacts contain the same manifests as GitHub releases; RBAC implications of installing providers are unchanged. This proposal only changes the transport.
- **Registry authentication.** clusterctl uses the standard Docker credential chain; credentials are stored where users already store them (Docker config / credential helpers), not in clusterctl configuration. The CAPI Operator reads registry credentials from the provider's `configSecret` (a Kubernetes Secret), existing behavior. No new secret material is introduced, and no credentials are ever embedded in artifacts.
- **Integrity.**
  - Artifacts are published through the Kubernetes staging + image-promotion pipeline, inheriting its access controls, audit trail, and provenance/signing posture — identical to the controller images users already trust.
  - Tags on `registry.k8s.io` are immutable once promoted.
  - Consumers can pin digests (`oci://...@sha256:...` in clusterctl configuration, digest references in `fetchConfig.oci`), giving content-addressed integrity end-to-end.
  - Transport is HTTPS; clusterctl's OCI client does not support plain HTTP registries.
- **Future work**: client-side signature verification (e.g. cosign) in clusterctl, once a verification UX (key distribution, air-gapped verification policy) is agreed. Digest pinning is the supported integrity mechanism until then.

### Risks and Mitigations

- **Risk: the publishing step fails or publishes a malformed artifact on release day.**
  Mitigation: opt-in rollout in release N (GitHub remains the default path); CI exercises the full publish→consume loop hermetically on every merge; a publishing failure causes the release job to fail; defaults only flip in N+1 after a proven cycle.
- **Risk: registry.k8s.io outage breaks `clusterctl init`.**
  Mitigation: comparable to today's github.com dependency, and an improvement for users with mirrors (mirroring OCI is standard practice; mirroring GitHub releases is not). Deterministic source selection means users can always override to another source.
- **Risk: drift between this contract and the CAPI Operator's expectations.**
  Mitigation: the file naming was chosen to match the operator's released implementation; an E2E test installs a provider through the operator from a published artifact; the contract documentation marks the operator as a compatibility constraint for any future format change.
- **Risk: divergence between GitHub assets and OCI content.**
  Mitigation: the artifact is built from the same staging directory as the release assets in the same release job ([NFR1](#nfr1)); the E2E smoke test compares file digests between the two channels for a released version.
- **Risk: third-party providers publish non-conformant artifacts.**
  Mitigation: single reusable publishing tool; clusterctl's annotation/layout validation produces errors that reference the contract documentation.

The clusterctl UX changes (URL scheme, error messages, docs) should be reviewed by the clusterctl maintainers, and the operator/GitOps flow by the CAPI Operator maintainers, who are reviewers on this proposal. The publishing-pipeline changes additionally go through the standard k8s-infra image-promotion review.

## Alternatives

- **Two artifact variants (plain + air-gapped ConfigMap), or two images per provider.** The initial idea in the issue discussion, publish both the plain manifests and a ConfigMap-wrapped, compressed variant for the operator's air-gapped ConfigMap mode. Rejected per review feedback because it doubles the published surface and bakes one consumer's workaround into the release process. It is also unnecessary once the operator can pull artifacts directly from a mirrored registry. Publishing a single variant and letting consumers adapt the content to their needs is simpler.
- **Single artifact per release (all providers in one).** Mirrors the GitHub release layout (one release serving core + both kubeadm providers) and means one publish/promote/mirror operation. Rejected in favor of per-provider artifacts: a 1:1 mapping between artifact and provider matches the operator's `fetchConfig.oci` model (one reference per provider CR), makes tags/annotations unambiguous, and gives third-party providers (who ship one provider per repo) the same shape as the core providers. The operator's file-naming conventions also support multi-provider artifacts, so users who want to build a single bundled artifact for internal use can still do so — it is simply not what CAPI publishes.
- **Name-suffix naming (`cluster-api-controller-manifests:v1.14.0`).** The first naming idea in the issue. Superseded by path-based organization (`.../manifests/...`), which groups the artifacts cleanly in registry listings and avoids overloading the controller image name.
- **Custom media types / artifactType** (as Flux and Helm use, and as suggested in the issue). Considered but rejected in favor of generic layer media types plus required OCI manifest annotations. Annotations provide the same self-identification for consumers that need it, while generic types maximize registry and tooling compatibility and avoid standardizing a new media type that other tools would need to learn. If a compelling need for a dedicated `artifactType` emerges (e.g. registry-side filtering), it can be added later without breaking consumers that ignore it.
- **Layer selection (one artifact, one layer per scenario).** Requires layer-selection support in every consumer (unclear in Argo, absent in the operator); rejected in the issue discussion in favor of "one variant, plain files".
- **Helm charts as the distribution format.** Would bring real templating, but changes the content contract, not just the packaging, and the community has repeatedly declined to maintain official charts ("we don't want/can't handle the complexity of helm charts"). Out of scope; the templating discussion is deliberately deferred.
- **Probe-and-fallback source selection in clusterctl** (try OCI, fall back to GitHub). Rejected: network-dependent behavior is confusing to debug, masks publishing failures, and behaves badly in air-gapped environments. Deterministic version-based selection was chosen instead.

## Upgrade Strategy

There is no impact on existing management clusters or workload clusters as this proposal changes how manifests are *fetched*, not what is installed.

- **Existing behavior is preserved**: users doing nothing continue to fetch from GitHub in release N. Provider versions older than the OCI floor are only available via GitHub, permanently (no backfill).
- **Using the enhancement**: in release N, set an `oci://` URL (override or user-defined provider) or use `fetchConfig.oci` with the operator. From release N+1, `clusterctl init/upgrade` uses OCI automatically for built-in providers at versions >= the floor.
- **clusterctl upgrade across the floor**: `upgrade plan` merges version lists from both sources; upgrading a provider from a pre-floor version (fetched via GitHub) to a post-floor version (fetched via OCI) is transparent.
- **Version skew**: an older clusterctl (without OCI support) keeps working against GitHub for all versions — GitHub release assets continue to be published unchanged; this proposal adds a channel, it does not remove one.

## Additional Details

### Test Plan [optional]

The guiding constraint is hermeticity ([NFR4](#nfr4)), CI must not depend on external registries for new tests involving manifest OCI images.

- **Unit tests**: the new clusterctl OCI repository client against an in-memory/fake registry (tag listing, artifact pulling, annotation validation, digest pinning, auth resolution); the publishing tool (layout, annotations, determinism).
- **E2E/integration**: a local OCI registry (zot or docker registry) is started as part of the test environment. The publishing tool pushes artifacts built from the working tree and `clusterctl init` runs with provider URLs pointing at `oci://localhost:...`. This exercises the full publish→consume loop, including the conformance of the publisher and consumer against the same contract. A variant installs the core provider through the CAPI Operator from the local registry to enforce the compatibility statement.
- **Post-release verification**: a periodic job pulls the published artifacts from `registry.k8s.io` for the latest release, validates annotations/layout, and compares file digests against the GitHub release assets.

### Graduation Criteria [optional]

- **Phase 1 (release N — opt-in)**: artifacts published for all cluster-api providers; `oci://` support in clusterctl behind explicit configuration; CAPI CI consuming OCI; documentation for operator/GitOps and clusterctl usage.
- **Phase 2 (release N+1 — default)**: built-in clusterctl providers default to OCI for versions >= the floor, gated on: one full release cycle with no publishing failures, the periodic verification job green, and no unresolved consumer bug reports.
- Signals for further graduation of the provider contract (adoption by providers, e.g. the kubeadm-adjacent and major infrastructure providers) will be tracked on the umbrella issue.

## Implementation History

- [x] 05/2026: Problem discussed on [#cluster-api Slack](https://kubernetes.slack.com/archives/C8TSNPY4T/p1779354884813219)
- [x] 05/2026: Issue opened: [#13729 — Publish release manifest/configuration as OCI artifacts](https://github.com/kubernetes-sigs/cluster-api/issues/13729)
- [x] 05/2026–06/2026: Design discussion on the issue (format variants, naming, templating, clusterctl direction)
- [x] 08/04/2026: Open proposal PR (this document)
- [ ] MM/DD/YYYY: Present proposal at a [community meeting]

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1ushaVqAKYnZ2VN_aa3GyKlS4kEd6bSug13xaXOakAQI/edit#heading=h.pxsq37pzkbdq
