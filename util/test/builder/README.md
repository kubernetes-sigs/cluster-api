# "Generic" custom resource builder

This package helps create custom resources definitions of "generic" Cluster API resources. For example, resources for a "generic" infra provider, such as InfraClusters, InfraMachines, InfraTemplates, etc. These resources are used for testing Cluster API controllers that reconcile resources created by different infrastructure, bootstrap, and control-plane providers.

## Compatibility notice

This package does not adhere to any compatibility guarantees. This package is meant to be used by tests only.
