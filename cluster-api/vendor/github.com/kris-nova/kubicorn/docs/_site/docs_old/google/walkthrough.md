# Setting up Kubernetes in Google with TLS secured private VPN mesh

In the following, we're going to show you how to use `kubicorn` to ramp up a Kubernetes cluster in Google cloud, use it and tear it down again.
The cluster will be running over Google private networking on an encrypted VPN mesh.

As a prerequisite, you need to have `kubicorn` installed. Since we don't have binary releases yet, we assume you've got Go installed and simply do:

#### Installing

```
$ go get github.com/kris-nova/kubicorn
```

The first thing you will do now is to define the cluster resources.
For this, you need to select a certain profile. Of course, once you're more familiar with `kubicorn`, you can go ahead and extend existing profiles or create new ones.
In the following we'll be using an existing profile called `do`, which is a profile for a cluster in Google.

#### Creating

You will need to create a project in you google cloud account. This project will get a projectid, something like kubicorn-132742.
Now execute the following command to create a cluster with name `myfirstk8s`:

```
$ kubicorn create myfirstk8s --cloudid kubicorn-132742 --profile google
```

Verify that `kubicorn create` did a good job by executing:

```
$ cat _state/myfirstk8s/cluster.yaml
```

Feel free to tweak the configuration to your liking here.

#### Authenticating

We're now in a position to have the cluster resources defined, locally, based on the selected profile.
Next we will apply the so defined resources using the `apply` command, but before we do that we'll set up the access to Google.
You will need a Google Service account key.
You can use [this guide to create an Service account key](https://cloud.google.com/iam/docs/creating-managing-service-account-keys).
You will then get a file that is needed to communicate with google.

Next, export the environment variable `GOOGLE_APPLICATION_CREDENTIALS` so that `kubicorn` can pick it up in the next step:

```
$ export GOOGLE_APPLICATION_CREDENTIALS="~/location.json"
```

Also, make sure that the public SSH key for your Google account is called `id_rsa.pub`, which is the default in above profile:

```
$ ls -al ~/.ssh/id_rsa.pub
-rw-------@ 1 mhausenblas  staff   754B 20 Mar 04:03 /Users/mhausenblas/.ssh/id_rsa.pub
```

#### Applying

With the access set up, we can now apply the resources we defined in the first step.
This actually creates resources in Google. Up to now we've only been working locally.

So, execute:

```
$ kubicorn apply myfirstk8s
```

Now `kubicorn` will reconcile your intended state against the actual state in the cloud, thus creating a Kubernetes cluster.
A `kubectl` configuration file (kubeconfig) will be created or appended for the cluster on your local filesystem.
You can now `kubectl get nodes` and verify that Kubernetes 1.7.0 is now running.
You can also `ssh` into your instances using the example command found in the output from `kubicorn`

#### Deleting

To delete your cluster run:

```
$ kubicorn delete myfirstk8s
```

Congratulations, you're an official `kubicorn` user now and might want to dive deeper,
for example, learning how to define your own [profiles](https://github.com/kris-nova/kubicorn/tree/master/profiles).
