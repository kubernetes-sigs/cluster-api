# Repository Naming

The naming convention for new Cluster API [_provider_ repositories][repo-naming]
is generally of the form `cluster-api-provider-${env}`, where `${env}` is a,
possibly short, name for the _environment_ in question. For example
`cluster-api-provider-gcp` is an implementation for the Google Cloud Platform,
and `cluster-api-provider-aws` is one for Amazon Web Services. Note that an
environment may refer to a cloud, bare metal, virtual machines, or any other
infrastructure hosting Kubernetes. Finally, a single environment may include
more than one [_variant_][variant-naming]. So for example,
`cluster-api-provider-aws` may include both an implementation based on EC2 as
well as one based on their hosted EKS solution.

## A note on Acronyms

Because these names end up being so long, developers of Cluster API frequently refer to providers by acronyms.
Cluster API itself becomes [CAPI], pronounced "Cappy."
cluster-api-provider-aws is [CAPA], pronounced "KappA."
cluster-api-provider-gcp is [CAPG], pronounced "Cap Gee," [and so on][letterc].

[CAPI]: https://cluster-api.sigs.k8s.io/reference/glossary.html#capi
[CAPA]: https://cluster-api.sigs.k8s.io/reference/glossary.html#capa
[CAPG]: https://cluster-api.sigs.k8s.io/reference/glossary.html#capg
[letterc]: https://cluster-api.sigs.k8s.io/reference/glossary.html#c

## Resource Naming

For the purposes of this guide we will create a provider for a
service named **mailgun**. Therefore the name of the repository will be
`cluster-api-provider-mailgun`.

Every Kubernetes resource has a *Group*, *Version* and *Kind* that uniquely
identifies it.

* The resource *Group* is similar to package in a language.
  It disambiguates different APIs that may happen to have identically named *Kind*s.
  Groups often contain a domain name, such as k8s.io.
  The domain for Cluster API resources is `cluster.x-k8s.io`, and infrastructure providers generally use `infrastructure.cluster.x-k8s.io`.
* The resource *Version* defines the stability of the API and its backward compatibility guarantees.
  Examples include v1alpha1, v1beta1, v1, etc. and are governed by the Kubernetes API Deprecation Policy [^1].
  Your provider should expect to abide by the same policies.
* The resource *Kind* is the name of the objects we'll be creating and modifying.
  In this case it's `MailgunMachine` and `MailgunCluster`.

For example, our cluster object will be:
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha3
kind: MailgunCluster
```

[repo-naming]: https://github.com/kubernetes-sigs/cluster-api/issues/383
[variant-naming]: https://github.com/kubernetes-sigs/cluster-api/issues/480

[^1]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/
