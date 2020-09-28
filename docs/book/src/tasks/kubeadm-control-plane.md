# Kubeadm control plane

Using the Kubeadm control plane type to manage a control plane provides several ways to upgrade control plane machines.

### Kubeconfig management

KCP will generate and manage the admin Kubeconfig for clusters. The client certificate for the admin user is created
with a valid lifespan of a year, and will be automatically regenerated when the cluster is reconciled and has less than
6 months of validity remaining.

### Upgrades

See the section on [upgrading clusters][upgrades].

#### Using Kubeadm Control Plane when upgrading from Cluster API v1alpha2 (0.2.x)

See the section on [Adopting existing machines into KubeadmControlPlane management][adoption]


<!-- links -->
[adoption]: upgrading-cluster-api-versions.md#adopting-existing-machines-into-kubeadmcontrolplane-management
[upgrades]: upgrading-clusters.md#how-to-upgrade-the-kubernetes-control-plane-version
