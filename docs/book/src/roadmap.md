# Cluster API Roadmap

This roadmap is a constant work in progress, subject to frequent revision. Dates are approximations.

## Ongoing

- Documentation improvements

## v0.3 (v1alpha3) ~ March 2020

- Control plane provider concept, KubeadmControlPlane implementation
- clusterctl v2 - easier installation & upgrades of Cluster API + providers
- Integration with cloud provider VM scaling groups (MachinePools)
- Machine failure remediation
- Provider end to end testing framework

## v0.4 (v1alpha4) ~ July 2020

- [Machine bootstrap failure detection](https://github.com/kubernetes-sigs/cluster-api-provider-aws/issues/972)
- [Improved status conditions](https://github.com/kubernetes-sigs/cluster-api/issues/1658)
- [Machine load balancers](https://github.com/kubernetes-sigs/cluster-api/issues/1250)
- [Extensible machine pre-delete hooks](https://github.com/kubernetes-sigs/cluster-api/issues/1514)

## v0.5 (v1???) ~ November 2020

- ?

## TBD

- [Pluggable MachineDeployment upgrade strategies](https://github.com/kubernetes-sigs/cluster-api/issues/1754)
- Dual stack IPv4/IPv6 support
- Windows nodes
- Auto-scaling support (either with cluster-autoscaler, or something else)
- UEFI booting
- GPU support (BitFusion, Elastic GPU, etc.)
- SR-IOV support
- FIPS/NIST/STIG compliance
- [Simplified cluster creation experience](https://github.com/kubernetes-sigs/cluster-api/issues/1227)
- Solid story on certificate management
- [Node attestation](https://github.com/kubernetes-sigs/cluster-api/issues/1739)
- Desired machine states ([running/stopped](https://github.com/kubernetes-sigs/cluster-api/issues/1810) / [restart](https://github.com/kubernetes-sigs/cluster-api/issues/1808))
- Define Cluster API conformance
