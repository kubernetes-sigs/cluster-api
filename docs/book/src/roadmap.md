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

## v0.4 (v1alpha4) ~ July 2020

- [Machine bootstrap failure detection](https://github.com/kubernetes-sigs/cluster-api-provider-aws/issues/972)
- [Improved status conditions](https://github.com/kubernetes-sigs/cluster-api/issues/1658)
- [Machine load balancers](https://github.com/kubernetes-sigs/cluster-api/issues/1250)
- [Extensible machine pre-delete hooks](https://github.com/kubernetes-sigs/cluster-api/issues/1514)
- Define clusterctl inventory specification & have providers implement it
- Figure out how to change configurations through rolling updates (e.g. apiserver flags, DNS/addons, etc.)

## v0.5 (v1alpha5? v1beta1?) ~ November 2020

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
