1. there is a mandatory unique field

2. add mandatory unique field (user)

3. add optional field with default value (that the webhook has to make unique) (system)


Options:

1. Optional+Default+Webhook on DockerCluster & DockerClusterTemplate
   => We can't random default on DockerClusterTemplate as we get other UUIDs every time we apply the DockerClusterTemplate,
   because in the local YAML uuid is not set and the default webhook generates another UUID.
2. Optional+Default+Webhook on DockerCluster
   => We see an infinite append operation on subnets in DockerCluster. Reason is probably because in managedFields we only see ownership for the "dummy" value. The dummy value only exists after OpenAPI schema defaulting and before our defaulting webhook. tl;dr we're infinite appending orphaned subnets.
3. mandatory DockerClusterTemplate, Optional+Default+Webhook DockerCluster
   => Works for ClusterClass (as the uuid for the DockerCluster is always set)
   => Similar problems for the non-ClusterClass case as in 2.
4. mandatory DockerClusterTemplate, mandatory DockerCluster
   => Works but breaking

