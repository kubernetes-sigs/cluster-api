# Tuning Controller

When tuning controllers, both for scalability, performance or for reducing their footprint, following suggestions can make your work simpler and much more effective.

- You need the right tools for the job: without logs, metrics, traces and profiles tuning is hardly possible. Also, given that tuning is an iterative work, having a setup that allows you to experiment and improve quickly could be a huge boost in your work.
- Only optimize if there is clear evidence of an issue. This evidence is key for you to measure success and it can provide the necessary context for developing, validating, reviewing and approving the fix. On the contrary, optimizing without evidence can be not worth the effort or even make things worse.

## Tooling for controller tuning in CAPI

Cluster API provides a full stack of tools for tuning its own controllers as well as controllers for all providers if developed using controller runtime. As a bonus, most of this tooling can be used with any other controller runtime based controllers.

With tilt, you can easily deploy a full observability stack with Grafana, Loki, promtail, Prometheus, kube-state-metrics, Parca and Tempo.

All tools are preconfigured, and most notably kube-state-metrics already collects CAPI metrics and Grafana is configured with a set of dashboards that we used in previous rounds of CAPI tuning. Overall, the CAPI dev environment offers a considerable amount of expertise, free to use and to improve for the entire community. We highly recommend to invest time in looking into those tools, learn and provide feedback.

Additionally, Cluster API includes both CAPD (Cluster API provider for Docker) and CAPIM (Cluster API provider in-memory). Both allow you to quickly create development clusters with the limited resources available on a developer workstation, however:

- CAPD gives you a fully functional cluster running in containers; scalability and performance are limited by the size of your machine.
- CAPIM gives you a fake cluster running in memory; you can scale more easily but the clusters do not support any Kubernetes feature other than what is strictly required for CAPI, CABPK and KCP to work.

<aside class="note warning">

<h1>Warning</h1>

Maintainers are continuously working on improving Cluster API developer environment and tooling; any help is more than welcome and with the community contribution we can make this happen sooner!

With regards to this document, following areas could benefit from community help:

- Controller runtime currently has a limited set of metrics for client-go, making it more complex to observe phenomenon like client-go rate limiting; we should start a discussion with the controller runtime-team about how to get those metrics, even if only temporarily during bottleneck investigation.

- Cluster API metrics still exists only as a dev tool, and work is required to automate metrics config generation and/or to improve consumption from kube-state-metrics; when this work will be completed it will be much more easier for other providers/other controllers to implement metrics and for user to get access to them. See [#7158](https://github.com/kubernetes-sigs/cluster-api/issues/7158).

- Tracing in Cluster API is not yet implemented; this will make much more easier to investigate slowness in reconcile loops as well as provide a visual and intuitive representation of Cluster API reconcile loops. See [#3760](https://github.com/kubernetes-sigs/cluster-api/issues/3760).

Please reach out to maintainers if you are interested in helping us to make progress in this area.

</aside>

## Analyzing metrics, traces and profiles

Tuning controllers and finding performance bottlenecks can vary depending on the issues you are dealing with, so please consider following guidelines as collection of suggestions, not as a strict process to follow.

Before looking at data, it usually helps to have a clear understanding of:

- What are the requirements and constraints of the use case you are looking at, e.g.:
  - Use a management cluster with X cpu, Y memory
  - Create X cluster, with concurrency Y
  - Each cluster must have X CP nodes, Y workers

- What does it mean for you if the system is working well, e.g.:
  - All machines should be provisioned in less than X minutes
  - All controllers should reconcile in less than Y ms
  - All controllers should allocate less than Z Gb memory

Once you know the scenario you are looking at and what you are tuning for, you can finally look at data, but given that the amount of data available could be overwhelming, you probably need a strategy to navigate all the available metrics, traces, etc. .

Among the many possible strategies, one usually very effective is to look at the KPIs you are aiming for, and then, if the current system performance is not good enough, start looking at other metrics trying to identify the biggest factor that is impacting the results. Usually by removing a single, performance bottleneck the behaviour of the system changes in a significant way; after that you can decide if the performance is now good enough or you need another round of tuning.

Let's try to make this more clear by using an example, *machine provisioning time is degrading when running CAPI at scale* (machine provisioning time can be seen in the [Cluster API Performance dashboard](http://localhost:3001/d/b2660352-4f3c-4024-837c-393d901e6981/cluster-api-performance?orgId=1)).

When running at scale, one of the first things to take care of is the client-go rate limiting, which is a mechanism built inside client-go that prevents a Kubernetes client from being accidentally too aggressive to the API server.
However this mechanism can also limit the performance of a controller when it actually requires to make many calls to the API server.

So one of the first data point to look at is the rate limiting metrics; given that upstream CR doesn't have metric for that we can only look for logs containing "client-side throttling" via [Loki](http://localhost:3001/explore) (Note: this link should be open while tilt is running).

If rate limiting is not your issue, then you can look at the controller's work queue. In an healthy system reconcile events are continuously queued, processed and removed from the queue. If the system is slowing down at scale, it could be that some controllers are struggling to keep up with the events being added in the queue, thus leading to slowness in reconciling the desired state.

So then the next step after looking at rate limiting metrics, is to look at the "work queue depth" panel in the [Controller-Runtime dashboard](http://localhost:3001/d/abe29aa7-e44a-4eef-9474-970f95f08ee6/controller-runtime?orgId=1).

Assuming that one controller is struggling with its own work queue, the next step is to look at why this is happening. It might be that the average duration of each reconcile is high for some reason. This can be checked in the "Reconcile Duration by Controller" panel in the [Controller-Runtime dashboard](http://localhost:3001/d/abe29aa7-e44a-4eef-9474-970f95f08ee6/controller-runtime?orgId=1).

If this is the case, then it is time to start looking at traces, looking for the longer spans in average (or total). Unfortunately traces are not yet implemented in Cluster API, so alternative approaches must be used, like looking at condition transitions or at logs to figure out what the slowest operations are.

And so on.

Please note that there are also cases where CAPI controllers are just idle waiting for something else to happen on the infrastructure side. In this case investigating bottlenecks requires access to a different set of metrics. Similar considerations apply if the issue is slowness of the API server or of the network.

## Runtime tuning options

Cluster API offers a set of options that can be set on the controller deployment at runtime, without the need of changing the CAPI code.

- Client-go rate limiting; by increasing the client-go rate limits we allow a controller to make more API server calls per second (`--kube-api-qps`) or to have a bigger burst to handle spikes (`--kube-api-burst`). Please note that these settings must be increased carefully, because being too aggressive on the API server might lead to different kind of problems.

- Controller concurrency (e.g. via `--kubeadmcontrolplane-concurrency`); by increasing the number of concurrent reconcile loops for each controller  it is possible to help the system in keeping the work queue clean, and thus reconciling to the desired state faster. Also in this case, trade-offs should be considered, because by increasing concurrency not only the controller footprint is going to increase, but also the number of API server calls is likely going to increase (see previous point).

- Resync period (`--sync-period`); this setting defines the interval after which reconcile events for all current objects will be triggered. Historically this value in Cluster API is much lower than the default in controller runtime (10m vs. 10h). This has some advantages, because e.g. it is a fallback in case controller struggle to pick up events from external infrastructure. But it also has impact at scale when a controller gets a sudden spike of events at every resync period. This can be mitigated by increasing the resync period.

As a general rule, you should tune those parameters only if you have evidence supported by data that you are hitting a bottleneck of the system. Similarly, another sample of data should be analyzed after tuning the parameter to check the effects of the change.

## Improving code for better performance

Performance is usually a moving target, because things can change due the evolution of the use cases, of the user needs, of the codebase and of all the dependencies Cluster API relies on, starting from Kubernetes and the infrastructure we are using.

That means that no matter of the huge effort that has been put into making CAPI performant, more work will be required to preserve the current state or to improve performance.

Also in this case, most of the considerations really depend on the issue your are dealing with, but some suggestions are worth to be considered for the majority of the use cases.

The best optimization that can be done is to avoid any work at all for controllers. E.g instead of re-queuing every few seconds when a controller is waiting for something to happen, which leads to the controller to do some work to check if something changed in the system, it is always better to watch for events, so the controller is going to do the work only once when it is actually required. When implementing watches, non-relevant changes should be filtered out whenever possible.

Same considerations apply also for the actual reconcile implementation, if you can avoid API server calls or expensive computations under certain conditions, it is always better and faster than any optimization you can do to that code.

However, when work from the controllers is required, it is necessary to make sure that expensive operations are limited as much as possible.

A common example for an expensive operation is the generation of private keys for certificates, or the creation of a Kubernetes client, but the most frequent expensive operations that each controller does are API server calls.

Luckily controller runtime does a great job in helping to address this by providing a delegating client per default that reads from a cache that is maintained by client-go shared informers. This is a huge boost of performance (microseconds vs. seconds) that everyone gets at the cost of some memory allocation and the need of considering stale reads when writing code.

As a rule of thumbs it is always better to deal with stale reads/memory consumption than disabling caching. Even if stale reads could be a concern under certain circumstances, e.g when reading an object right after it has been created.

Also, please be aware that some API server read operations are not cached by default, e.g. reads for unstructured objects, but you can enable caching for those operations when creating the controller runtime client.

But at some point some API server calls must be done, either uncached reads or write operations.

When looking at unchached reads, some operation are more expensive than others, e.g. a list call with a label selector degrades according to the number of object in the same namespace and the number of the items in the result set.

Whenever possible, you should avoid uncached list calls, or make sure they happen only once in a reconcile loop and possibly only under specific circumstances.

When looking at write operations, you can rely on some best practices developed in CAPI. Like for example use a defer call to patch the object with the patch helper to make a single write at the end of the reconcile loop (and only if there are actual changes).

In order to complete this overview, there is another category of operations that can slow down CAPI controllers, which are network calls to other services like e.g. the infrastructure provider.

Some general recommendations apply also in those cases, like e.g re-using long lived clients instead of continuously re-creating new ones, leverage on async callback and watches whenever possible vs. continuously checking for status, etc. .
