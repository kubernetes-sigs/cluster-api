# NodeRef Controller

The NodeRef controller populates a `Machine`'s `Status.NodeRef` field using the Kubernetes Node
object associated with the Machine. This controller supports only machines linked to a cluster.

Infrastructure providers can opt-in to use this controller by providing a `kubeconfig` as a Kubernetes Secret.
The secret must be stored in the namespace the Cluster lives in and named as `<cluster-name>-kubeconfig`,
the data should only contain a single key called `value` and its data should be a valid Kubernetes `kubeconfig`.
