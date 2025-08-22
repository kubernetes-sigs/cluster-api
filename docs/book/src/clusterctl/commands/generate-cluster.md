# clusterctl generate cluster

The `clusterctl generate cluster` command returns a YAML template for creating a workload cluster.

For example

```bash
clusterctl generate cluster my-cluster --kubernetes-version v1.28.0 --control-plane-machine-count=3 --worker-machine-count=3 > my-cluster.yaml
```

Generates a YAML file named `my-cluster.yaml` with a predefined list of Cluster API objects; Cluster, Machines,
Machine Deployments, etc. to be deployed in the current namespace (in case, use the `--target-namespace` flag to
specify a different target namespace).

Then, the file can be modified using your editor of choice; when ready, run the following command
to apply the cluster manifest.

```bash
kubectl apply -f my-cluster.yaml
```

### Selecting the infrastructure provider to use

The `clusterctl generate cluster` command uses smart defaults in order to simplify the user experience; in the example above,
it detects that there is only an `aws` infrastructure provider in the current management cluster and so it automatically
selects a cluster template from the `aws` provider's repository.

In case there is more than one infrastructure provider, the following syntax can be used to select which infrastructure
provider to use for the workload cluster:

```bash
clusterctl generate cluster my-cluster --kubernetes-version v1.28.0 \
    --infrastructure aws > my-cluster.yaml
```

or

```bash
clusterctl generate cluster my-cluster --kubernetes-version v1.28.0 \
    --infrastructure aws:v0.4.1 > my-cluster.yaml
```

### Flavors

The infrastructure provider authors can provide different types of cluster templates, or flavors; use the `--flavor` flag
to specify which flavor to use; e.g.

```bash
clusterctl generate cluster my-cluster --kubernetes-version v1.28.0 \
    --flavor high-availability > my-cluster.yaml
```

Please refer to the providers documentation for more info about available flavors.

### Alternative source for cluster templates

clusterctl uses the provider's repository as a primary source for cluster templates; the following alternative sources
for cluster templates can be used as well:

#### ConfigMaps

Use the `--from-config-map` flag to read cluster templates stored in a Kubernetes ConfigMap; e.g.

```bash
clusterctl generate cluster my-cluster --kubernetes-version v1.28.0 \
    --from-config-map my-templates > my-cluster.yaml
```

Also following flags are available `--from-config-map-namespace` (defaults to current namespace) and `--from-config-map-key`
(defaults to `template`).

#### GitHub, raw template URL, local file system folder or standard input

Use the `--from` flag to read cluster templates stored in a GitHub repository, raw template URL, in a local file system folder,
or from the standard input; e.g.

```bash
clusterctl generate cluster my-cluster --kubernetes-version v1.28.0 \
   --from https://github.com/my-org/my-repository/blob/main/my-template.yaml > my-cluster.yaml
```

or

```bash
clusterctl generate cluster my-cluster --kubernetes-version v1.28.0 \
   --from https://foo.bar/my-template.yaml > my-cluster.yaml
```

or

```bash
clusterctl generate cluster my-cluster --kubernetes-version v1.28.0 \
   --from ~/my-template.yaml > my-cluster.yaml
```

or

```bash
cat ~/my-template.yaml | clusterctl generate cluster my-cluster --kubernetes-version v1.28.0 \
    --from - > my-cluster.yaml
```

### Variables

If the selected cluster template expects some environment variables, the user should ensure those variables are set in advance.

E.g. if the `AWS_CREDENTIALS` variable is expected for a cluster template targeting the `aws` infrastructure, you
should ensure the corresponding environment variable to be set before executing `clusterctl generate cluster`.

Please refer to the providers documentation for more info about the required variables or use the
`clusterctl generate cluster --list-variables` flag to get a list of variables names required by a cluster template.

The [clusterctl configuration](./../configuration.md) file can be used as alternative to environment variables.
