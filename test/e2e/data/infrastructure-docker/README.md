# CAPD cluster templates

This folder contains cluster templates used in e2e tests. Each sub-folder contains cluster templates
complying to the corresponding contract.

The templates in [v1alpha3](./v1alpha3), [v1alpha4](./v1alpha4) and [v1.2](./v1beta1/v1.2) are used for 
clusterctl upgrade tests. In those tests we first deploy an old version of Cluster API and then upgrade 
to the version from the current branch. We can't use the same templates for `v1.2` and `main` as not all 
fields we use in `main` already existed in `v1.2`.

Note: The `v1.2` folder can be deleted once we start developing on v1.4.x. Depending on if we introduce and 
use new fields in v1.4.x we might have to introduce a `v1.3` folder then.
