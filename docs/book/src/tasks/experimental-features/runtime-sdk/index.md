# Experimental Feature: Runtime SDK (alpha)

The Runtime SDK feature provides an extensibility mechanism that allows systems, products, and services built on top of Cluster API to hook into a workload cluster’s lifecycle.

<aside class="note warning">

All currently implemented hooks require to also enable the [ClusterClass](../cluster-class/index.md) feature.
Please note that hooks are only invoked for Clusters created using ClusterClass.

</aside>

<aside class="note warning">

<h1>Caution</h1>

Please note Runtime SDK is an advanced feature. If implemented incorrectly, a failing Runtime Extension can severely impact the Cluster API runtime.

</aside>

**Feature gate name**: `RuntimeSDK`

**Variable name to enable/disable the feature gate**: `EXP_RUNTIME_SDK`

Additional documentation:

* Background information:
    * [Runtime SDK CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220221-runtime-SDK.md)
    * [Topology Mutation Hook CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220330-topology-mutation-hook.md)
    * [Runtime Hooks for Add-on Management CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220414-runtime-hooks.md)
* For Runtime Extension developers:
    * [Implementing Runtime Extensions](./implement-extensions.md)
    * [Implementing Lifecycle Hook Extensions](./implement-lifecycle-hooks.md)
    * [Implementing Topology Mutation Hook Extensions](./implement-topology-mutation-hook.md)
* For Cluster operators:
    * [Deploying Runtime Extensions](./deploy-runtime-extension.md)
