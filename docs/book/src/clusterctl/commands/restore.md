# clusterctl restore

The `clusterctl restore` command allows to restore Cluster API objects and all dependencies from a management cluster.

You can use:

```shell
clusterctl restore --directory=/tmp/backup-directory
```

That will restore the objects in the management cluster and create the namespace if that does not exist.

To create a backup, please check [backup](backup.md)
