# `clusterctl`

## clusterctl v1alpha3 (clusterctl redesign)

`clusterctl` is a CLI tool for handling the lifecycle of a cluster-API management cluster.

The v1alpha3 release is designed for providing a simple day 1 experience; `clusterctl` is bundled with Cluster API and can be reused across providers
that are compliant with the following rules.

### Components YAML

The provider is required to generate a single YAML file with all the components required for installing the provider
itself (CRD, Controller, RBAC etc.). 

Infrastructure providers MUST release a file called `infrastructure-components.yaml`, while bootstrap provider MUST
release a file called ` bootstrap-components.yaml` (exception for CABPK, which is included in CAPI by default).

The components YAML should contain one Namespace object, which will be used as the default target namespace
when creating the provider components.

> If the generated component YAML does't contain a Namespace object, user will need to provide one to `clusterctl init` using
> the `--target-namespace` flag.

> In case there is more than one Namespace object in the components YAML, `clusterctl` will generate an error and abort
> the provider installation.

The components YAML can contain environment variables matching the regexp `\${\s*([A-Z0-9_]+)\s*}`; it is highly
recommended to prefix the variable name with the provider name e.g. `{ $AWS_CREDENTIALS }`

> Users are required to ensure that environment variables are set in advance before running `clusterctl init`; if a variable
> is missing, `clusterctl` will generate an error and abort the provider installation.
 
### Workload cluster templates

Infrastructure provider could publish cluster templates to be used by `clusterctl config cluster`.

Cluster templates MUST be stored in the same folder of the component YAML and adhere to the the following naming convention:
1. The default cluster template should be named `config-{bootstrap}.yaml`. e.g `config-kubeadm.yaml`
2. Additional cluster template should be named `config-{flavor}-{bootstrap}.yaml`. e.g `config-production-kubeadm.yaml`

`{bootstrap}` is the name of the bootstrap provider used in the template; `{flavor}` is the name the user can pass to the
`clusterctl config cluster --flavor` flag to identify the specific template to use.

## Previous versions (unsupported) 

### v1alpha1

`clusterctl` was a command line tool packaged with v1alpha1 providers. The goal of this tool was to go from nothing to a
running management cluster in whatever environment the provider was built for. For example, Cluster-API-Provider-AWS
packaged a `clusterctl` that created a Kubernetes cluster in EC2 and installed the necessary controllers to respond to
Cluster API's APIs.

### v1alpha2

`clusterctl` was likely becoming provider-agnostic meaning one clusterctl was bundled with Cluster API and can be reused
across providers. Work here is still being figured out but providers will not be packaging their own `clusterctl`
anymore.
