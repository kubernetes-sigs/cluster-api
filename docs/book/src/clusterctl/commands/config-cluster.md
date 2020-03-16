# clusterctl config cluster

The `clusterctl config cluster` command returns a YAML template for creating a workload cluster.

For example

```
clusterctl config cluster my-cluster --kubernetes-version v1.16.3 --control-plane-machine-count=3 --worker-machine-count=3 > my-cluster.yaml
```

Creates a YAML file named `my-cluster.yaml` with a predefined list of Cluster API objects; Cluster, Machines,
Machine Deployments, etc. to be deployed in the current namespace (in case, use the `--target-namespace` flag to
specify a different target namespace).

Then, the file can be modified using your editor of choice; when ready, run the following command
to apply the cluster manifest.

```
kubectl apply -f my-cluster.yaml
```

### Selecting the infrastructure provider to use

The `clusterctl config cluster` command uses smart defaults in order to simplify the user experience; in the example above,
it detects that there is only an `aws` infrastructure provider in the current management cluster and so it automatically
selects a cluster template from the `aws` provider's repository.

In case there is more than one infrastructure provider, the following syntax can be used to select which infrastructure
provider to use for the workload cluster:

```
clusterctl config cluster my-cluster --kubernetes-version v1.16.3 \
    --infrastructure:aws > my-cluster.yaml
```

or

```
clusterctl config cluster my-cluster --kubernetes-version v1.16.3 \
    --infrastructure:aws:v0.4.1 > my-cluster.yaml
```

### Flavors

The infrastructure provider authors can provide different type of cluster templates, or flavors; use the `--flavor` flag
to specify which flavor to use; e.g.

```
clusterctl config cluster my-cluster --kubernetes-version v1.16.3 \
    --flavor high-availability > my-cluster.yaml
```

Please refer to the providers documentation for more info about available flavors.

### Alternative source for cluster templates

clusterctl uses the provider's repository as a primary source for cluster templates; the following alternative sources
for cluster templates can be used as well:

#### ConfigMaps

Use the `--from-config-map` flag to read cluster templates stored in a Kubernetes ConfigMap; e.g.

```
clusterctl config cluster my-cluster --kubernetes-version v1.16.3 \
    --from-config-map my-templates > my-cluster.yaml
```

Also following flags are available `--from-config-map-namespace` (defaults to current namespace) and `--from-config-map-key`
(defaults to `template`).

#### GitHub or local file system folder

Use the `--from` flag to read cluster templates stored in a GitHub repository or in a local file system folder; e.g.

```
clusterctl config cluster my-cluster --kubernetes-version v1.16.3 \
   --from https://github.com/my-org/my-repository/blob/master/my-template.yaml > my-cluster.yaml
```

or

```
clusterctl config cluster my-cluster --kubernetes-version v1.16.3 \
   --from ~/my-template.yaml > my-cluster.yaml
```

### Variables

If the selected cluster template expects some environment variables, user should ensure those variables are set in advance.

e.g. if the `AWS_CREDENTIALS` variable is expected for a cluster template targeting the `aws` infrastructure, you
should ensure the corresponding environment variable to be set before executing `clusterctl config cluster`.

Please refer to the providers documentation for more info about the required variables or use the
`clusterctl config cluster --list-variables` flag to get a list of variables names required by a cluster template.

The [clusterctl configuration](./../configuration.md) file can be used as alternative to environment variables.
