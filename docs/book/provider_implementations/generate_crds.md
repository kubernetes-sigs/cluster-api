
{% method %}
### Create a repository

{% sample lang="bash" %}
```bash
mkdir ${GOPATH}/src/sigs.k8s.io/cluster-api-provider-solas
cd ${GOPATH}/src/sigs.k8s.io/cluster-api-provider-solas
git init
```
{% endmethod %}

{% method %}
### Generate scaffolding

`kubebuilder init` will create the basic repository layout, including a
simple containerized manager. It will also initialize the vendored go libraries
that will be required to build your project.

When asked:

```
Run `dep ensure` to fetch dependencies (Recommended) [y/n]?
y
```

you should.

{% sample lang="bash" %}
```bash
kubebuilder init --domain cluster.k8s.io --license apache2 --owner "The Kubernetes Authors"
```
```bash
git add .
git commit -m "Generate scaffolding."
```
{% endmethod %}

{% method %}
### Generate provider resources for Clusters and Machines

Here you will be asked if you want to generate resources and controllers.
You only want to generate the resources because the controllers are defined
in the common Cluster API code. Once we define Actuators in the next section
we will register them with the existing Cluster API controllers.

```
Create Resource under pkg/apis [y/n]?
y
Create Controller under pkg/controller [y/n]?
n
```

{% sample lang="bash" %}
```bash
kubebuilder create api --group solas --version v1alpha1 --kind SolasClusterProviderSpec
kubebuilder create api --group solas --version v1alpha1 --kind SolasClusterProviderStatus
```
```bash
kubebuilder create api --group solas --version v1alpha1 --kind SolasMachineProviderSpec
kubebuilder create api --group solas --version v1alpha1 --kind SolasMachineProviderStatus
```

```bash
git add .
git commit -m "Generate Cluster and Machine resources."
```
{% endmethod %}
