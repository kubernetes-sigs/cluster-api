# clusterctl adopt

The `clusterctl adopt` command is designed for allowing users to start using clusterctl on management clusters originally
created by installing providers with `kubectl apply <components-yaml>` instead of `clusterctl init`.

The adoption process must be repeated for each provider installed in the cluster, thus allowing clusterctl to re-create
the providers inventory as described in the `clusterctl init` [documentation](init.md#additional-information). 

## Pre-requisites

In order for `clusterctl adopt` to work, ensure the components are correctly
labeled. Please see the [provider contract labels][provider-contract-labels] for reference.

<!-- links -->
[provider-contract-labels]: ../provider-contract.md#labels

## Adopting a provider

TODO