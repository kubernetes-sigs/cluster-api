# Deploy Runtime Extensions

<aside class="note warning">

<h1>Caution</h1>

Please note Runtime SDK is an advanced feature. If implemented incorrectly, a failing Runtime Extension can severely impact the Cluster API runtime.

</aside>

Cluster API requires that each Runtime Extension must be deployed using an endpoint accessible from the Cluster API
controllers. The recommended deployment model is to deploy a Runtime Extension in the management cluster by:

- Packing the Runtime Extension in a container image.
- Using a Kubernetes Deployment to run the above container inside the Management Cluster.
- Using a Cluster IP Service to make the Runtime Extension instances accessible via a stable DNS name.
- Using a cert-manager generated Certificate to protect the endpoint.
- Register the Runtime Extension using ExtensionConfig.

For an example, please see our [test extension](https://github.com/kubernetes-sigs/cluster-api/tree/main/test/extension)
which follows, as closely as possible, the kubebuilder setup used for controllers in Cluster API.

There are a set of important guidelines that must be considered while choosing the deployment method:

## Availability

It is recommended that Runtime Extensions should leverage some form of load-balancing, to provide high availability
and performance benefits. You can run multiple Runtime Extension servers behind a Kubernetes Service to leverage the
load-balancing that services support.

## Identity and access management

The security model for each Runtime Extension should be carefully defined, similar to any other application deployed
in the Cluster. If the Runtime Extension requires access to the apiserver the deployment must use a dedicated service 
account with limited RBAC permission. Otherwise no service account should be used.

On top of that, the container image for the Runtime Extension should be carefully designed in order to avoid
privilege escalation (e.g using [distroless](https://github.com/GoogleContainerTools/distroless) base images).
The Pod spec in the Deployment manifest should enforce security best practices (e.g. do not use privileged pods).

##  Alternative deployments methods

Alternative deployment methods can be used as long as the HTTPs endpoint is accessible, like e.g.:

- deploying the HTTPS Server as a part of another component, e.g. a controller.
- deploying the HTTPS Server outside the Management Cluster.

In those cases recommendations about availability and identity and access management still apply.
