# Developing "core" Cluster API

This section of the book is about developing "core" Cluster API.

With "core" Cluster API we refer to the common set of API and controllers that are required to run 
any Cluster API provider.

Please note that in the Cluster API code base, side by side of "core" Cluster API components there
is also a limited number of in-tree providers:

- Kubeadm bootstrap provider (CAPBK)
- Kubeadm control plane provider (KCP)
- Docker infrastructure provider (CAPD) - The Docker provider is not designed for production use and is intended for development & test only.

Please refer to [Developing providers](../providers/overview.md) for documentation about in-tree providers (and out of tree providers too).

## Rules

- Core code must work with any provider: core controllers never import provider types (KCP, CABPK, CAPD); they
  access provider objects as `unstructured` through `internal/contract` and `controllers/external`.
- `util/` and `utils/` are public API used by providers; don't break them without a reason. `exp/` and `internal/`
  have no compatibility guarantees.
- Never hand-edit generated files (`zz_generated.*`, `config/crd/bases`, `config/rbac`, `config/webhook`); change the
  Go types or kubebuilder markers and run `make generate`.
- Manager cache and client configuration lives in `<provider>/setup`; `main.go` and every `suite_test.go` use the same
  helpers, so don't duplicate it (see [Code layout](#code-layout)).
- Before changing a specific area, read the matching page: [API](API.md), [Controllers](controllers/overview.md),
  [Logging](logging.md), [Testing](testing.md), [E2E tests](e2e.md), [Provider contracts](../providers/contracts/overview.md).

## Code layout

The repository ships several controller-manager binaries that share code: core Cluster API (`core/main.go`),
kubeadm bootstrap / CABPK (`bootstrap/kubeadm/main.go`), kubeadm control plane / KCP (`controlplane/kubeadm/main.go`),
CAPD (`test/infrastructure/docker/main.go`), the Runtime SDK test extension (`test/extension/main.go`), plus the
`clusterctl` CLI (`cmd/clusterctl`).

Each provider uses the same layout:

- `<provider>/reconcilers/<kind>/`: controllers, e.g. `core/reconcilers/machine`.
- `<provider>/webhooks/admission/`: defaulting and validation webhooks; `<provider>/webhooks/conversion/`: conversion webhooks.
- `<provider>/setup/setup.go`: manager cache/client options (`ManagerCacheOptions`, `ManagerClientOptions`, ...) and
  controller wiring shared by `main.go` and the tests, so tests run with a production-like cache configuration.
- `<provider>/config/`: kustomize bases; `config/crd/bases`, `config/rbac` and `config/webhook` are generated from
  kubebuilder markers (`make generate-manifests-<provider>`).

There are five Go modules: the root module, `api/` (API types), `utils/` (public helpers for providers and extension
authors, with a strict depguard allow-list), `test/` (e2e framework, CAPD, in-memory provider, test extension) and
`hack/tools/`. Run `go build`/`go test` from the module that owns the package, and `make generate-modules` to tidy them.

Other important packages:

- `internal/contract`: typed accessors for provider objects (InfraCluster, InfraMachine, BootstrapConfig, ControlPlane)
  handled as `unstructured`, encoding the provider contract per contract version.
- `controllers/clustercache`: cached clients and watches for workload clusters; use it instead of building REST clients ad hoc.
- `controllers/crdmigrator`: migrates stored CRs and managedFields to the current storage version.
- ClusterClass and managed topology: `core/reconcilers/topology/cluster` drives it, `exp/topology/desiredstate`
  computes the desired state, `internal/topology/*` holds variables, patches, upgrade sequencing and ownerRefs.
- Runtime SDK: `api/runtime/hooks` (API), `pkg/runtimesdk` (client, registry, hook tracking), `utils/runtimesdk/server`
  (server for extension authors).
- `util/patch`, `util/conditions`, `util/collections`, `util/predicates`, `util/annotations`: helpers used by all
  controllers, including providers; `util/test/builder`: generic test providers and objects.
- `internal/util/ssa`: server-side apply helpers (topology and KCP use SSA with managed-field-aware diffing).
- `feature/`: feature gates.

KCP and MachineSet share logic in places (e.g. creating BootstrapConfig and InfraMachine, managedFields cleanup).
Keep the code and comments of both identical, and move code to a shared helper when it is genuinely reused.
