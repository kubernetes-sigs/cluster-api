---
title: From CAPD(docker) to CAPD(dev)
authors:
  - "@fabrziopandini"
reviewers:
  - ""
creation-date: 2025-01-25
last-updated: 2025-01-25
status: implementable
see-also: []  
replaces: []
superseded-by: []  
---

# From CAPD(docker) to CAPD(dev)

## Table of Contents

- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goalsfuture-work)
- [Proposal](#proposal)
- [Implementation History](#implementation-history)

## Summary

This document proposes to evolve the CAPD(docker) provider into a more generic CAPD(dev) provider
supporting multiple backends including docker, in-memory, may be also kubemark.

## Motivation

In the Cluster API core repository  we currently have two infrastructure providers designed for use during development
and test, CAPD(docker) and CAPIM(in-memory).

If we look to all the Kubernetes SIG cluster lifecycle sub projects, there is also a third infrastructure provider
designed for development and test, CAPK(Kubemark).

Maintaining all those providers requires a certain amount or work, and this work mostly falls on a small set
of maintainers taking care of CI/release signal.

This proposal aims to reduce above toil and reducing the size/complexity of test machinery in Cluster API 
by bringing together all the infrastructure providers designed for use during development and listed above.

### Goals

- Reduce toil and maintenance effort for infrastructure provider designed for development and test.

### Non-Goals

- Host any infrastructure provider designed for development and test outside of the scope defined above.

## Proposal

This proposal aims to evolve the CAPD(docker) provider into a more generic CAPD(dev) provider
capable to support multiple backends including docker, in-memory, may be also kubemark.

The transition will happen in two phases:

In phase 1, targeting CAPI 1.1 (current release cycle), two new kinds will be introduced in CAPD: `DevCluster` and `DevMachine`

`DevCluster` and `DevMachine` will have 
- A `docker` backend, functionally equivalent to `DockerCluster` and `DockerMachine`
- An `inMemory`, functionally equivalent to CAPIM's `InMemoryCluster` and `InMemoryMachine`
- if and when Maintainers of CAPK(Kubemark) will choose to converge to CAPD(dev), a `kubemark` backend,  
  functionally equivalent to `KubemarkMachine` (there is no `KubemarkCluster`)

Below you can fin an example of `DockerCluster` and corresponding `DevMachine` using the docker backend:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: DockerMachine
metadata:
  name: controlplane
spec:
  extraMounts:
    - containerPath: "/var/run/docker.sock"
      hostPath: "/var/run/docker.sock"
```

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: DevMachine
metadata:
  name: controlplane
spec:
  backend:
    docker:
      extraMounts:
        - containerPath: "/var/run/docker.sock"
          hostPath: "/var/run/docker.sock"
```

And an example of `InMemoryMachine` and corresponding `DevMachine` using the in memory backend:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/alpha1
kind: InMemoryMachine
metadata:
  name: in-memory-control-plane
spec:
  behaviour:
    inMemory:
      vm:
        provisioning:
          startupDuration: "10s"
          startupJitter: "0.2"
      node:
        provisioning:
          startupDuration: "2s"
          startupJitter: "0.2"
      apiServer:
        provisioning:
          startupDuration: "2s"
          startupJitter: "0.2"
      etcd:
        provisioning:
          startupDuration: "2s"
          startupJitter: "0.2"
```

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: DevMachine
metadata:
  name: in-memory-control-plane
spec:
  backend:
    inMemory:
      vm:
        provisioning:
          startupDuration: "10s"
          startupJitter: "0.2"
      node:
        provisioning:
          startupDuration: "2s"
          startupJitter: "0.2"
      apiServer:
        provisioning:
          startupDuration: "2s"
          startupJitter: "0.2"
      etcd:
        provisioning:
          startupDuration: "2s"
          startupJitter: "0.2"
```

Please note that at the end of phase 1
- `DockerCluster` and `DockerMachine` will continue to exist, thus allowing maintainers and users 
  time to migrate progressively to the new kinds; however, it is important to notice that
  `DockerCluster` and `DockerMachine` should be considered as deprecated from now on.
- `InMemoryCluster` and `InMemoryMachine` will be removed, as well as the entire CAPIM provider.
  (CAPIM is used only for CAPI scale tests, so users are not impacted and migration can be executed faster).

Phase 2 consist of the actual removal of `DockerCluster` and `DockerMachine`; this phase will happen after maintainers 
will complete the transition of all the E2E tests to `DevCluster` and `DevMachine`; considering we have upgrade tests using
older releases of CAPD, completing this phase would likely require a few release cycles, targeting tentatively CAPI 1.13, Apr 2026.

## Implementation History

- [ ] 2025-01-25: Open proposal PR
- [ ] 2025-01-29: Present proposal at a community meeting
