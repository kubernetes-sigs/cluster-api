# Openstack Example Files
## Contents
*.yaml files - concrete example files that can be used as is.
*.yaml.template files - template example files that need values filled in before use.

## Generation
For convenience, a generation script which populates templates based on openstack cloud provider
configuration is provided.

1. Run the generation script.
```
./generate-yaml.sh
```

If yaml file already exists, you will see an error like the one below:

```
$ ./generate-yaml.sh
File provider-components.yaml already exists. Delete it manually before running this script.
```

## Manual Modification
You may always manually curate files based on the examples provided.

