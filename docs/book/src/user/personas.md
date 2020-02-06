# Personas

This document describes the personas for the Cluster API 1.0 project as driven
from use cases.

We are marking a “proposed priority for project at this time” per use case.
This is not intended to say that these use cases aren’t awesome or important.
They are intended to indicate where we, as a project, have received a great deal
of interest, and as a result where we think we should invest right now to get
the most users for our project. If interest grows in other areas, they will
be elevated. And, since this is an open source project, if you want to drive
feature development for a less-prioritized persona, we absolutely encourage
you to join us and do that.

## Use-case driven personas

### Service Provider: Managed Kubernetes

Managed Kubernetes is an offering in which a provider is automating the
lifecycle management of Kubernetes clusters, including full control planes
that are available to, and used directly by, the customer.

Proposed priority for project at this time: High

There are several projects from several companies that are building out
proposed managed Kubernetes offerings (Project Pacific’s Kubernetes Service
from VMware, Microsoft Azure, Google Cloud, Red Hat) and they have all
expressed a desire to use Cluster API. This looks like a good place to make
sure Cluster API works well, and then expand to other use cases.  

**Feature matrix**

|   |   |
|---|---|
| Is Cluster API exposed to this user? | Yes 
| Are control plane nodes exposed to this user? | Yes 
| How many clusters are being managed via this user? | Many 
| Who is the CAPI admin in this scenario? | Platform Operator 
| Cloud / On-Prem | Both
| Upgrade strategies desired? | Need to gather data from users 
| How does this user interact with Cluster API? | API
| ETCD deployment | Need to gather data from users 
| Does this user have a preference for the control plane running on pods vs. vm vs. something else? | Need to gather data from users

### Service Provider: Kubernetes-as-a-Service

Examples of a Kubernetes-as-a-Service provider include services such as
Red Hat’s hosted OpenShift, AKS, GKE, and EKS. The cloud services manage the
control plane, often giving those cloud resources away “for free,” and the
customers spin up and down their own worker nodes.

Proposed priority for project at this time: Medium

Existing Kubernetes as a Service providers, e.g. AKS, GKE have indicated
interest in replacing their off-tree automation with Cluster API, however
since they already had to build their own automation and it is currently
“getting the job done,” switching to Cluster API is not a top priority for
them, although it is desirable.

**Feature matrix**

|   |   |
|---|---|
| Is Cluster API exposed to this user? | Need to gather data from users  
| Are control plane nodes exposed to this user? | No
| How many clusters are being managed via this user? | Many 
| Who is the CAPI admin in this scenario? | Platform itself (AKS, GKE, etc.) 
| Cloud / On-Prem | Cloud
| Upgrade strategies desired? | tear down/replace (need confirmation from platforms) 
| How does this user interact with Cluster API? | API
| ETCD deployment | Need to gather data from users 
| Does this user have a preference for the control plane running on pods vs. vm vs. something else? | Need to gather data from users

### Cluster API Developer 

The Cluster API developer is a developer of Cluster API who needs tools and
services to make their development experience more productive and pleasant.
It’s also important to take a look at the on-boarding experience for new
developers to make sure we’re building out a project that other people can
more easily submit patches and features to, to encourage inclusivity and
welcome new contributors. 

Proposed priority for project at this time: Low

We think we’re in a good place right now, and while we welcome contributions
to improve the development experience of the project, it should not be the
primary product focus of the open source development team to make development
better for ourselves.

**Feature matrix**

|   |   |
|---|---|
| Is Cluster API exposed to this user? | Yes 
| Are control plane nodes exposed to this user? | Yes 
| How many clusters are being managed via this user? | Many 
| Who is the CAPI admin in this scenario? | Platform Operator 
| Cloud / On-Prem | Both
| Upgrade strategies desired? | Need to gather data from users 
| How does this user interact with Cluster API? | API
| ETCD deployment | Need to gather data from users 
| Does this user have a preference for the control plane running on pods vs. vm vs. something else? | Need to gather data from users

### Raw API Consumers

Examples of a raw API consumer is a tool like Prow, a customized enterprise
platform built on top of Cluster API, or perhaps an advanced “give me a
Kubernetes cluster” button exposing some customization that is built using
Cluster API. 

Proposed priority for project at this time: Low

**Feature matrix**

|   |   |
|---|---|
| Is Cluster API exposed to this user? | Yes 
| Are control plane nodes exposed to this user? | Yes 
| How many clusters are being managed via this user? | Many 
| Who is the CAPI admin in this scenario? | Platform Operator 
| Cloud / On-Prem | Both
| Upgrade strategies desired? | Need to gather data from users 
| How does this user interact with Cluster API? | API
| ETCD deployment | Need to gather data from users 
| Does this user have a preference for the control plane running on pods vs. vm vs. something else? | Need to gather data from users

### Tooling: Provisioners

Examples of this use case, in which a tooling provisioner is using
Cluster API to automate behavior, includes tools such as kops and kubicorn.

Proposed priority for project at this time: Low

Maintainers of tools such as kops have indicated interest in using
Cluster API, but they have also indicated they do not have much time to
take on the work. If this changes, this use case would increase in priority.

**Feature matrix**

|   |   |
|---|---|
| Is Cluster API exposed to this user? | Need to gather data from tooling maintainers
| Are control plane nodes exposed to this user? | Yes 
| How many clusters are being managed via this user? | One (per execution) 
| Who is the CAPI admin in this scenario? | Kubernetes Platform Consumer 
| Cloud / On-Prem | Cloud 
| Upgrade strategies desired? | Need to gather data from users 
| How does this user interact with Cluster API? | CLI 
| ETCD deployment | (Stacked or external) AND new 
| Does this user have a preference for the control plane running on pods vs. vm vs. something else? | Need to gather data from users

### Service Provider: End User/Consumer 

This user would be an end user or consumer who is given direct access to
Cluster API via their service provider to manage Kubernetes clusters.
While there are some commercial projects who plan on doing this (Project
Pacific, others), they are doing this as a “super user” feature behind the
backdrop of a “Managed Kubernetes” offering. 

Proposed priority for project at this time: Low

This is a use case we should keep an eye on to see how people use Cluster API
directly, but we think the more relevant use case is people building managed
offerings on top at this top. 

**Feature matrix**

|   |   |
|---|---|
| Is Cluster API exposed to this user? | Yes 
| Are control plane nodes exposed to this user? | Yes 
| How many clusters are being managed via this user? | Many 
| Who is the CAPI admin in this scenario? | Platform Operator 
| Cloud / On-Prem | Both
| Upgrade strategies desired? | Need to gather data from users 
| How does this user interact with Cluster API? | API
| ETCD deployment | Need to gather data from users 
| Does this user have a preference for the control plane running on pods vs. vm vs. something else? | Need to gather data from users
