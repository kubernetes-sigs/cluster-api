# clusterctl generate provider

Generate templates for provider components.

clusterctl fetches the provider components from the provider repository and performs variable substitution.

Variable values are either sourced from the clusterctl config file or
from environment variables

Usage: `clusterctl generate provider [flags]`

Current usage of the command is as follows:
```bash
# Generates a yaml file for creating provider with variable values using
# components defined in the provider repository.
clusterctl generate provider --infrastructure aws

# Generates a yaml file for creating provider for a specific version with variable values using
# components defined in the provider repository.
clusterctl generate provider --infrastructure aws:v0.4.1

# Displays information about a specific infrastructure provider.
# If applicable, prints out the list of required environment variables.
clusterctl generate provider --infrastructure aws --describe

# Displays information about a specific version of the infrastructure provider.
clusterctl generate provider --infrastructure aws:v0.4.1 --describe

# Generates a yaml file for creating provider for a specific version.
# No variables will be processed and substituted using this flag
clusterctl generate provider --infrastructure aws:v0.4.1 --raw
```
