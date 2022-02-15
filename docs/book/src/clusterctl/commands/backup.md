# clusterctl backup

The `clusterctl backup` command allows to backup Cluster API objects and all dependencies from a management cluster.

You can use:

```shell
clusterctl backup --directory=/tmp/backup-directory
```

That will back the current namespace and output the objects to the destination directory.

To restore a backup, please check [restore](restore.md)
