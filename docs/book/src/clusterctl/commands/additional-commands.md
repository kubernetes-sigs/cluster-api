# clusterctl backup

Backup Cluster API objects and all dependencies from a management cluster.

# clusterctl config repositories

Display the list of providers and their repository configurations.

clusterctl ships with a list of known providers; if necessary, edit
$HOME/.cluster-api/clusterctl.yaml file to add a new provider or to customize existing ones.

# clusterctl help

Help provides help for any command in the application.
Simply type `clusterctl help [command]` for full details.

# clusterctl restore

Restore Cluster API objects from file by glob. Object files are searched in the default config directory
or in the provided directory.

# clusterctl version

Print clusterctl version.

# clusterctl init list-images

Lists the container images required for initializing the management cluster.