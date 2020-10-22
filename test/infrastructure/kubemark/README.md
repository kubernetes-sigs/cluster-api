```
   ________           __               ___    ____  ____
  / ____/ /_  _______/ /____  _____   /   |  / __ \/  _/
 / /   / / / / / ___/ __/ _ \/ ___/  / /| | / /_/ // /
/ /___/ / /_/ (__  ) /_/  __/ /     / ___ |/ ____// /
\____/_/\__,_/____/\__/\___/_/     /_/  |_/_/   /___/

    ____                  _     __
   / __ \_________ _   __(_)___/ /__  ______
  / /_/ / ___/ __ \ | / / / __  / _ \/ ___(_)
 / ____/ /  / /_/ / |/ / / /_/ /  __/ /  _
/_/   /_/   \____/|___/_/\__,_/\___/_/  (_)

    __ __      __                              __
   / //_/_  __/ /_  ___  ____ ___  ____ ______/ /__
  / ,< / / / / __ \/ _ \/ __ `__ \/ __ `/ ___/ //_/
 / /| / /_/ / /_/ /  __/ / / / / / /_/ / /  / ,<
/_/ |_\__,_/_.___/\___/_/ /_/ /_/\__,_/_/  /_/|_|

```

## What is the Cluster API Provider Kubemark

Cluster API Provider Kubemark (CAPK) is a provider for [Cluster
API][cluster_api] (CAPI) that allows users to deploy fake, [Kubemark][kubemark_docs]-backed machines to their
clusters. This is useful in a variety of scenarios, such load-testing and
simulation testing. 

It is slightly similar to [CAPD][capd], the Docker
provider, in that it does not deploy actual infrastructure resources (VMs or
bare-metal machines). It differs significantly in that Kubemark nodes only
pretend to run pods scheduled to them. For more information on Kubemark, the
[Kubemark developer guide][kubemark_docs] has more details.

## Getting started
At this point the Kubemark provider is extremely alpha. To deploy the Kubemark
provider, you can add the latest release to your clusterctl config file, by
default located at `~/.cluster-api/clusterctl.yaml`.

```yaml
providers:
- name: "kubemark"
  url: "https://github.com/benmoss/cluster-api-provider-kubemark/releases/latest/infrastructure-components.yaml"
  type: "InfrastructureProvider"
```

For demonstration purposes, we'll use the [CAPD][capd] provider. Other
providers will also work, but CAPD is supported with a custom
[template](templates/cluster-template-capd.yaml) that makes deployment super
simple.

Initialize this provider into your cluster by running:

```bash
clusterctl init --infrastructure kubemark,capd
```

Once initialized, you'll need to deploy your workload cluster using the `capd`
flavor to get a hybrid CAPD/CAPK cluster:

```bash
clusterctl config cluster wow --infrastructure kubemark --flavor capd --kubernetes-version 1.19.1 --control-plane-machine-count=1 --worker-machine-count=4 | kubectl apply -f-
```

You should see your cluster come up and quickly become available with 4 Kubemark machines connected to your CAPD control plane.

For other providers, you can either create a custom hybrid cluster template, or deploy the control plane and worker machines separately, specifiying the same cluster name:

```bash
clusterctl config cluster wow --infrastructure aws      --kubernetes-version 1.19.1 --control-plane-machine-count=1 | kubectl apply -f-
clusterctl config cluster wow --infrastructure kubemark --kubernetes-version 1.19.1 --worker-machine-count=4        | kubectl apply -f-
```

## Using tilt
To deploy the Kubemark provider, the recommended way at this time is using
[Tilt][tilt]. Clone this repo and use the [CAPI tilt guide][capi_tilt] to get
Tilt setup. Add `kubemark` to the list of providers in your
`tilt-settings.json` file and you should be off to the races.

<!-- References -->

[capd]: https://github.com/kubernetes-sigs/cluster-api/tree/master/test/infrastructure/docker
[kubemark_docs]: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-scalability/kubemark-guide.md
[cluster_api]: https://github.com/kubernetes-sigs/cluster-api
[tilt]: https://tilt.dev
[capi_tilt]: https://master.cluster-api.sigs.k8s.io/developer/tilt.html
