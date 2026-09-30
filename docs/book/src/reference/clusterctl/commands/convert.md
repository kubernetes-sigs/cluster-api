# clusterctl convert

**Warning**: This command is EXPERIMENTAL and may be removed in a future release!

The `clusterctl convert` command converts Cluster API resources between API versions.

## Usage

```bash
clusterctl convert [SOURCE] [flags]
```

## Examples

```bash
# Convert from file to stdout
clusterctl convert cluster.yaml

# Convert from stdin to stdout
cat cluster.yaml | clusterctl convert

# Save output to a file
clusterctl convert cluster.yaml --output converted-cluster.yaml

# Explicitly specify target version
clusterctl convert cluster.yaml --to-version v1beta2 --output converted-cluster.yaml
```

## Flags

- `--output, -o`: Output file path (default: stdout)
- `--to-version`: Target API version for conversion (default: "v1beta2")

## Scope and Limitations

- **Only cluster.x-k8s.io resources are converted** - Core CAPI resources like Cluster, MachineDeployment, Machine, etc.
- **Other CAPI API groups are passed through unchanged** - Infrastructure, bootstrap, and control plane provider resources are not converted
- **ClusterClass patches are not converted** - Manual intervention required for ClusterClass patch conversions
- **Field order may change** - YAML field ordering is not preserved in the output
- **Comments are removed** - YAML comments are stripped during conversion
- **API version references are dropped** - Except for ClusterClass and external remediation references
