# Deleting a Kubernetes Cluster

If you would like to delete a kubernetes cluster, there are a few tools available to you:

* [UpUp](#using-upup-to-delete-a-cluster)

## Using UpUp to delete a cluster

To see a list of the resources that will be deleted:

```upup delete cluster --name <ClusterName> --region <Region>```

That will show you a list of resources that will be deleted; please double
check them before passing `--yes` to actually perform the deletion:

```upup delete cluster --name <ClusterName> --region <Region> --yes```

