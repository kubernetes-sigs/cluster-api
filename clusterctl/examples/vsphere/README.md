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
2. To define the master machine, copy `machines.yaml.template` to `machines.yaml` and
manually edit `machineVariables`.

3. To define nodes, copy `machineset.yaml.template` to `machineset.yaml` and
manually edit `machineVariables`. If needed, adjust `replicas` as well.

4. Copy `cluster.yaml.template` to `cluster.yaml` and
manually edit `providerConfig`.

5. *Optional*: Copy `addons.yaml.template` to `addons.yaml` and
+manually edit `parameters`.

## Manual Modification
You may always manually curate files based on the examples provided.

