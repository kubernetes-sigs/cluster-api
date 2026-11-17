# Runtime Extensions

> [!IMPORTANT]
> The `RuntimeSDK` feature flag must be set to true in order to use this feature;
> please check documentation of different set of hooks to check if additional feature flags are required. 
> See [Feature Gates](../../reference/feature-gates.md) for more details.

The Runtime SDK feature provides an extensibility mechanism that allows systems, products, and services built on top of Cluster API to hook into a workload cluster’s lifecycle.

> [!CAUTION]
> Please note Runtime SDK is an advanced feature. If implemented incorrectly, a failing Runtime Extension can severely impact the Cluster API runtime.

**Feature gate name**: `RuntimeSDK`

**Variable name to enable/disable the feature gate**: `EXP_RUNTIME_SDK`

Additional documentation:

* Background information:
    * [Runtime SDK CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220221-runtime-SDK.md)
    * [Topology Mutation Hook CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220330-topology-mutation-hook.md)
    * [Runtime Hooks for Add-on Management CAEP](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220414-lifecycle-hooks.md)
* For Runtime Extension developers:
    * [Implementing Runtime Extensions](./implement-extensions.md)
    * [Implementing In-Place Update Hooks Extensions](./implement-in-place-update-hooks.md)
    * [Implementing Lifecycle Hook Extensions](./implement-lifecycle-hooks.md)
    * [Implementing Topology Mutation Hook Extensions](./implement-topology-mutation-hook.md)
    * [Implementing Upgrade Plan Runtime Extensions](./implement-upgrade-plan-hooks.md)
* For Cluster operators:
    * [Deploying Runtime Extensions](./deploy-runtime-extension.md)
