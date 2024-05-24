# clusterctl Extensions with Plugins

You can extend `clusterctl` with plugins, similar to `kubectl`. Please refer to the [kubectl plugin documentation](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/) for more information, 
as `clusterctl` plugins are implemented in the same way, with the exception of plugin distribution.

## Installing clusterctl plugins

To install a clusterctl plugin, place the plugin's executable file in any location on your `PATH`.

## Writing clusterctl plugins

No plugin installation or pre-loading is required. Plugin executables inherit the environment from the `clusterctl` binary. A plugin determines the command it implements based on its name. 
For example, a plugin named `clusterctl-foo` provides the `clusterctl` foo command. The plugin executable should be installed in your `PATH`.

Example plugin

```bash
#!/bin/bash

# optional argument handling
if [[ "$1" == "version" ]]
then
echo "1.0.0"
exit 0
fi

# optional argument handling
if [[ "$1" == "example-env-var" ]]
then
    echo "$EXAMPLE_ENV_VAR"
    exit 0
fi

echo "I am a plugin named clusterctl-foo"
```

### Using a plugin
To use a plugin, make the plugin executable:

```bash
sudo chmod +x ./clusterctl-foo
```

and place it anywhere in your `PATH`:

```bash
sudo mv ./clusterctl-foo /usr/local/bin
```

You may now invoke your plugin as a `clusterctl` command:

```bash
clusterctl foo
```

```
I am a plugin named clusterctl-foo
```

All args and flags are passed as-is to the executable:
```bash
clusterctl foo version
```

```
1.0.0
```

All environment variables are also passed as-is to the executable:

```bash
export EXAMPLE_ENV_VAR=example-value
clusterctl foo example-env-var
```

```
example-value
```

```bash
EXAMPLE_ENV_VAR=another-example-value clusterctl foo example-env-var
```

```
another-example-value
```

Additionally, the first argument that is passed to a plugin will always be the full path to the location where it was invoked ($0 would equal /usr/local/bin/clusterctl-foo in the example above).

## Naming a plugin

A plugin determines the command path it implements based on its filename. Each sub-command in the path is separated by a dash (-). For example, a plugin for the command `clusterctl foo bar baz` would have the filename `clusterctl-foo-bar-baz`.
