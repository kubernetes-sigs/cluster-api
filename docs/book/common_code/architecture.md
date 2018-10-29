# Architecture

{% panel style="info", title="Background Information" %}
It may be useful to read at least the following chapters of the Kubebuilder 
book in order to better understand this section.

- [What is a Resource](https://github.com/kubernetes-sigs/kubebuilder.git/docs/book/basics/what_is_a_resource.md)
- [What is a Controller](https://github.com/kubernetes-sigs/kubebuilder.git/docs/book/basics/what_is_a_controller.md)
{% endpanel %}

{% panel style="warning", title="Architecture Diagram" %}
We need a new simple diagram here reflecting the move to CRDs.
{% endpanel %}

![Architecture](architecture.png)

## Cluster API Resources

The Cluster API currently defines the following resource types:

- Cluster
- Machine
- MachineSet
- MachineClass
- MachineDeployment

## Controllers and Actuators

A Kubernetes Controller is a routine running in a Kubernetes cluster that 
watches for create / update / delete events on Resources, and triggers a 
Reconcile function in response. Reconcile is a function that may be called
at any time with the Namespace and Name of an object (Resource instance), 
and it will make the cluster state match the state declared in the object
Spec. Upon completion, Reconcile updates the object Status to the new actual
state.

The Cluster API consists of a shared set of controllers in order to provide a 
consistent user experience. In order to support multiple cloud environments, 
some of these controllers (e.g. `Cluster`, `Machine`) call provider specific 
actuators to implement the the behavior expected of the controller. Other 
controllers (e.g. `MachineSet`) are generic and operate by interacting with 
other Cluster API (and Kubernetes) resources.

The task of the provider implementor is to implement the actuator methods so 
that the shared controllers can call those methods when they must reconcile 
state and update status.
