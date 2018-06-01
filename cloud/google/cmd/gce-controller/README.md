# GCE Machine Controller

The GCE Machine Controller is a machine controller implementation for clusters running on Google Compute Engine.

## Development
The instructions below will enable you to get started with running your own version of the gce-controller on a GCE Kubernetes cluster.

### Prerequisites

1. Follow the "Before you begin" section from the google cloud container registry [pushing and pulling images documentation](https://cloud.google.com/container-registry/docs/pushing-and-pulling).
1. From this directory, run `make dev_push`.
1. Note from the ouput of the above step the name of the image. It should be in this format, `gcr.io/MY-GCP-PROJECT-NAME/gce-controller:VERSION-dev`.
1. From the same terminal in which you will create your cluster using gcp-deployer, run `export MACHINE_CONTROLLER_IMAGE=gcr.io/MY-GCP-PROJECT-NAME/gce-controller:VERSION-dev`. Note that the value should be equal to the name of the image you noted in the output from the previous step.
1. Follow the steps listed at [gcp-deployer](../../../../gcp-deployer/) and create a new cluster.
1. Run the following command to ensure that new images are fetched for every new gce-controller container, `kubectl patch deployment clusterapi -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"gce-controller\",\"imagePullPolicy\":\"Always\"}]}}}}"`.

### Running a Custom GCE Machine Controller

1. Make a change to gce-controller. For example, edit main.go and insert the following print statement `glog.Error("Hello World!")` below `logs.InitLogs()`.
1. From this folder, run `make dev_push`.
1. Run `kubectl patch deployment clusterapi -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"gce-controller\",\"env\":[{\"name\":\"DATE\",\"value\":\"$(date +'%s')\"}]}]}}}}"`. This command inserts or updates an environment variable named `DATE` which triggers a new deployment.

### Verifying Your Environment

1. Install `jq`. Instructions can be found [here](https://stedolan.github.io/jq/download/). 
1. Run the following, `kubectl get pods -o json | jq '.items[].status.containerStatuses[] | select(.name=="gce-controller")'`. Validate the the hash in the `imageID` field matches the image you built above. 
1. Run the following, it will store, in `${POD_NAME}`, the name of your main clusterapi pod, `POD_NAME=$(kubectl get pods -o json | jq '.items[] | select(.status.containerStatuses[].name=="gce-controller") | .metadata.name' --raw-output)`.
1. Run `kubectl logs --namespace=default ${POD_NAME} -c gce-controller`. Look for the output or change that you added to gce-controller.