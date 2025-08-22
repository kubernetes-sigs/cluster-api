# Updating Machine Infrastructure and Bootstrap Templates

## Updating Infrastructure Machine Templates

Several different components of Cluster API leverage _infrastructure machine templates_,
including `KubeadmControlPlane`, `MachineDeployment`, and `MachineSet`. These
`MachineTemplate` resources should be immutable, unless the infrastructure provider
documentation indicates otherwise for certain fields (see below for more details).

The correct process for modifying an infrastructure machine template is as follows:

1. Duplicate an existing template.
    Users can use `kubectl get <MachineTemplateType> <name> -o yaml > file.yaml`
    to retrieve a template configuration from a running cluster to serve as a starting
    point.
2. Update the desired fields.
    Fields that might need to be modified could include the SSH key, the AWS instance
    type, or the Azure VM size. Refer to the provider-specific documentation
    for more details on the specific fields that each provider requires or accepts.
3. Give the newly-modified template a new name by modifying the `metadata.name` field
    (or by using `metadata.generateName`).
4. Create the new infrastructure machine template on the API server using `kubectl`.
    (If the template was initially created using the command in step 1, be sure to clear
    out any extraneous metadata, including the `resourceVersion` field, before trying to
    send it to the API server.)

Once the new infrastructure machine template has been persisted, users may modify
the object that was referencing the infrastructure machine template. For example,
to modify the infrastructure machine template for the `KubeadmControlPlane` object,
users would modify the `spec.infrastructureTemplate.name` field. For a `MachineDeployment`, users would need to modify the `spec.template.spec.infrastructureRef.name`
field and the controller would orchestrate the upgrade by managing `MachineSets` pointing to the new and old references. In the case of a `MachineSet` with no `MachineDeployment` owner, if its template reference is changed, it will only affect upcoming Machines.

In all cases, the `name` field should be updated to point to the newly-modified
infrastructure machine template. This will trigger a rolling update. (This same process
is described in the documentation for [upgrading the underlying machine image for
KubeadmControlPlane](./control-plane/kubeadm-control-plane.md) in the "How to upgrade the underlying
machine image" section.)

Some infrastructure providers _may_, at their discretion, choose to support in-place
modifications of certain infrastructure machine template fields. This may be useful
if an infrastructure provider is able to make changes to running instances/machines,
such as updating allocated memory or CPU capacity. In such cases, however, Cluster
API **will not** trigger a rolling update.

## Updating Bootstrap Templates

Several different components of Cluster API leverage _bootstrap templates_,
including `MachineDeployment`, and `MachineSet`. When used in `MachineDeployment` or 
`MachineSet` changes to those templates do not trigger rollouts of already existing `Machines`.
New `Machines` are created based on the current version of the bootstrap template.

The correct process for modifying a bootstrap template is as follows:

1. Duplicate an existing template.
   Users can use `kubectl get <BootstrapTemplateType> <name> -o yaml > file.yaml`
   to retrieve a template configuration from a running cluster to serve as a starting
   point.
2. Update the desired fields.
3. Give the newly-modified template a new name by modifying the `metadata.name` field
   (or by using `metadata.generateName`).
4. Create the new bootstrap template on the API server using `kubectl`.
   (If the template was initially created using the command in step 1, be sure to clear
   out any extraneous metadata, including the `resourceVersion` field, before trying to
   send it to the API server.)

Once the new bootstrap template has been persisted, users may modify
the object that was referencing the bootstrap template. For example,
to modify the bootstrap template for the `MachineDeployment` object,
users would modify the `spec.template.spec.bootstrap.configRef.name` field.
The `name` field should be updated to point to the newly-modified
bootstrap template. This will trigger a rolling update.
