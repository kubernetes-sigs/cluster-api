# Deleting a Kubernetes Cluster

If you would like to delete a kubernetes cluster, there are a few tools available to you:

* [Kops](#using-kops-to-delete-a-cluster)

## Using Kops to delete a cluster

To see a list of the resources that will be deleted:

```kops delete cluster --name <ClusterName> --region <Region>```

That will show you a list of resources that will be deleted; please double
check them before passing `--yes` to actually perform the deletion:

```kops delete cluster --name <ClusterName> --region <Region> --yes```

More information on kops can be found at the [main repo](https://github.com/kubernetes/kops)
