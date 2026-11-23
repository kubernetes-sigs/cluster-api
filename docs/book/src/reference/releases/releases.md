# Cluster API releases

This page lists the Cluster API releases currently under support; see [Support matrix](../versions.md#cluster-api-release-support)
for the rules defining how a release is supported.

| Minor Release | First release (.0)                                                                                                          | Latest patch release                                                                                                        | Migration notes                                  | Status                  | Supported Until (including maintenance mode)                                                |
|---------------|-----------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------|-------------------------|---------------------------------------------------------------------------------------------|
| v1.15.x       | [v1.15.0](https://github.com/kubernetes-sigs/cluster-api/releases?q=tag%3Av1.15.0) | [Latest patch](https://github.com/kubernetes-sigs/cluster-api/releases?q=tag:v1.15+prerelease:false) | [v1.14 to v1.15](./migrations/v1.14-to-v1.15.md) | Standard support period | in maintenance mode when v1.17.0 will be released, EOL when v1.18.0 will be released        |
| v1.14.x       | [v1.14.0](https://github.com/kubernetes-sigs/cluster-api/releases?q=tag%3Av1.14.0) | [Latest patch](https://github.com/kubernetes-sigs/cluster-api/releases?q=tag:v1.14+prerelease:false) | [v1.13 to v1.14](./migrations/v1.13-to-v1.14.md) | Standard support period | in maintenance mode when v1.16.0 will be released, EOL when v1.17.0 will be released        |
| v1.13.x       | [v1.13.0](https://github.com/kubernetes-sigs/cluster-api/releases?q=tag%3Av1.13.0) | [Latest patch](https://github.com/kubernetes-sigs/cluster-api/releases?q=tag:v1.13+prerelease:false) | [v1.12 to v1.13](./migrations/v1.12-to-v1.13.md) | Maintenance mode        | Maintenance mode since 2026-12-08 - v1.15.0 release date, EOL when v1.16.0 will be released |

## End of life releases

| Minor Release | Status | End of life                                 |
|---------------|--------|---------------------------------------------|
| v1.12.x       | EOL    | EOL since 2026-12-08 - v1.15.0 release date |
| v1.11.x       | EOL    | EOL since 2026-08-18 - v1.14.0 release date |
| v1.10.x       | EOL    | EOL since 2026-04-21 - v1.13.0 release date |

