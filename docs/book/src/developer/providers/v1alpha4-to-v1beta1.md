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
