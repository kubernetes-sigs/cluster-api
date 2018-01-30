# Contributing Guidelines

## Build

```bash
$ cd $GOPATH/src/k8s.io/
$ git clone git@github.com:kubernetes/kube-deploy.git
$ cd kube-deploy/cluster-api-gcp/
$ go build
```

This will create a binary `cluster-api-gcp` that you can use to manage a GCP cluster.

## Developing

When making changes to the machine controller, it's generally a good idea to delete any existing cluster created with an older version of the cluster-api.

```bash
$ ./cluster-api-gcp delete
```

After making changes to the machine controller or the actuator, you need to follow these two steps:

1. Rebuild the machine-controller image. Modify the `machine-controller/Makefile` and change the `PROJECT` to your own GCP project. This is important so you don't overwrite the official image. Also change `machineControllerImage` in `cloud/google/pods.go` to the new image path (make sure the version in the Makefile and `pods.go` match if you want to use the new image). Then, rebuild and push the image.

	```bash
	$ cd machine-controller
	$ make push fix-image-permissions
	```

2. Rebuild cluster-api-gcp

	```bash
	$ go build
	```

The new `cluster-api-gcp` will have your changes.

## Testing

We do not have unit tests or integration tests currently. For any changes, it is recommended that you test a create-edit-delete sequence using the new machine controller image and the new `cluster-api-gcp` binary.

1. Create a cluster

	```bash
	$ ./cluster-api-gcp create -c cluster.yaml -m machines.yaml
	```

2. Edit the machine to trigger an update

	```bash
	$ kubectl edit machine $MACHINE_NAME
	```

3. Make sure the new behavior is working as intended. Then delete the cluster.

	```bash
	$ ./cluster-api-gcp delete
	```
