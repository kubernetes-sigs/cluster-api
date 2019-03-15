# Architecture

{% panel style="info", title="Background Information" %}
It may be useful to read at least the following chapters of the Kubebuilder 
book in order to better understand this section.

- [What is a Resource](https://github.com/kubernetes-sigs/kubebuilder/blob/master/docs/book/basics/what_is_a_resource.md)
- [What is a Controller](https://github.com/kubernetes-sigs/kubebuilder/blob/master/docs/book/basics/what_is_a_controller.md)
{% endpanel %}

{% panel style="warning", title="Architecture Diagram" %}
We need a new simple diagram here reflecting the move to CRDs.
{% endpanel %}

![Architecture](architecture.svg)

## Cluster API Resources

The Cluster API currently defines the following resource types:

- Cluster
- Machine
- MachineSet
- MachineDeployment
- MachineClass

## Controllers and Actuators

A Kubernetes Controller is a routine running in a Kubernetes cluster that 
watches for create / update / delete events on Resources, and triggers a 
Reconcile function in response. Reconcile is a function that may be called
at any time with the Namespace and Name of an object (Resource instance), 
and it will attempt to make the cluster state match the state declared in the 
object `Spec`. 

The Cluster API consists of a shared set of controllers in order to provide a 
consistent user experience. In order to support multiple cloud environments, 
some of these controllers (e.g. `Cluster`, `Machine`) call provider specific 
actuators to implement the behavior expected of the controller. Other 
controllers (e.g. `MachineSet`) are generic and operate by interacting with 
other Cluster API (and Kubernetes) resources.

The task of the provider implementor is to implement the actuator methods so 
that the shared controllers can call those methods when they must reconcile 
state and update status.

When an actuator function returns an error, if it is [RequeueAfterError](
https://github.com/kubernetes-sigs/cluster-api/blob/fa906f36843b065c5294501efe7d78ebd85c3c04/pkg/controller/error/requeue_error.go#L27) then the object will be
requeued for further processing after the given RequeueAfter time has
passed.
