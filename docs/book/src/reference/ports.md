# Ports used by Cluster API

Name      | Port Number | Description |
---       | ---         | ---
`diagnostics-address` |             | Port that exposes the metrics, the pprof endpoint and an endpoint to change the log level. This can be customized by setting the `--diagnostics-address` flag when starting the manager. The default port is  `8443`.
`webhook` | `9443`      | Webhook server port. To disable this set `--webhook-port` flag to `0`.
`health`  | `9440`      | Port that exposes the health endpoint. CThis can be customized by setting the `--health-addr` flag when starting the manager.
`profiler`|             | Expose the pprof profiler. By default is not configured. Can set the `--profiler-address` flag. e.g. `--profiler-address 6060`

> Note: external providers (e.g. infrastructure, bootstrap, or control-plane) might allocate ports differently, please refer to the respective documentation.
