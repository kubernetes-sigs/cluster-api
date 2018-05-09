# Deploy GCE Ingress Controller

Instructions for how to deploy an ingress controller in a cluster
that was deployed by gcp-deployer

1. Replace `<YOUR PROJECT ID>` and `<YOUR CLUSTER NAME>` in
`ingress-controller.yaml`.
1. Run `kubectl create -f ingress-controller.yaml`. This will create
Kubernetes service account with the correct permissions in the cluster,
a default backend for the ingress controller, and the glbc app

Now you will be able to create ingress objects.