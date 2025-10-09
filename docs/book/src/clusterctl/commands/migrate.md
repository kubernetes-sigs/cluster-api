# clusterctl migrate

**Warning**: This command is EXPERIMENTAL and may be removed in a future release!

The `clusterctl migrate` command converts cluster.x-k8s.io resources between API versions.

## Usage

```bash
clusterctl migrate [SOURCE] [flags]
```

## Examples

```bash
# Migrate from file to stdout
clusterctl migrate cluster.yaml

# Migrate from stdin to stdout
cat cluster.yaml | clusterctl migrate

# Save output to a file
clusterctl migrate cluster.yaml --output migrated-cluster.yaml

# Explicitly specify target version
clusterctl migrate cluster.yaml --to-version v1beta2 --output migrated-cluster.yaml
```

## Flags

- `--output, -o`: Output file path (default: stdout)
- `--to-version`: Target API version for migration (default: "v1beta2")

## Scope and Limitations

- **Only cluster.x-k8s.io resources are converted** - Core CAPI resources like Cluster, MachineDeployment, Machine, etc.
- **Other CAPI API groups are passed through unchanged** - Infrastructure, bootstrap, and control plane provider resources are not converted
- **ClusterClass patches are not migrated** - Manual intervention required for ClusterClass patch conversions
- **Field order may change** - YAML field ordering is not preserved in the output
- **Comments are removed** - YAML comments are stripped during conversion
- **API version references are dropped** - Except for ClusterClass and external remediation references
