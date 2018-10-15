# Machine Controller

The machine controller is responsible for taking an incoming Kubernetes
request, figuring out what should be done with it, and dispatching it to the
machine actuator methods. 

It is the actuator methods that will be communicating with the API that will be
providing the machines (eg. a cloud provider), to realize the desired machine
state. The basic methods for this are the `Create`, `Update`, `Exists`, and
`Delete` methods and the interface for this can be seen in the
[`/pkg/controller/machine/actuator.go`] file.

In the simple case, a machine may be defined as a Linux instance at a cloud
provider whose parameters are defined within the machine state. However, this
may not always be the case and the definition of a machine is left up to the
provider implementation.

## Basic Controller Flow

  - Request comes in
  - Attempt to get the machine object that the request is for from the API
    server
  - Check if the machine is being deleted or not.
    - If it is being deleted:
      - No-op if there’s no finalizer.
      - Skip if we’re not allowed to delete this machine.
      - Delete the machine by calling actuator `Delete` method.
      - If the machine was deleted, delete the finalizer.
    - If it is not being deleted:
      - If there's no finalizer, add a finalizer.
  - Get the cluster that the machine is part of.
  - Ask actuator if the machine exists in the cluster by calling actuator
    `Exists` method.
  - If the machine exists, the request must be for an update. Call actuator
    `Update` method.
    - If the `Update` fails and returns a retryable error:
      - Retry the `Update` after N seconds.
  - If the machine does not exist yet, attempt to create machine by calling
    actuator `Create` method.
  - Desired state is reached.

<!-- Links used in the document, placed here to avoid breaking the document
     flow with URLs in the middle of a sentence. -->
[`/pkg/controller/machine/actuator.go`]: https://github.com/kubernetes-sigs/cluster-api/blob/master/pkg/controller/machine/actuator.go
