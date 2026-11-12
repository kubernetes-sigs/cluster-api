# Version migration

The following pages provide an overview of relevant changes between versions of Cluster API and their direct successors. These guides are intended to assist 
maintainers of other providers and consumers of the Go API in upgrading from one version of Cluster API to a subsequent version.

For older versions please refer to [Older Cluster API documentation versions](../../../introduction.md#getting-started)

## Writing a migration doc

Changes that affect providers or consumers of the Go API are documented in the migration doc for the next release
(`vX-to-vY.md`), preferably in the same PR that introduces them. When reviewing the doc before a release:

- Build "API Changes" from `git diff vX.0 main -- api/`, considering only the `*_types.go` files of the current API version.
- Build "Runtime hooks Changes" from `api/runtime/hooks`.
- Build "Contract changes" from the [contract pages](../contracts/overview.md) and `internal/contract`.
- Skip internal annotations, changes to comments only, and changes already backported to vX.y patch releases.
