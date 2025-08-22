# Running multiple providers

Cluster API supports running multiple infrastructure/bootstrap/control plane providers on the same management cluster. It's highly recommended to rely on
[clusterctl init](../clusterctl/commands/init.md) command in this case. [clusterctl](../clusterctl/overview.md) will help ensure that all providers support the same
[API Version of Cluster API](../developer/providers/contracts/clusterctl.md#metadata-yaml) (contract).

<aside class="note warning">

<h1>Warning</h1>

Currently, the case of running multiple providers is not covered in Cluster API E2E test suite. It's recommended to set up a custom validation pipeline of
your specific combination of providers before deploying it on a production environment.

</aside>
