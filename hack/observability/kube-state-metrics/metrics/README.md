# Metrics

**Disclamer**: This is a temporary workaround. The long-term goal is to generate metric configuration from API type markers.

The make target `generate-metrics-config` is used to generate a single file which contains the Cluster API specific custom resource configuration for kube-state-metrics.

To regenerate the file `../crd-config.yaml`, execute the `make generate-metrics-config` command.
