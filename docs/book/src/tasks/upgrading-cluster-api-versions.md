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


<!-- links -->
[components]: ../reference/glossary.md#provider-components
[management cluster]: ../reference/glossary.md#management-cluster
[Cluster API v1alpha3 compared to v1alpha4 section]: ../developer/providers/migrations/v0.3-to-v0.4.md
[Cluster API v1alpha4 compared to v1beta1 section]: ../developer/providers/migrations/v0.4-to-v1.0.md
