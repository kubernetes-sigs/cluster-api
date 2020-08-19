# exp

This subrepository holds experimental code and API types.

**Warning**: Packages here are experimental and unreliable. Some may one day be promoted to the main repository, or they may be modified arbitrarily or even disappear altogether.

In short, code in this subrepository is not subject to any compatibility or deprecation promise.

Experiments follow a strict lifecycle: Alpha -> Beta prior to Graduation.

For more information on graduation criteria, see: [Contributing Guidelines](../CONTRIBUTING.md#experiments)

## Active Features
 DockerMachinePool  (alpha)

## Create a new Resource
Below is an example of creating a `DockerMachinePool` resource in the experimental group.
```
kubebuilder create api --kind DockerMachinePool --group exp.infrastructure --version v1alpha3 \
  --controller=true --resource=true --make=false
```