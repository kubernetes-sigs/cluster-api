# Kubicorn and ARK integration

## Introduction
This document proposes integration between Kubicorn and ARK, Kubicorn provides infrastructure management and ARK provides Kubernetes cluster resources and applications backup and recovery mechanism. Both the tools are recent addition to the Kubernetes ecosystem and are still evolving. Users of either tools are looking for better integrations, which can provide a seamless solution for backup / restore of the complete infra + apps.


## Backup Scenarios

Two Scenarios for doing backups, Kubernetes cluster resources, applications (ARK) and infrastructure (Kubicorn)

**Scenario 1** - Backup for any existing Kubernetes cluster existing applications and infrastructure:
ARK provides you with a mechanism to snapshot with highly customizable settings. We can add this functionality to Kubicorn, given any running Kubernetes cluster. User should be able to run something like -

                `kubicorn backup -f <kube.cofig>`

Kubicorn at this point can install ARK and Kubicorn components in the cluster, which can backup the Kubernetes configuration and applications. Kubicorn can take the infrastructure snapshot. Kubicorn can use Cloud metadata to get VM infrastructure information (in AWS and GCE its possible). Once both the snapshots are done, user can edit the configuration at this point (optional) and restore the complete cluster in same or different cloud.

**Scenario 2** - For clusters created using Kubicorn, Kubicorn already has all the info to access recreate the cluster. If someone at any point wants to backup the Kubernetes configuration and applications, they can use 

                `kubicorn backup <profileName> -b <backupLocation(optional)> `

Kubicorn now same as earlier will install ARK and Kubicorn components (optional) in the cluster, which can perform the backup. 

> Note: Kubicorn backup command should support flags like --selector, which are used in Ark client.

## State Store

At this moment state is being stored on the disk in Kubicorn, but there are plans to enhance this. Add more options to store the state like S3, Git etc. 

Ark supports the object storage for specific cloud providers, which seems like a good approach also for Kubicorn.

Kubicorn and Ark backups created should stay in different files / tars (may be) and can be saved together to the filesystem to start with. We can slowly add support for Cloud storage.

## Restore / Apply scenarios

**Scenario 1** - Restore an application or Config in a Kubernetes cluster, the back up could have been taken in a different Kubernetes cluster or same cluster, shouldn’t matter.

No existing Ark running in the Cluster where backup needs to be restored. It’s also possible the cluster is not created by Kubicorn. User can do this -

                `kubicorn apply -f <kubeconfig> -b <backupLocation>`

Kubicorn can create Ark components and request to restore the backup. In case, cluster is created using Kubicorn, only -b flag shoulld be required.


**Scenario 2** - Restore the complete infrastructure, config and applications to any Cloud.
In this case user can do something like this - 
            
                `kubicorn apply <profileName> -b <backupLocation>`

Kubicorn will create infrastructure, deploy Ark and request Ark server to restore the back
