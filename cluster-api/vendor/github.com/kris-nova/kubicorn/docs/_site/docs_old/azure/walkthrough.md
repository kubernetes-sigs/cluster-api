# Setting up Kubernetes in Azure

In the following, we're going to show you how to use `kubicorn` to ramp up a Kubernetes cluster in Azure, use it and tear it down again.

As a prerequisite, you need to have `kubicorn` installed. Since we don't have binary releases yet, we assume you've got Go installed and simply do:

#### Installing

```
$ go get github.com/kris-nova/kubicorn
```

The first thing you will do now is to define the cluster resources.
For this, you need to select a certain profile. Of course, once you're more familiar with `kubicorn`, you can go ahead and extend existing profiles or create new ones.
In the following we'll be using an existing profile called `do`, which is a profile for a cluster in DigitalOcean.

#### Creating

Now execute the following command:

```
$ kubicorn create myfirstk8s --profile azure
```

Verify that `kubicorn create` did a good job by executing:

```
$ cat _state/myfirstk8s/cluster.yaml
```

Feel free to tweak the configuration to your liking here.

#### Authenticating

Some work will be needed to configure your system to authenticate with `kubicorn`.
Please spend some time and go through each step carefully to ensure your configuration work correctly.

##### Manually using the Azure CLI tool

Install the Azure CLI tool and login using the following commands.

```bash
$ curl -L https://aka.ms/InstallAzureCli | bash
$ exec -l $SHELL
$ az login
```

Create a `Service Principal` for the Azure Active Directory using the following command.

```bash
$ az ad sp create-for-rbac
{
  "appId": "1234567-1234-1234-1234-1234567890ab",
  "displayName": "azure-cli-2017-08-18-19-25-59",
  "name": "http://azure-cli-2017-08-18-19-25-59",
  "password": "1234567-1234-1234-be18-1234567890ab",
  "tenant": "1234567-1234-1234-be18-1234567890ab"
}
```

Translate the output from the previous command to newly exported environmental variables.

**Warning**: The names of the values might change from the previous command to the environmental variables. 
Follow the chart closely.

Service Principal Variable Name | Environmental variable
--- | ---
appId | AZURE_CLIENT_ID
password | AZURE_CLIENT_SECRET
tenant | AZURE_TENANT_ID

Run the following command to get you Azure subscription ID.

```bash
$ az account show --query id
"1234567-1234-1234-1234567890ab"
```

Finally export that value as an environmental variable as well.

Command| Environmental variable
--- | ---
az account show --query id | AZURE_SUBSCRIPTION_ID

**At this point you should have the following 4 environmental variables set!**

```bash
export AZURE_CLIENT_ID = "1234567-1234-1234-1234567890ab"
export AZURE_CLIENT_SECRET = "1234567-1234-1234-1234567890ab"
export AZURE_TENANT_ID = "1234567-1234-1234-1234567890ab"
export AZURE_SUBSCRIPTION_ID = "1234567-1234-1234-1234567890ab"
```


Also, make sure that the public SSH key for your Azure account is called `id_rsa.pub`, which is the default in above profile:

```
$ ls -al ~/.ssh/id_rsa.pub
-rw-------@ 1 knova  wheel   754B 20 Mar 04:03 /home/knova/.ssh/id_rsa.pub
```

#### Applying

With the access set up, we can now apply the resources we defined in the first step. 
This actually creates resources in Azure! Up to now we've only been working locally.

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
