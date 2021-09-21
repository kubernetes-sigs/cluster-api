# clusterctl generate yaml

The `clusterctl generate yaml` command processes yaml using clusterctl's yaml
processor.

The intent of this command is to allow users who may have specific templates
to leverage clusterctl's yaml processor for variable substitution. For
example, this command can be leveraged in local and CI scripts or for
development purposes.

clusterctl ships with a simple yaml processor that performs variable
substitution that takes into account default values.
Under the hood, clusterctl's yaml processor uses
[drone/envsubst][drone-envsubst] to replace variables and uses the defaults if
necessary.

Variable values are either sourced from the clusterctl config file or
from environment variables.

Current usage of the command is as follows:
```bash
# Generates a configuration file with variable values using a template from a
# specific URL.
clusterctl generate yaml --from https://github.com/foo-org/foo-repository/blob/main/cluster-template.yaml

# Generates a configuration file with variable values using
# a template stored locally.
clusterctl generate yaml  --from ~/workspace/cluster-template.yaml

# Prints list of variables used in the local template
clusterctl generate yaml --from ~/workspace/cluster-template.yaml --list-variables

# Prints list of variables from template passed in via stdin
cat ~/workspace/cluster-template.yaml | clusterctl generate yaml --from - --list-variables

# Default behavior for this sub-command is to read from stdin.
# Generate configuration from stdin
cat ~/workspace/cluster-template.yaml | clusterctl generate yaml
```

<!-- Links -->
[drone-envsubst]: https://github.com/drone/envsubst
