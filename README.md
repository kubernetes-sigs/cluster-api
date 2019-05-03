# Cluster API

Cluster API provides the ability to manage Kubernetes supportable hosts in the
context of OpenShift.

This branch contains an implementation of a machineset-controller and
machine-controller as well as their supporting libraries.

Each of these controllers is deployed by the
[machine-api-operator](https://github.com/openshift/machine-api-operator)

# Upstream Implementation
Other branches of this repository may choose to track the upstream
Kubernetes [Cluster-API project](https://github.com/kubernetes-sigs/cluster-api)

In the future, we may align the master branch with the upstream project as it
stabilizes within the community.
