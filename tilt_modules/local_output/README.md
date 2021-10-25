# local_output

Author: [Çağatay Yücelen](https://github.com/cyucelen)

Get the output of a shell command as string to use it in Tiltfile.

`local_output` runs given command and strips trailing `\n`.

## Usage

Pass a command to execute with local.

### Example:

```python
desired_memory_capacity = "4096"
current_memory_capacity = local_output('minikube config get memory') # e.g. "2048"

if int(current_memory_capacity) < int(desired_memory_capacity):
    local('minikube config set memory {}'.format(desired_memory_capacity))
    local('minikube delete')
    local('minikube start')
```

### Arguments:

- `command`: shell command to execute with `local` function
