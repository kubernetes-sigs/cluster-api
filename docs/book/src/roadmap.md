# Cluster API Roadmap

This roadmap is a constant work in progress, subject to frequent revision. Dates are approximations.

## Ongoing

- Documentation improvements

## v0.3 (v1alpha3) ~ March 2020

- [Control plane provider concept, KubeadmControlPlane implementation](https://github.com/kubernetes-sigs/cluster-api/blob/bf51a2502f9007b531f6a9a2c1a4eae1586fb8ca/docs/proposals/20191017-kubeadm-based-control-plane.md)
- [clusterctl v2 - easier installation & upgrades of Cluster API + providers](https://github.com/kubernetes-sigs/cluster-api/blob/bf51a2502f9007b531f6a9a2c1a4eae1586fb8ca/docs/proposals/20191016-clusterctl-redesign.md)
- [Integration with cloud provider VM scaling groups (MachinePools)](https://github.com/kubernetes-sigs/cluster-api/blob/bf51a2502f9007b531f6a9a2c1a4eae1586fb8ca/docs/proposals/20190919-machinepool-api.md)
- [Machine failure remediation](https://github.com/kubernetes-sigs/cluster-api/blob/bf51a2502f9007b531f6a9a2c1a4eae1586fb8ca/docs/proposals/20191030-machine-health-checking.md)
- [Provider end to end testing framework](https://github.com/kubernetes-sigs/cluster-api/blob/bf51a2502f9007b531f6a9a2c1a4eae1586fb8ca/docs/proposals/20191016-e2e-test-framework.md)

## v0.3.x (v1alpha3+) ~ June/July 2020

- [E2E Test plan](https://docs.google.com/spreadsheets/d/1uB3DyacOLctRjbI6ov7mVoRb6PnM4ktTABxBygt5sKI/edit#gid=0)
- [Enable webhooks in integration tests](https://github.com/kubernetes-sigs/cluster-api/issues/2788)
- [KCP robustness](https://github.com/kubernetes-sigs/cluster-api/issues/2753)
- [KCP adoption](https://github.com/kubernetes-sigs/cluster-api/issues/2214)
- [Clusterctl templating](https://github.com/kubernetes-sigs/cluster-api/issues/2339)
- [Clusterctl cert-manager lifecycle](https://github.com/kubernetes-sigs/cluster-api/issues/2635)
- [Post apply YAML experiment](https://github.com/kubernetes-sigs/cluster-api/issues/2395)
- [MHC Support for control-plane nodes](https://github.com/kubernetes-sigs/cluster-api/issues/2836)
- [Remote workload cluster watcher](https://github.com/kubernetes-sigs/cluster-api/issues/2414)
- [Improved status conditions](https://github.com/kubernetes-sigs/cluster-api/issues/1658)
    - UX: Clusterctl should offer a command to see the status of a cluster (library + cli).
- [Spot instances support](https://github.com/kubernetes-sigs/cluster-api/issues/1876)
- [Machine bootstrap failure detection](https://github.com/kubernetes-sigs/cluster-api/issues/2554)
- [Machine load balancers](https://github.com/kubernetes-sigs/cluster-api/issues/1250)
- [Extensible machine pre-delete hooks](https://github.com/kubernetes-sigs/cluster-api/issues/1514)
- [Autoscaler integration](https://github.com/kubernetes-sigs/cluster-api/pull/2530)

## v0.4 (v1alpha4) ~ Q4 2020

- Define clusterctl inventory specification & have providers implement it
- Figure out how to change configurations through rolling updates (e.g. apiserver flags, DNS/addons, etc.)
- [Move away from corev1.ObjectReference](https://github.com/kubernetes-sigs/cluster-api/issues/2318)
- ?

## TBD

- [Pluggable MachineDeployment upgrade strategies](https://github.com/kubernetes-sigs/cluster-api/issues/1754)
- Dual stack IPv4/IPv6 support
- Windows nodes
- Auto-scaling support (either with cluster-autoscaler, or something else)
- UEFI booting (image-builder)
- GPU support (BitFusion, Elastic GPU, etc.) (may be provider-specific)
- SR-IOV support (may be provider-specific, may be image-builder)
- FIPS/NIST/STIG compliance
- [Simplified cluster creation experience](https://github.com/kubernetes-sigs/cluster-api/issues/1227)
- Solid story on certificate management
- [Node attestation](https://github.com/kubernetes-sigs/cluster-api/issues/1739)
- Desired machine states ([running/stopped](https://github.com/kubernetes-sigs/cluster-api/issues/1810) / [restart](https://github.com/kubernetes-sigs/cluster-api/issues/1808))
- Define Cluster API conformance
