# Cluster API Version Support and Kubernetes Version Skew Policy

## Supported Versions

The Cluster API team maintains branches for **v1.x (v1beta1)**. For more details see [Support and guarantees](https://github.com/kubernetes-sigs/cluster-api/blob/main/CONTRIBUTING.md#support-and-guarantees).

Releases include these components:

- Core Provider
- Kubeadm Bootstrap Provider
- Kubeadm Control Plane Provider
- clusterctl client

All Infrastructure Providers are maintained by independent teams. Other Bootstrap and Control Plane Providers are also maintained by independent teams. For more information about their version support, see [below](#providers-maintained-by-independent-teams).

## Supported Kubernetes Versions

A Cluster API minor release supports (when it's initially created):
* 4 Kubernetes minor releases for the management cluster (N - N-3)
* 6 Kubernetes minor releases for the workload cluster (N - N-5)

When a new Kubernetes minor release is available, we will try to support it in an upcoming Cluster API patch release
(although only in the latest supported Cluster API minor release). But this depends on the changes made in Kubernetes, if
the corresponding required changes in Cluster API are too invasive we won't backport the support and users have to wait 
for the next Cluster API minor release.

For example, Cluster API v1.4.0 would support the following Kubernetes versions:
* v1.23.x to v1.26.x for the management cluster
* v1.21.x to v1.26.x for the workload cluster
* When Kubernetes 1.27 is released, it will be supported in v1.4.x (but not in v1.3.x)

Support in this context means that we:
* maintain corresponding code paths
* have test coverage
* accept bug fixes

Important! This is not a replacement/alternative for upstream Kubernetes support policies!
Support for versions of Kubernetes which itself are out of support is limited to "Cluster API can start a Cluster with this Kubernetes version"
and "Cluster API  can upgrade to the next Kubernetes version"; it does not include any extended support to Kubernetes itself.

Whenever a new Cluster API release is cut, we will document the Kubernetes version compatibility matrix the release
has been tested with. Summaries of Kubernetes versions supported by each component are additionally maintained in
the [tables](#release-components) below.

On a final comment, let's praise all the contributors keeping care of such a wide support matrix.
If someone is looking for opportunities to help with the project, this is definitely an area where additional hands
and eyes will be more than welcome and greatly beneficial to the entire community.

See the [following section](#kubernetes-version-support-as-a-function-of-cluster-topology) to understand how cluster topology affects version support.

### Kubernetes Version Support As A Function Of Cluster Topology

The Core Provider, Kubeadm Bootstrap Provider, and Kubeadm Control Plane Provider run on the Management Cluster, and clusterctl talks to that cluster's API server.

In some cases, the Management Cluster is separate from the Workload Clusters. The Kubernetes version of the Management and Workload Clusters are allowed to be different.

Management Clusters and Workload Clusters can be upgraded independently and in any order, however, if you are additionally moving from
v1alpha3 (v0.3.x) to v1beta1 (v1.x) as part of the upgrade rollout, the management cluster will need to be upgraded to at least v1.20.x,
prior to upgrading any workload cluster using Cluster API v1beta1 (v1.x)

These diagrams show the relationships between components in a Cluster API release (yellow), and other components (white).

#### Management And Workload Cluster Are the Same (Self-hosted)

![Management/Workload Same Cluster](../images/management-workload-same-cluster.png)

#### Management And Workload Clusters Are Separate

![Management/Workload Separate Clusters](../images/management-workload-separate-clusters.png)

### Release Components

#### Core Provider (`cluster-api-controller`)

|                   | v1.1 (v1beta1) (EOL) | v1.2 (v1beta1)    | v1.3 (v1beta1)    | v1.4 (v1beta1)    |
|-------------------|----------------------|-------------------|-------------------|-------------------|
| Kubernetes v1.18  | ✓ (only workload)    | ✓ (only workload) | ✓ (only workload) |                   |
| Kubernetes v1.19  | ✓                    | ✓ (only workload) | ✓ (only workload) |                   |
| Kubernetes v1.20  | ✓                    | ✓                 | ✓                 |                   |
| Kubernetes v1.21  | ✓                    | ✓                 | ✓                 | ✓ (only workload) |
| Kubernetes v1.22  | ✓                    | ✓                 | ✓                 | ✓ (only workload) |
| Kubernetes v1.23* | ✓                    | ✓                 | ✓                 | ✓                 |
| Kubernetes v1.24  | ✓                    | ✓                 | ✓                 | ✓                 |
| Kubernetes v1.25  |                      | ✓                 | ✓                 | ✓                 |
| Kubernetes v1.26  |                      | ✓                 | ✓                 | ✓                 |


\* There is an issue with CRDs in Kubernetes v1.23.{0-2}. ClusterClass with patches is affected by that (for more details please see [this issue](https://github.com/kubernetes-sigs/cluster-api/issues/5990)). Therefore we recommend to use Kubernetes v1.23.3+ with ClusterClass.
	 Previous Kubernetes **minor** versions are not affected.

\** When using CAPI v1.2 or v1.3 with the CLUSTER_TOPOLOGY experimental feature on, the Kubernetes Version for the management cluster must be >= 1.22.0.

The Core Provider also talks to API server of every Workload Cluster. Therefore, the Workload Cluster's Kubernetes version must also be compatible.

#### Kubeadm Bootstrap Provider (`kubeadm-bootstrap-controller`)

|                                    | v1.1 (v1beta1) (EOL) | v1.2 (v1beta1)    | v1.3 (v1beta1)    | v1.4 (v1beta1)     |
|------------------------------------|----------------------|-------------------|-------------------|--------------------|
| Kubernetes v1.18 + kubeadm/v1beta2 | ✓ (only workload)    | ✓ (only workload) | ✓ (only workload) |                    |
| Kubernetes v1.19 + kubeadm/v1beta2 | ✓                    | ✓ (only workload) | ✓ (only workload) |                    |
| Kubernetes v1.20 + kubeadm/v1beta2 | ✓                    | ✓                 | ✓                 |                    |
| Kubernetes v1.21 + kubeadm/v1beta2 | ✓                    | ✓                 | ✓                 | ✓  (only workload) |
| Kubernetes v1.22 + kubeadm/v1beta3 | ✓                    | ✓                 | ✓                 | ✓  (only workload) |
| Kubernetes v1.23 + kubeadm/v1beta3 | ✓                    | ✓                 | ✓                 | ✓                  |
| Kubernetes v1.24 + kubeadm/v1beta3 | ✓                    | ✓                 | ✓                 | ✓                  |
| Kubernetes v1.25 + kubeadm/v1beta3 |                      | ✓                 | ✓                 | ✓                  |
| Kubernetes v1.26 + kubeadm/v1beta3 |                      | ✓                 | ✓                 | ✓                  |

The Kubeadm Bootstrap Provider generates kubeadm configuration using the API version recommended for the target Kubernetes version.

#### Kubeadm Control Plane Provider (`kubeadm-control-plane-controller`)

|                            | v1.1 (v1beta1) (EOL) | v1.2 (v1beta1)    | v1.3 (v1beta1)    | v1.4 (v1beta1)    |
|----------------------------|----------------------|-------------------|-------------------|-------------------|
| Kubernetes v1.18 + etcd/v3 | ✓ (only workload)    | ✓ (only workload) | ✓ (only workload) |                   |
| Kubernetes v1.19 + etcd/v3 | ✓                    | ✓ (only workload) | ✓ (only workload) |                   |
| Kubernetes v1.20 + etcd/v3 | ✓                    | ✓                 | ✓                 |                   |
| Kubernetes v1.21 + etcd/v3 | ✓                    | ✓                 | ✓                 | ✓ (only workload) |
| Kubernetes v1.22 + etcd/v3 | ✓                    | ✓                 | ✓                 | ✓ (only workload) |
| Kubernetes v1.23 + etcd/v3 | ✓                    | ✓                 | ✓                 | ✓                 |
| Kubernetes v1.24 + etcd/v3 | ✓                    | ✓                 | ✓                 | ✓                 |
| Kubernetes v1.25 + etcd/v3 |                      | ✓                 | ✓                 | ✓                 |
| Kubernetes v1.26 + etcd/v3 |                      | ✓                 | ✓                 | ✓                 |

The Kubeadm Control Plane Provider talks to the API server and etcd members of every Workload Cluster whose control plane it owns. It uses the etcd v3 API.

The Kubeadm Control Plane requires the Kubeadm Bootstrap Provider.

\*  Newer versions of CoreDNS may not be compatible as an upgrade target for clusters managed with Cluster API. Kubernetes versions marked on the table are supported as an upgrade target only if CoreDNS is not upgraded to the latest version supported by the respective Kubernetes version. The versions supported are represented in the below table.

##### CoreDNS

| CAPI Version         | Max CoreDNS Version for Upgrade |
|----------------------|---------------------------------|
| v1.1 (v1beta1)       | v1.9.3                          |
| v1.2 (v1beta1)       | v1.9.3                          |
| >= v1.2.7 (v1beta1)  | v1.10.0                         |
| >= v1.2.11 (v1beta1) | v1.10.1                         |
| v1.3 (v1beta1)       | v1.10.0                         |
| >= v1.3.4 (v1beta1)  | v1.10.1                         |
| v1.4 (v1beta1)       | v1.10.1                         |

#### Kubernetes version specific notes

**1.26**:
* No specific notes

**1.25**:
* No specific notes

**1.24**:
* Kubeadm Bootstrap Provider:
		* `kubeadm` now sets both the `node-role.kubernetes.io/control-plane` and `node-role.kubernetes.io/master` taints on control plane nodes.
		* `kubeadm` now only sets the `node-role.kubernetes.io/control-plane` label on control plane nodes (the `node-role.kubernetes.io/master` label is not set anymore).
* Kubeadm Bootstrap Provider and Kubeadm Control Plane Provider
		* `criSocket` without a scheme prefix has been deprecated in the kubelet since a while. `kubeadm` now shows a warning if no scheme is present and eventually the support for `criSocket`'s without prefix will be dropped. Please adjust the `criSocket` accordingly (e.g. `unix:///var/run/containerd/containerd.sock`) if you are configuring the `criSocket` in CABPK or KCP resources.

#### clusterctl

It is strongly recommended to always use the latest version of [clusterctl](../clusterctl/overview.md), in order to get all the fixes/latest changes.

In case of upgrades, clusterctl should be upgraded first and then used to upgrade all the other components.

## Providers Maintained By Independent Teams

In general, if a Provider version M says it is compatible with Cluster API version N, then version M must be compatible with a subset of the Kubernetes versions supported by Cluster API version N.

To understand the version compatibility of a specific provider, please see its documentation. This book includes [a list of independent providers](providers.md)
