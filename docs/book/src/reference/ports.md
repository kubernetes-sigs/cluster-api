## Ports used by Cluster API

Name      | Port Number | Description |
---       | ---         | ---
`metrics` | `8080`      | Port that exposes the metrics. Can be customized, for that set the `--metrics-addr` flag when starting the manager.
`webhook` | `9443`      | Webhook server port. To disable this set `--webhook-port` flag to `0`.
`health`  | `9440`      | Port that exposes the heatlh endpoint. Can be customized, for that set the `--health-addr` flag when starting the manager.
`profiler`| ` `         | Expose the pprof profiler. By default is not configured. Can set the `--profiler-address` flag. e.g. `--profiler-address 6060`


> Note: external providers (e.g. infrastructure, bootstrap, or control-plane) might allocate ports differently, please refer to the respective documentation.
