# Cluster API v1alpha4 compared to v1beta1

## Minimum Go version

- The Go version used by Cluster API is still Go 1.16+

## Controller Runtime version

- The Controller Runtime version is now v0.10.+

## Controller Tools version (if used)

- The Controller Tools version is now v0.7.+

## Kind version

- The KIND version used for this release is still v0.11.x

## Conversion from v1alpha3 and v1alpha4 to v1beta1 types

The core ClusterAPI providers will support upgrade from v1alpha3 **and** v1alpha4 to v1beta1. Thus, conversions of API types 
from v1alpha3 and v1alpha4 to v1beta1 have been implemented. If other providers also want to support the upgrade from v1alpha3 **and**
v1alpha4, the same conversions have to be implemented.

## Removed items

### API Fields 

- **ClusterTopologyLabelName**, a ClusterClass related constant has been deprecated and removed. This label has been replaced by `ClusterTopologyOwnedLabel`.

- **MachineNodeNameIndex** has been removed from the common types in favor of `api/v1beta1/index.MachineNodeNameField`.

- **MachineProviderNameIndex** has been removed from common types in favor of `api/v1beta1/index.MachineProviderIDField`.

### Clusterctl

- **clusterctl config provider** has been removed in favor of `clusterctl generate provider`.

- **clusterctl config cluster** has been removed in favor of `clusterctl generate cluster`.

### Utils and other

- **TemplateSuffix** has been removed in favor of `api/v1alpha4.TemplatePrefix`.
- **AddMachineNodeIndex** has been removed in favor of `api/v1alpha4/index.ByMachineNode`
- **GetMachineFromNode** has been removed. This functionality is now private in the controllers package.
- **ConverReferenceAPIContract** has been removed in favor of `UpdateReferenceAPIContract` in the util/conversion package.
- **ParseMajorMinorPatch** has been removed in favor of `ParseMajorMinorPatch` in the util/version package.
- **GetMachinesForCluster** has been removed in favor of `GetFilteredMachinesForCluster` in the util/collection package.
- **GetControlPlaneMachines** has been removed in favor of `FromMachines(machine).Filter(collections.ControlPlaneMachines(cluster.Name))`  in the util/collection package.
- **GetControlPlaneMachinesFromList** has been removed in favor of `FromMachineList(machines).Filter(collections.ControlPlaneMachines(cluster.Name))` in the util/collection package. 
- **GetCRDMetadataFromGVK** has been removed in favor of `GetGVKMetadata`.

