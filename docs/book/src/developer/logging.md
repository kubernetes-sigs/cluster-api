# Logging
The Cluster API project is committed to improving the SRE/developer experience when troubleshooting issues, and logging
plays an important part in this goal.

In Cluster API we strive to follow three principles while implementing logging:

- **Logs are for SRE & developers, not for end users!**
  Whenever an end user is required to read logs to understand what is happening in the system, most probably there is an
  opportunity for improvement of other observability in our API, like e.g. conditions and events.
- **Navigating logs should be easy**:
  We should make sure that SREs/Developers can easily drill down logs while investigating issues, e.g. by allowing to
  search all the log entries for a specific Machine object, eventually across different controllers/reconciler logs.
- **Cluster API developers MUST use logs!**
  As Cluster API contributors you are not only the ones that implement logs, but also the first users of them. Use it!
  Provide feedback!

## Upstream Alignment

Kubernetes defines a set of [logging conventions](https://git.k8s.io/community/contributors/devel/sig-instrumentation/logging.md), 
as well as tools and libraries for logging.

## Continuous improvement

The foundational items of Cluster API logging are:

- Support for structured logging in all the Cluster API controllers (see [log format](#log-format)).
- Using contextual logging (see [contextual logging](#contextual-logging)).
- Adding a minimal set of key/value pairs in the logger at the beginning of each reconcile loop, so all the subsequent
  log entries will inherit them (see [key value pairs](#keyvalue-pairs)).

Starting from the above foundations, then the long tail of small improvements will consist of following activities: 
 
- Improve consistency of additional key/value pairs added by single log entries (see [key value pairs](#keyvalue-pairs)).
- Improve log messages (see [log messages](#log-messages)).
- Improve consistency of log levels (see [log levels](#log-levels)).

## Log Format

Controllers MUST provide support for [structured logging](https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/1602-structured-logging)
and for the [JSON output format](https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/1602-structured-logging#json-output-format); 
quoting the Kubernetes documentation, these are the key elements of this approach:

- Separate a log message from its arguments.
- Treat log arguments as key-value pairs.
- Be easily parsable and queryable.

Cluster API uses all the tooling provided by the Kubernetes community to implement structured logging: [Klog](https://github.com/kubernetes/klog), a
[logr](https://github.com/go-logr/logr) wrapper that works with controller runtime, and other utils for exposing flags
in the controller’s main.go.

Ideally, in a future release of Cluster API we will make JSON output format the default format for all the Cluster API
controllers (currently the default is still text format).

## Contextual logging

[Contextual logging](https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/3077-contextual-logging)
is the practice of using a log stored in the context across the entire chain of calls of a reconcile
action. One of the main advantages of this approach is that key value pairs which are added to the logger at the
beginning of the chain are then inherited by all the subsequent log entries created down the chain.

Contextual logging is also embedded in controller runtime; In Cluster API we use contextual logging via controller runtime's
`LoggerFrom(ctx)` and `LoggerInto(ctx, log)` primitives and this ensures that:

- The logger passed to each reconcile call has a unique `reconcileID`, so all the logs being written during a single 
  reconcile call can be easily identified (note: controller runtime also adds other useful key value pairs by default).
- The logger has a key value pair identifying the objects being reconciled,e.g. a Machine Deployment, so all the logs
  impacting this object can be easily identified.

Cluster API developer MUST ensure that:

- The logger has a set of key value pairs identifying the hierarchy of objects the object being reconciled belongs to,
  e.g. the Cluster a Machine Deployment belongs to, so it will be possible to drill down logs for related Cluster API
  objects while investigating issues.

## Key/Value Pairs

One of the key elements of structured logging is key-value pairs.

Having consistent key value pairs is a requirement for ensuring readability and for providing support for searching and
correlating lines across logs.

A set of good practices for defining key value pairs is defined in the [Kubernetes Guidelines](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/migration-to-structured-logging.md#name-arguments), and
one of the above practices is really important for Cluster API developers

- Developers MUST use `klog.KObj` or `klog.KRef` functions when logging key value pairs for Kubernetes objects, thus
  ensuring a key value pair representing a Kubernetes object is formatted consistently in all the logs.

Please note that, in order to ensure logs can be easily searched it is important to ensure consistency for the following
key value pairs (in order of importance):

- Key value pairs identifying the object being reconciled, e.g. a Machine Deployment.
- Key value pairs identifying the hierarchy of objects being reconciled, e.g. the Cluster a Machine Deployment belongs
  to.
- Key value pairs identifying side effects on other objects, e.g. while reconciling a MachineDeployment, the controller 
  creates a MachinesSet.
- Other Key value pairs.

## Log Messages

- A Message MUST always start with a capital letter.
- Period at the end of a message MUST be omitted.
- Always prefer logging before the action, so in case of errors there will be an immediate, visual correlation between
  the action log and the corresponding error log; While logging before the action, log verbs should use the -ing form.
- Ideally log messages should surface a different level of detail according to the target log level (see [log levels](#log-levels)
  for more details).

## Log Levels

Kubernetes provides a set of [recommendations](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md#what-method-to-use)
for log levels; as a small integration on the above guidelines we would like to add:

- Logs at the lower levels of verbosity (<=3) are meant to document “what happened” by describing how an object status
  is being changed by controller/reconcilers across subsequent reconciliations; as a rule of thumb, it is reasonable
  to assume that a person reading those logs has a deep knowledge of how the system works, but it should not be required
  for those persons to have knowledge of the codebase. 
- Logs at higher levels of verbosity (>=4) are meant to document “how it happened”, providing insight on thorny parts of
  the code; a person reading those logs usually has deep knowledge of the codebase. 
- Don’t use verbosity higher than 5.

Ideally, in a future release of Cluster API we will switch to use 2 as a default verbosity (currently it is 0) for all the Cluster API
controllers as recommended by the Kubernetes guidelines.

## Trade-offs

When developing logs there are operational trade-offs to take into account, e.g. verbosity vs space allocation, user
readability vs machine readability, maintainability of the logs across the code base.

A reasonable approach for logging is to keep things simple and implement more log verbosity selectively and only on
thorny parts of code. Over time, based on feedback from SRE/developers, more logs can be added to shed light where necessary.

## Developing and testing logs

We are continuously improving our [Tilt](tilt.md) setup allowing Cluster API developers to use logs and improve them as part of
their development process.
 
Developers can deploy an entire log observability suite by simply setting the
`deploy_observability` value in your [tilt-setting.yaml](tilt.md#create-a-tilt-settings-file).

```yaml
deploy_observability:
- promtail
- loki
- grafana
```

In order to test structured logging it is required to override some controller flags, and similar approach can be used 
for increasing log verbosity, e.g.

```yaml
extra_args:
  core:
    - "--logging-format=json"
    - "--v=5"
  docker:
    - "--v=5"
    - "--logging-format=json"
  kubeadm-bootstrap:
    - "--v=5"
    - "--logging-format=json"
  kubeadm-control-plane:
    - "--v=5"
    - "--logging-format=json"
```

Over time, we would like to further improve this with the help of the community; feel free to open issues or propose
ideas!

## What about providers
Cluster API providers are developed by independent teams, and each team is free to define their own processes and
conventions.

However, given that SRE/developers looking at logs are often required to look both at logs from core CAPI and providers,
we encourage providers to adopt and contribute to the guidelines defined in this document.

It is also worth noting that the foundational elements of the approach described in this document are easy to achieve
by leveraging default Kubernetes tooling for logging.
