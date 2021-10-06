# Upgrading Cluster API components

## When to upgrade

In general, it's recommended to upgrade to the latest version of Cluster API to take advantage of bug fixes, new
features and improvements.

## Considerations

If moving between different API versions, there may be additional tasks that you need to complete. See below for
detailed instructions.

Ensure that the version of Cluster API is compatible with the Kubernetes version of the management cluster.

## Upgrading to newer versions of 1.0.x

Use [clusterctl to upgrade between versions of Cluster API 1.0.x](../clusterctl/commands/upgrade.md).

## Upgrading from Cluster API v1alpha3 (0.3.x) to Cluster API v1beta1 (1.0.x)

For detailed information about the changes from `v1alpha3` to `v1beta1`, please refer to the [Cluster API v1alpha3 compared to v1alpha4 section] and the [Cluster API v1alpha4 compared to v1beta1 section].

Use [clusterctl to upgrade from Cluster API v0.3.x to Cluster API 1.0.x](../clusterctl/commands/upgrade.md).

You should now be able to manage your resources using the `v1beta1` version of the Cluster API components.

## Upgrading from Cluster API v1alpha4 (0.4.x) to Cluster API v1beta1 (1.0.x)

For detailed information about the changes from `v1alpha4` to `v1beta1`, please refer to the [Cluster API v1alpha4 compared to v1beta1 section].

Use [clusterctl to upgrade from Cluster API v0.4.x to Cluster API 1.0.x](../clusterctl/commands/upgrade.md).

You should now be able to manage your resources using the `v1alpha4` version of the Cluster API components.

<!-- links -->
[components]: ../reference/glossary.md#provider-components
[management cluster]: ../reference/glossary.md#management-cluster
[Cluster API v1alpha3 compared to v1alpha4 section]: ../developer/providers/v1alpha3-to-v1alpha4.md
[Cluster API v1alpha4 compared to v1beta1 section]: ../developer/providers/v1alpha4-to-v1beta1.md
