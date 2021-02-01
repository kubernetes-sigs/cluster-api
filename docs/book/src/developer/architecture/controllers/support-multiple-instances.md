# Support running multiple instances of the same provider

Up until v1alpha3, the need of supporting [multiple credentials](../../../reference/glossary.md#multi-tenancy) was addressed by running multiple
instances of the same provider, each one with its own set of credentials while watching different namespaces.

However, running multiple instances of the same provider proved to be complicated for several reasons:

- Complexity in packaging providers: CustomResourceDefinitions (CRD) are global resources, these may have a reference
  to a service that can be used to convert between CRD versions (conversion webhooks). Only one of these services should
  be running at any given time, this requirement led us to previously split the webhooks code to a different deployment
  and namespace.
- Complexity in deploying providers, due to the requirement to ensure consistency of the management cluster, e.g.
  controllers watching the same namespaces.
- The introduction of the concept of management groups in clusterctl, with impacts on the user experience/documentation.
- Complexity in managing co-existence of different versions of the same provider while there could be only
  one version of CRDs and webhooks. Please note that this constraint generates a risk, because some version of the provider
  de-facto were forced to run with CRDs and webhooks deployed from a different version.

Nevertheless, we want to make it possible for users to choose to deploy multiple instances of the same providers,
in case the above limitations/extra complexity are acceptable for them.

## Contract

In order to make it possible for users to deploy multiple instances of the same provider:

- Providers MUST support the `--namespace` flag in their controllers.
- Providers MUST support the `--watch-filter` flag in their controllers.

⚠️ Users selecting this deployment model, please be aware:

- Support should be considered best-effort.
- Cluster API (incl. every provider managed under `kubernetes-sigs`, won't release a specialized components file
  supporting the scenario described above; however, users should be able to create such deployment model from
  the `/config` folder.
- Cluster API (incl. every provider managed under `kubernetes-sigs`) testing infrastructure won't run test cases
  with multiple instances of the same provider.

In conclusion, giving the increasingly complex task that is to manage multiple instances of the same controllers,
the Cluster API community may only provide best effort support for users that choose this model.

As always, if some members of the community would like to take on the responsibility of managing this model,
please reach out through the usual communication channels, we'll make sure to guide you in the right path.
