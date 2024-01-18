# prowjob-gen

Prowjob-gen is a tool which helps generating prowjob configuration.

## Usage

Flags:

```txt
  -config string
        Path to the config file
  -output-dir string
        Path to the directory to create the files in
  -templates-dir string
        Path to the directory containing the template files referenced inside the config file
```

When running prowjob-gen, all flags need to be provided.
The tool then will iterate over all templates defined in the config file and execute them per configured branch.

The configuration file is supposed to be in yaml format and to be stored inside the [test-infra](https://github.com/kubernetes/test-infra)
repository, we have to make sure it is not getting parsed as configuration for prow.
Because of that the top-level key for the configuration file is `prow-ignored:`.

A sample configuration looks as follows:

<!-- test/test-configuration.yaml -->
```yaml
prow_ignored:
  branches:
    main: # values below the branch here are available in the template 
      testImage: "gcr.io/k8s-staging-test-infra/kubekins-e2e:v20231208-8b9fd88e88-1.29"
      interval: "2h"
      upgradesInterval: "2h"
      kubernetesVersionManagement: "v1.26.6@sha256:6e2d8b28a5b601defe327b98bd1c2d1930b49e5d8c512e1895099e4504007adb"
      kubebuilderEnvtestKubernetesVersion: "1.26.1"
      upgrades:
      - from: "1.29"
        to: "1.30"

  templates:
  - name: "test.yaml.tpl"
    template: "test-{{ .branch }}.yaml.tmp"

  versions:
    "1.29":
      etcd: "3.5.10-0"
      coreDNS: "v1.11.1"
      k8sRelease: "stable-1.29"
    "1.30":
      etcd: "3.5.10-0"
      coreDNS: "v1.11.1"
      k8sRelease: "ci/latest-1.30"
```
<!-- test/test-configuration.yaml -->

With this configuration, the template `cluster-api-periodics.yaml.tpl` would get executed for each branch.
In this example we only configure the `main` branch which results in the output file `cluster-api-periodics-main.yaml`.

When executing a template, the following functions are available as addition to the standard functions in go templates:

- `TrimPrefix`: [strings.TrimPrefix](https://pkg.go.dev/strings#TrimPrefix)
- `TrimSuffix`: [strings.TrimSuffix](https://pkg.go.dev/strings#TrimSuffix)
- `ReplaceAll`: [strings.ReplaceAll](https://pkg.go.dev/strings#ReplaceAll)
- `last`: `func(any) any`: returns the last element of an array or slice.

When executing a template, the following variables are available:

- `branch`: The branch name the file gets templated for (The key in `.prow_ignored.branches`).
- `config`: The branch's configuration from `.prow_ignored.branches.<branch>`.
- `versions`: The versions mapper from `.prow_ignored.versions`.
