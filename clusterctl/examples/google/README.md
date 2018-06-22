# Google Example Files
## Contents
*.yaml files - concrete example files that can be used as is.
*.yaml.template files - template example files that need values filled in before use.

## Generation
For convenience, a generation script which populates templates based on gcloud 
configuration is provided.

1. Run the generation script.
```
./generate-yaml.sh
```

If gcloud isn't configured, you will see an error like the one below:

```
$ ./generate-yaml.sh
ERROR: (gcloud.config.get-value) Section [core] has no property [project].
```

## Manual Modification
You may always manually curate files based on the examples provided.

## Setup Environment
In order to run `clusterctl create`, setup the GOOGLE_APPLICATION_CREDENTIALS environment
variable:

```
export GOOGLE_APPLICATION_CREDENTIALS="$(pwd)/out/machine-controller-serviceaccount.json"
```
