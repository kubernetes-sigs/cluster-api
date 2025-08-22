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

For the purposes of this guide we will create an infrastructure provider for a
service named **mailgun**. Therefore the name of the repository will be
`cluster-api-provider-mailgun`.

Please note that other naming conventions/best practices applies, e.g. for
API types (continue to this guide to get more info).

## A note on Acronyms

Because these names end up being so long, developers of Cluster API frequently refer to providers by acronyms.
Cluster API itself becomes [CAPI], pronounced "Cappy."
cluster-api-provider-aws is [CAPA], pronounced "KappA."
cluster-api-provider-gcp is [CAPG], pronounced "Cap Gee," [and so on][letterc].

[CAPI]: ../../../reference/glossary.md#capi
[CAPA]: ../../../reference/glossary.md#capa
[CAPG]: ../../../reference/glossary.md#capg
[letterc]: ../../../reference/glossary.md#c
[repo-naming]: https://github.com/kubernetes-sigs/cluster-api/issues/383
[variant-naming]: https://github.com/kubernetes-sigs/cluster-api/issues/480
