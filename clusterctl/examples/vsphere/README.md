# Vsphere Example Files
## Contents
* *.yaml files - concrete example files that can be used as is.
* *.yaml.template files - template example files that need values filled in before use.

## Generation
For convenience, a generation script which populates templates where possible.

1. Run the generation script. This wil produce ```provider-components.yaml```
```
./generate-yaml.sh
```
2. Copy machines.yaml.template to machines.yaml and
Manually edit ```terraformVariables``` for machines in machines.yaml
```
cp machines.yaml.template machines.yaml
```

3. Copy cluster.yaml.template to cluster.yaml and
Manually edit ```providerConfig``` for the cluster in cluster.yaml
```
cp cluster.yaml.template cluster.yaml
```

## Manual Modification
You may always manually curate files based on the examples provided.

