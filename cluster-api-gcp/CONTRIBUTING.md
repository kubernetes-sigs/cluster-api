# Contributing Guidelines

## Prerequisites

### Google Cloud Project

If you don't have a Google Cloud Project, please [create one](https://cloud.google.com/resource-manager/docs/creating-managing-projects).

### Install Google Cloud SDK (gcloud)

Google Cloud SDK (gcloud) will be helpful for two reasons:
-  Inspect GCP resources during development;
-  Set configuration values that will be used during development (like project name).

Steps to follow:
1.  Install as per [Cloud SDK instructions](https://cloud.google.com/sdk/);
2.  Configure Cloud SDK to point to the GCP project you will be using.

    ```bash
    $ gcloud config set project <GCP_PROJECT_ID>
    ```

### Set GCP Credentials

In order to use the GCP machine controller, you need to configure the credentials so that the code has access to the GCP project where resources will be created.

You can set it in two ways, as explained below.

#### Environment Variable GOOGLE_APPLICATION_CREDENTIALS

Steps to follow:
1. Verify that the environment variable `GOOGLE_APPLICATION_CREDENTIALS` is set pointing to valid service account credentials
2. If not set, follow the [instructions on Google Cloud Platform site](https://cloud.google.com/docs/authentication/getting-started) to have it set up.

#### Login Using Cloud SDK

The alternative is to set the client credentials via gcloud by executing the command line below.

```bash
$ gcloud auth application-default login
```

### Create Firewall

Create a firewall rule to allow communication from kubectl (and nodes) to the control plane.

   ```bash
   gcloud compute firewall-rules create cluster-api-open --allow=TCP:443 --source-ranges=0.0.0.0/0 --target-tags='https-server'
   ```

### Install Docker

1. Install [Docker](https://docs.docker.com/install/) on your machine;
2. Make sure your user can execute docker commmands (without sudo). This is a way to test it:
```bash
$ docker run hello-world

Hello from Docker!
This message shows that your installation appears to be working correctly.
...
```

## Fetch Source Code

1. Fork [kube-deploy repo](https://github.com/kubernetes/kube-deploy). If it's your first time forking, please take a look at [GitHub Repo instructions](https://help.github.com/articles/fork-a-repo/). The general [Kubernetes GitHub workflow](https://github.com/kubernetes/community/blob/master/contributors/guide/github-workflow.md) is helpful here too if you're getting started.

2. Clone Repo Locally
```bash
$ cd $GOPATH/src/k8s.io/
$ git clone https://github.com/<GITHUB_USERNAME>/kube-deploy.git
$ cd kube-deploy/cluster-api-gcp/
$ go build
```

## Build

```bash
$ cd $GOPATH/src/k8s.io/kube-deploy/cluster-api-gcp/
$ go build
```

This will create a binary `cluster-api-gcp` in the same directory. You can use that binary to manage a GCP cluster.

## Developing

When making changes to the machine controller, it's generally a good idea to delete any existing cluster created with an older version of the cluster-api.

```bash
$ ./cluster-api-gcp delete
```

After making changes to the machine controller or the actuator, you need to follow these two steps:

1. Rebuild the machine-controller image. Also change `machineControllerImage` in `cloud/google/pods.go` to the new image path (make sure the version in the Makefile and `pods.go` match if you want to use the new image). Then, rebuild and push the image.

	```bash
	$ cd machine-controller
	$ make push fix-image-permissions
	```

NOTE: that the image will be pushed to `gcr.io/$(GCLOUD_PROJECT)/machine-controller`. Image storage is a billable resource.

2. Rebuild cluster-api-gcp

	```bash
    $ cd ..
	$ go build
	```

The new `cluster-api-gcp` will have your changes.

## Testing

We do not have unit tests or integration tests currently. For any changes, it is recommended that you test a create-edit-delete sequence using the new machine controller image and the new `cluster-api-gcp` binary.

1. Generate machines configuration file.

This step is necessary to include the project name (as configured in Google Cloud SDK) in the yaml file.

```bash
$ ./generate-yaml.sh
```

If Cloud SDK isn't configured, you will see an error like the one below:

```bash
$ ./generate-yaml.sh
ERROR: (gcloud.config.get-value) Section [core] has no property [project].
```

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

## Updating vendor directory for cluster-api changes

For changes to dependencies, like [cluster-api](https://github.com/kubernetes/kube-deploy/tree/master/cluster-api), follow the steps:

1. Make sure you have dep installed (or install it from [here])

2. Run ```dep ensure -update``` as follows:

```bash
$ cd $GOPATH/src/k8s.io/kube-deploy/cluster-api-gcp/
$ dep ensure -update k8s.io/kube-deploy
```

3. Submit a pull request with the updated files.
