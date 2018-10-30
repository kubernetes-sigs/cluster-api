# Repository Naming

The naming convention for new Cluster API [_provider_ repositories][repo-naming]
is generally of the form `cluster-api-provider-${cloud}`, where `${cloud}` is a,
possibly short, name for the cloud in question. For example `cluster-api-provider-gcp`
is an implementation for the Google Cloud Platform, and `cluster-api-provider-aws` 
is one for Amazon Web Services. Note that a single cloud provider may include 
more than one [_variant_][variant-naming]. So for example, `cluster-api-provider-aws` 
may include both an implementation based on EC2 as well as one based on their
hosted EKS solution. Finally, it is not uncommon for the word _cloud_ in this 
context to refer to even bare metal or virtual machine infrastructure, when the difference is not important.

{% panel style="info", title="Important" %}
For the purposes of this guide we will create a fictional provider for a 
cloud provider named **solas**. Therefore the name of the repository will be 
`cluster-api-provider-solas`.
{% endpanel %}

# Resource Naming

{% method %}

Every Kubernetes resource has a *Group*, *Version* and *Kind* that uniquely 
identifies it.

* The resource *Kind* is the short name of the API, for example, 
  `SolasMachineSpec` and `SolasMachineStatus` could be the provider specific
  API for `Machine` resources.
* The resource *Version* defines the stability of the API and its backward 
  compatibility guarantees. Examples include v1alpha1, v1beta1, v1, etc.
  and are governed by the Kubernetes API Deprecation Policy [^1].
* The resource *Group* is similar to package in a language.  It disambiguates 
  different APIs that may happen to have identically named *Kind*s.  Groups 
  often contain a domain name, such as k8s.io. The domain for Cluster API
  resources is `cluster.k8s.io`.

{% sample lang="yaml" %}
```yaml
apiVersion: solas.cluster.k8s.io/v1alpha1
kind: SolasMachineSpec
```

{% sample lang="yaml" %}
```yaml
apiVersion: solas.cluster.k8s.io/v1alpha1
kind: SolasMachineStatus
```
{% endmethod %}

[repo-naming]: https://github.com/kubernetes-sigs/cluster-api/issues/383
[variant-naming]: https://github.com/kubernetes-sigs/cluster-api/issues/480

[^1]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

