---
title: Enable mounting etcd on a data disk
authors:
  - "@CecileRobertMichon"
reviewers:
  - "@bagnaram"
  - "@vincepri"
  - “@detiber”
  - "@fabrizio.pandini"
creation-date: 2020-04-23
last-updated: 2020-05-11
status: implementable
---

# Enable mounting etcd on a data disk

## Table of Contents

- [Table of Contents](#table-of-contents)
- [Glossary](#glossary)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals/Future Work](#non-goalsfuture-work)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
  - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
- [Upgrade Strategy](#upgrade-strategy)

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

## Summary

CAPZ issue that motivated this proposal: [Re-evaluate putting etcd in a data disk](https://github.com/kubernetes-sigs/cluster-api-provider-azure/issues/448).

Currently we deploy etcd on the OS disk for the VM instance. To avoid potential issues related to cached bandwidth/IOPS available on the VM, we should consider moving etcd to its own storage device. This might seem like an infrastructure-only change at first glance. However, in order to implement this proposal, we would need to mount the etcd disk.

This is one of the cases where infrastructure and bootstrapping overlap so we need to ensure we set the right precedent for future cases. In addition, most infrastructure will face a similar challenge so a common solution would be beneficial. One possible workaround to achieve this without any modifications to CABPK would be to leverage `preKubeadmCommands` to insert a script that does the mounting or to append a cloud init section to the bootstrap data to mount the disk right before setting the instance user data in the infrastructure provider. This is not a desired outcome however, because this would mean either 1) using  `preKubeadmCommands` for something infrastructure specific that is required for all clusters, whereas `preKubeadmCommands` is meant for user customization, or 2) making an assumption in the infrastructure provider about the content and format of the bootstrap data.

## Motivation

The main motivation of this proposal is to allow running etcd on a data disk to improve performance and reduce throttling.

A "side effect" motivation is to open up the option of putting other directories such as  /var/lib/containerd and /var/log onto separate partitions as well. This would also make upgrades to the OS and/or repairing a broken OS install much easier if all the important data is located on a data disk.

References:
https://docs.microsoft.com/en-us/archive/blogs/xiangwu/azure-vm-storage-performance-and-throttling-demystify
https://docs.d2iq.com/mesosphere/dcos/1.13/installing/production/system-requirements/azure-recommendations/#disk-configurations

### Goals

- Allow running etcd on its own disk
- Provide a solution that is reusable across infra providers
- Avoid tying infrastructure providers to cloud init or a specific OS
- To maintain backwards compatibility and cause no impact for users who don't intend to make use of this capability
- Provide a generic solution that can be used to put other data directories on data disks as well

### Non-Goals/Future Work

- Putting docker or any other component data on its own disk
- External etcd

## Proposal

The proposal is to modify CABPK to enable creating partitions and mounts as part of cloud-init. This would follow a similar pattern to the already available user configurable NTP and Files. The main benefit of this is that the disk setup and mount points options would be generic and reusable for other purposes besides mounting the etcd data directory.

### User Stories

#### Story 1

As an operator of a Management Cluster, I want to avoid potential issues related to cached bandwidth/IOPS of the etcd disk on my workload clusters.

#### Story 2

As a user of a Workload Cluster, I want provision and mount additional data storage devices for my application data.

### Implementation Details/Notes/Constraints

### Changes required in the bootstrap provider (ie. CABPK)

1. Add a two new fields to KubeadmConfig for disk setup and mount points

```go
   // DiskSetup specifies options for the creation of partition tables and file systems on devices.
   // +optional
   DiskSetup *DiskSetup `json:"diskSetup,omitempty"`

   // Mounts specifies a list of mount points to be setup.
   // +optional
   Mounts []MountPoints `json:"mounts,omitempty"`

   // DiskSetup defines input for generated disk_setup and fs_setup in cloud-init.
   type DiskSetup struct {
       // Partitions specifies the list of the partitions to setup.
       Partitions  []Partition  `json:"partitions,omitempty"`
       Filesystems []Filesystem `json:"filesystems,omitempty"`
   }

   // Partition defines how to create and layout a partition.
type Partition struct {
   Device string `json:"device"`
   Layout bool   `json:"layout"`
   // +optional
   Overwrite *bool `json:"overwrite,omitempty"`
   // +optional
   TableType *string `json:"tableType,omitempty"`
}

// Filesystem defines the file systems to be created.
type Filesystem struct {
   Device     string `json:"device"`
   Filesystem string `json:"filesystem"`
   // +optional
   Label *string `json:"label,omitempty"`
   // +optional
   Partition *string `json:"partition,omitempty"`
   // +optional
   Overwrite *bool `json:"overwrite,omitempty"`
   // +optional
   ReplaceFS *string `json:"replaceFS,omitempty"`
   // +optional
   ExtraOpts []string `json:"extraOpts,omitempty"`
}

   // MountPoints defines input for generated mounts in cloud-init.
   type MountPoints []string
```

2. Add templates for disk_setupm fs_setup, and mounts and add those to controlPlaneCloudInit and controlPlaneJoinCloudInit

references:
https://cloudinit.readthedocs.io/en/latest/topics/examples.html#disk-setup
https://cloudinit.readthedocs.io/en/latest/topics/examples.html#adjust-mount-points-mounted

### Changes required in the infrastructure provider (here Azure is used as an example to illustrate the required changes). 

These changes are required to run etcd on a data device but it should be noted that someone could use the KubeadmConfig changes described above without making any changes to infrastructure.

1. Add EtcdDisk optional field in AzureMachineSpec. For now, this will only have DiskSizeGB to specify the disk size but we can envision expanding this struct to allow further customization in the future.

```go
// DataDisk specifies the parameters that are used to add one or more data disks to the machine.
type DataDisk struct {
  // NameSuffix is the suffix to be appended to the machine name to generate the disk name.
  // Each disk name will be in format <machineName>_<nameSuffix>.
  NameSuffix string `json:"nameSuffix"`
  // DiskSizeGB is the size in GB to assign to the data disk.
  DiskSizeGB int32 `json:"diskSizeGB"`
  // Lun Specifies the logical unit number of the data disk. This value is used to identify data disks within the VM and therefore must be unique for each data disk attached to a VM.
  Lun int32 `json:"lun"`
}
```

2. Provision a data disk for each control plane

```go
   dataDisks := []compute.DataDisk{}
   for _, disk := range vmSpec.DataDisks {
       dataDisks = append(dataDisks, compute.DataDisk{
           CreateOption: compute.DiskCreateOptionTypesEmpty,
           DiskSizeGB:   to.Int32Ptr(disk.DiskSizeGB),
           Lun:          to.Int32Ptr(disk.Lun),
           Name:         to.StringPtr(azure.GenerateDataDiskName(vmSpec.Name, disk.NameSuffix)),

       })
   }
   storageProfile.DataDisks = &dataDisks
```

3. Modify the base cluster-template to specify the new KubeadmConfigSpec options above

```yaml
   diskSetup:
     partitions:
       - device: /dev/disk/azure/scsi1/lun0
         tableType: gpt
         layout: true
         overwrite: false
     filesystems:
       - label: etcd_disk
         filesystem: ext4
         device: /dev/disk/azure/scsi1/lun0
         extraOpts:
           - "-F"
           - "-E"
           - "lazy_itable_init=1,lazy_journal_init=1"
   mounts:
     - - etcd_disk
       - /var/lib/etcd
```

4. Prepend `rm -rf /var/lib/etcd/*` to preKubeadmCommand to remove `lost+found` from `/var/lib/etcd` otherwise kubeadm will complain that etcd data dir is not empty and fail (see https://github.com/kubernetes/kubeadm/issues/2127).

### Risks and Mitigations

- Is there anything Azure specific in the cloud init implementation and script below that won't work for other providers? 
A: No, see: https://cloudinit.readthedocs.io/en/latest/topics/examples.html#disk-setup
https://cloudinit.readthedocs.io/en/latest/topics/examples.html#adjust-mount-points-mounted

- If the data dir already exists, will kubeadm complain? While testing a prototype, I noticed kubeadm init failed if the data dir was not empty. Still need more investigation.
A: No, but it will fail if that data dir is not empty. For now, a workaround is to prepend `rm -rf /var/lib/etcd/*` to preKubeadmCommands in CAPZ (see changes required above).

- How much data will this add to UserData? In many infra providers, user data size is very restricted. See https://github.com/kubernetes-sigs/cluster-api/pull/2763#discussion_r397306055 for previous discussion on the topic.
A: Alternatives 2.1 would theoretically include snippets for disk layout, format, & mount.

- CAPZ: what should the default etcd disk size be? Right now I'm thinking 256GB but ideally it should depend on the number of nodes. https://etcd.io/docs/v3.3.12/op-guide/hardware/

- The default behaviour for etcd will need to be on the OS disk rather than its own etcd data disk in order to maintain backwards compatibility (until fully automated). For example, if the current UX requires the user to modify the cluster template, this will break current and previous workflows for CAPI. How can etcd data disk fields in the cluster template be made optional?
	A: All the fields added are 100% optional so a template without etcd data disks specified should still work with etcd on the OS disk. It's up to each provider to decide what they want to put in their "default" template. The provider can also leverage clusterctl flavors to have an OS disk and a data disk flavor.

- Adding the disk configuration to the cluster template as part of Kubeadm Config spec has the downside that the user could remove it. What would happen then?
	A: the data disk resource would still be created but the etcd data dir wouldn't get mounted which means etcd data would live on the OS disk. The cluster would be functional but the performance would be reduced (equivalent to what it is now). Do we want users to be able to change the configuration and risk shooting themselves in the foot? One thing to consider is that we already do that. There are some things in the cluster template that are "required" in the sense that the cluster creation will fail if the user removes them, for example "JoinConfiguration" in KubeadmConfigSpec.

- In Azure (might be the same for other providers?), the device name may not be persistent if there is more than one disk. https://docs.microsoft.com/en-us/azure/virtual-machines/troubleshooting/troubleshoot-device-names-problems#identify-disk-luns. 
In order to workaround that problem, the infra provider will have to find a way to refer to the device that is persistent. In Azure, we can use `/dev/disk/azure/scsi1/lun0` as the device in diskSetup and the file system label (`etcd_disk`)  in mounts. The caveat is that the LUN needs to match the data disk lun in infrastructure. This means that there is a dependency between how the data disks are specified and what the device should be in the bootstrap config. This goes back to the overall challenge of separating bootstrapping from infrastructure. In this case, there is a circular dependency: bootstrapping depends on infrastructure to determine which device should be used as etcd in cloud-init, but infrastructure depends on the bootstrap provider to generate userData for VMs. The way that we solve this for now is by putting the burden on the user (via templates) to reconcile the two. However, we should think about this problem (probably out of scope for this particular proposal) overall as a Cluster API design challenge: what happens when the bootstrap provider and infrastructure provider need to talk? Other examples of this are how to communicate bootstrap success/failure, how to deal with custom VM images during KCP upgrade, etc. 

## Alternatives

These alternatives do not require any changes to CABPK.

### Use script to do the etcd mount and append that script to preKubeadmCommands

Script eg. :

```bash
set -x
DISK=/dev/sdc
PARTITION=${DISK}1
MOUNTPOINT=/var/lib/etcd
udevadm settle
mkdir -p $MOUNTPOINT
if mount | grep $MOUNTPOINT; then
 echo "disk is already mounted"
 exit 0
fi
if ! grep "/dev/sdc1" /etc/fstab; then
 echo "$PARTITION       $MOUNTPOINT       auto    defaults,nofail       0       2" >>/etc/fstab
fi
if ! ls $PARTITION; then
 /sbin/sgdisk --new 1 $DISK
 /sbin/mkfs.ext4 $PARTITION -L etcd_disk -F -E lazy_itable_init=1,lazy_journal_init=1
fi
mount $MOUNTPOINT
/bin/chown -R etcd:etcd $MOUNTPOINT
```

### Use Cloud init to mount the data dir but modify bootstrap data in the infrastructure provider before passing to the instance user data

Add this to the control plane init and join cloud init templates:

```yaml
disk_setup:
 /dev/sdc:
   table_type: gpt
   layout: true
   overwrite: false

fs_setup:
- label: etcd_disk
 filesystem: ext4
 device: /dev/sdc1
 extra_opts:
   - "-F"
   - "-E"
   - "lazy_itable_init=1,lazy_journal_init=1"

mounts:
- - /dev/sdc1
 - /var/lib/etcd
```

### Instrument the OS image with image-builder to perform this customization automatically through custom UDEV rules, scripts, etc.

## Upgrade Strategy

N/A as this proposal only adds new types.
