# CAPD cluster templates

This folder contains cluster templates used in e2e tests. Each sub-folder contains cluster templates
for the corresponding Cluster API version.

Sub-folders for the old versions of Cluster API should contain only templates used for the clusterctl upgrade tests.
In those tests we first deploy an old version of Cluster API, create a workload cluster and then upgrade Cluster API
to the version from the current branch (and check that the workload cluster still works).

As of today we have clusterctl upgrade tests for the latest stable versions of each supported branch and an additional 
test for v1alpha4 / v0.4 which will be removed when that version is no longer served.

We cannot use the same cluster templates for all Cluster API versions as not each Cluster API version supports
the same API fields. For example `KubeadmControlPlane.spec.rolloutBefore.certificatesExpiryDays` was introduced 
with v1.3 so we couldn't have used it in v1.2 cluster templates.
