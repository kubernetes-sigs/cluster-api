# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Project Overview

Cluster API (CAPI) is a Kubernetes subproject providing declarative APIs and tooling to simplify provisioning, upgrading, and operating multiple Kubernetes clusters. It uses Kubernetes-style APIs and patterns to automate cluster lifecycle management.

## Common Commands

### Building
```bash
make managers              # Build all controller manager binaries
make clusterctl            # Build the clusterctl CLI binary
make generate              # Run all code generation (manifests, deepcopy, conversions, openapi)
make docker-build-e2e      # Build docker images for e2e testing
```

### Testing
```bash
make test                  # Run unit and integration tests with race detector
make test TEST_ARGS="-run TestName"  # Run a specific test
make test-verbose          # Run tests with verbose output
make lint                  # Run all linters (golangci-lint, hadolint, kube-api-linter)
make lint-fix              # Run linters with auto-fix
make verify                # Run all verification checks
```

### E2E Testing
```bash
make test-e2e              # Run end-to-end tests
make kind-cluster          # Create a kind cluster for development with Tilt
make tilt-up               # Start Tilt for local development
```

### Documentation
```bash
make serve-book            # Build and serve the documentation book with live-reload
```

## Architecture

### Core Components

The repository contains multiple Go modules:
- **Root module** (`sigs.k8s.io/cluster-api`): Core CAPI controllers and APIs
- **`test/`**: E2E test framework and test infrastructure providers (CAPD, in-memory)
- **`hack/tools/`**: Development tools

### API Structure

APIs are organized by domain and version under `api/`:
- `api/core/v1beta1`, `api/core/v1beta2`: Core types (Cluster, Machine, MachineSet, MachineDeployment, MachinePool, ClusterClass)
- `api/bootstrap/kubeadm/`: Kubeadm bootstrap provider types
- `api/controlplane/kubeadm/`: Kubeadm control plane provider types
- `api/addons/`: ClusterResourceSet types
- `api/ipam/`: IP address management types
- `api/runtime/`: Runtime extension types and hooks

### Controller Organization

Controllers are in `internal/controllers/` (core) and provider-specific directories:
- `internal/controllers/cluster/`: Cluster controller
- `internal/controllers/machine/`: Machine controller
- `internal/controllers/machinedeployment/`: MachineDeployment controller
- `internal/controllers/machineset/`: MachineSet controller
- `internal/controllers/machinepool/`: MachinePool controller
- `internal/controllers/topology/`: ClusterClass/topology controllers
- `bootstrap/kubeadm/internal/controllers/`: Kubeadm bootstrap controller
- `controlplane/kubeadm/internal/controllers/`: KubeadmControlPlane controller

### Built-in Providers

- **Bootstrap Provider (CABPK)**: `bootstrap/kubeadm/` - Generates kubeadm configuration
- **Control Plane Provider (KCP)**: `controlplane/kubeadm/` - Manages kubeadm-based control planes
- **Infrastructure Provider (CAPD)**: `test/infrastructure/docker/` - Docker-based infrastructure for testing
- **In-Memory Provider**: `test/infrastructure/inmemory/` - In-memory infrastructure for testing

### clusterctl CLI

The `clusterctl` CLI is in `cmd/clusterctl/`:
- `cmd/clusterctl/cmd/`: Command implementations
- `cmd/clusterctl/client/`: Client library for clusterctl operations
- `cmd/clusterctl/client/cluster/`: Cluster-level operations (move, upgrade)
- `cmd/clusterctl/client/repository/`: Provider repository handling

### Utilities

- `util/`: Shared utilities for controllers and providers
- `util/conditions/`: Condition helpers for status management
- `util/patch/`: Patch helpers for resource updates
- `util/predicates/`: Controller predicates
- `exp/`: Experimental features behind feature gates

## Key Patterns

### Controller Reentrancy
Controllers should be reentrant - able to recover from interruptions and act on current state rather than relying on flags from previous reconciliations. This is critical for the `clusterctl move` operation.

### API Conventions
- Follow Kubernetes API conventions enforced by kube-api-linter (`.golangci-kal.yml`)
- Breaking API changes require a CAEP (Cluster API Enhancement Proposal)
- CRD printer columns should follow the documented order in CONTRIBUTING.md

### PR Labels
All PRs must be labeled with one of:
- `:warning:` - Major or breaking changes
- `:sparkles:` - Feature additions
- `:bug:` - Bug fixes
- `:book:` - Documentation
- `:seedling:` - Minor or other changes

## Code Generation

After modifying API types, run:
```bash
make generate              # All generation
make generate-manifests    # CRD/RBAC manifests
make generate-go-deepcopy  # DeepCopy methods
make generate-go-conversions  # API version conversions
```

## Testing Infrastructure

- Unit tests use `envtest` (kubebuilder test framework)
- E2E tests use Ginkgo and create real clusters with CAPD
- Test artifacts in CI are available via the "Artifacts" link in Prow logs
