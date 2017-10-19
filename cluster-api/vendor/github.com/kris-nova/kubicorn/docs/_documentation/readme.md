---
layout: documentation
title: Project Walkthrough
date: 2017-10-18
doctype: general
---

## Introduction

This document explains how `kubicorn` project works, the project's structure and information you need to know so you can easily start contributing.

## Glossary

This chapter explains the most important concepts.

* [Cluster API](https://github.com/kris-nova/kubicorn/tree/master/apis) — universal (cloud provider agnostic) representation of a Kubernetes cluster. It defines every part of an cluster, including infrastructure parts such as virtual machines, networking units and firewalls. The matching of universal representation of the cluster to the representation for specific cloud provider is done as the part of Reconciling process.
* [State](https://github.com/kris-nova/kubicorn/blob/master/state/README.md) — representation of an specific Kubernetes cluster. It's used in reconciling process to create the cluster.
* [State store](https://github.com/kris-nova/kubicorn/blob/master/state/README.md) — place where state is stored. Currently, we only support states located on the disk and in [YAML](https://github.com/kris-nova/kubicorn/tree/master/state/fs) or [JSON](https://github.com/kris-nova/kubicorn/tree/master/state/jsonfs) format. We're looking forward to implementing Git and S3 state stores.
* [Reconciler](https://github.com/kris-nova/kubicorn/tree/master/cloud) — the core of the project and place where provision logic is located. It matches cloud provider agnostic Cluster definition to the specific cloud provider definition, which is used to provision cluster. It takes care of provisioning new clusters, destroying the old ones and keeping the consistency between Actual and Expected states.
* Actual state — the representation of current resources in the cloud.
* Expected state — the representation of intended resources in the cloud.
* [Bootstrap scripts](https://github.com/kris-nova/kubicorn/tree/master/bootstrap) — Bootstrap scripts are provided as the `user data` on the cluster creation to install dependencies and create the cluster. They're provided as Bash scripts, so you can easily create them without Go knowledge. You can also inject values in the reconciling process, per your needs.
* [VPN Boostrap scripts](https://github.com/kris-nova/kubicorn/tree/master/bootstrap/vpn) — to improve security of our cluster, we create VPN server on master, and connect every node using it. Some cloud providers, such as DigitalOcean, doesn't provide real private networking between Droplets, so we want master and nodes can only communicate between themselves, and with the Internet only on selected ports.
* [Profile](https://github.com/kris-nova/kubicorn/blob/master/profiles/README.md) — profile is a unique representation of a cluster written in Go. Profiles containts the all information needed to create an cluster, such as: cluster name, cloud provider, VM size, SSH key, network and firewall configurations...

## Project structure

Project is contained from the several packages. 

The most important package is the [`cloud`](https://github.com/kris-nova/kubicorn/tree/master/cloud) which contains Reconciler interface and Reconciler implementations for each cloud provider. Currently, we have implementations for three cloud providers: [Amazon](https://github.com/kris-nova/kubicorn/tree/master/cloud/amazon), [DigitalOcean](https://github.com/kris-nova/kubicorn/tree/master/cloud/digitalocean), [Google](https://github.com/kris-nova/kubicorn/tree/master/cloud/google) and [Azure in-development](https://github.com/kris-nova/kubicorn/pull/327).

The Cluster API is located in the [`apis`](https://github.com/kris-nova/kubicorn/tree/master/apis) package.

The Bootstrap Scripts are located in the [`bootstrap`](https://github.com/kris-nova/kubicorn/tree/master/bootstrap) directory of the project. It also contains [`vpn`](https://github.com/kris-nova/kubicorn/tree/master/bootstrap/vpn) sub-directory with VPN implementations.

Default profiles are located in the [`profiles`](https://github.com/kris-nova/kubicorn/tree/master/profiles) package. Currently, we have Ubuntu profiles available for Amazon, DigitalOcean and GCE, and CentOS profiles available for Amazon and DigitalOcean.

State store definitions are located in the [`state`](https://github.com/kris-nova/kubicorn/tree/master/state) package.

We have two type of tests — CI tests and E2E tests. CI tests are regular Go tests, while E2E tests are run against real cloud infrastucture and it can cost money. E2E tests are available in the [`test`](https://github.com/kris-nova/kubicorn/tree/master/test) package.

The [`cutil`](https://github.com/kris-nova/kubicorn/tree/master/cutil) directory contains many useful, helper packages which are used to do various tasks, such as: [copy the file from the VM](https://github.com/kris-nova/kubicorn/tree/master/cutil/scp), [create `kubeadm` token](https://github.com/kris-nova/kubicorn/tree/master/cutil/kubeadm), [logger implementation](https://github.com/kris-nova/kubicorn/tree/master/cutil/logger)...

The [`cmd`](https://github.com/kris-nova/kubicorn/tree/master/cmd) package is the CLI implementation for `kubicorn`. We use [`cobra`](https://github.com/spf13/cobra) package to create the CLI.

## Reconciler

This part of the document will try to summarize what steps are being taken in the reconciling process and how it works.

When you want to create a Kubernetes cluster, you're providing [Cluster API](https://github.com/kris-nova/kubicorn/tree/master/apis) representation of an cluster in form of the [Profile](https://github.com/kris-nova/kubicorn/blob/master/profiles/README.md).

The first task of Reconciler is to convert universal, cloud-provider agnostic representation of an cluster to representation for specific cloud provider. This transition is called **rendering** and is defined in the `model.go` file, as well as in the `immutableRender` function of the cloud-specific Reconciler. For example, this is how [`model.go`](https://github.com/kris-nova/kubicorn/blob/master/cloud/digitalocean/droplet/model.go) and Droplet's [`immutableRender`](https://github.com/kris-nova/kubicorn/blob/master/cloud/digitalocean/droplet/resources/droplet.go#L305) looks for DigitalOcean.

Once cluster is rendered, we can easily obtain **Expected state** of the cluster using the `Expected` function, which represents what resources we are expecting in the cloud. This is how [`Expected` function](https://github.com/kris-nova/kubicorn/blob/master/cloud/digitalocean/droplet/resources/droplet.go#L92) is defined for DigitalOcean Droplet's.

Next, Reconciler obtains the **Actual state** using the `Actual ` function, which represents what resources we already have in the cloud. You can take a look at [the following function](https://github.com/kris-nova/kubicorn/blob/master/cloud/digitalocean/droplet/resources/droplet.go#L55), which is used to obtain Actual state of DigitalOcean Droplets.

The most important task of Reconciler is **applying**, using the `Apply` function. Firstly, Reconciler is checking is cluster already existing, by comparing Actual and Expected states, and if yes, it's stopping there returning already existing one. If it doesn't exist, we are proceeding on the creation part, which is consisted of:

* [Building Bootstrap Scripts and injecting values at the runtime](https://github.com/kris-nova/kubicorn/blob/master/cloud/digitalocean/droplet/resources/droplet.go#L182-#L196),
* [Creating the appropriate resources for Master node](https://github.com/kris-nova/kubicorn/blob/master/cloud/digitalocean/droplet/resources/droplet.go#L203-#L227),
* [Obtaining needed information for Node creation](https://github.com/kris-nova/kubicorn/blob/master/cloud/digitalocean/droplet/resources/droplet.go#L124-L188),
* Creating Nodes,
* Downloading the `.kubeconfig` file for the cluster from the master node.

At this point, we have fully functional Kubernetes cluster.

Beside creating the cluster, Reconciler also takes care of destroying the cluster, which is done by the `Delete` function. [This is how it looks for DigitalOcean Droplet's](https://github.com/kris-nova/kubicorn/blob/master/cloud/digitalocean/droplet/resources/droplet.go#L247).

## Website

The website runs on Jekyll, straight from GitHub Pages. It consists of templates and markdown files that are automatically built into HTML pages whenever any of the content changes. 

The most common edits are changing the home.html files (our index page), and adding or editing files in the documentation/ folder.

Below are the relevant bits of the website's structure. Other files can be ignored. For a more detailed overview see [the Website Documentation document](http://kubicorn.io/documentation/website-documentation.html). Instructions for how to test your changes locally can be found there as well.

```
kubicorn/docs/
│
├── _documentation/
│ → All docs to be displayed on in the Documentation section of the website
│   should go here. They should be markdown, and include the YAML header as
│   shown in the link above.
│
├── _friends/
│ → Files in this folder feed the *Friends of kubicorn* section of the website.
│   You should follow the formatting as per the files already present.
│
├── _includes/
│ → This holds the includes for every page of the website: footer, header, and
│   head sections.
│
├── _layouts/
│   │ → This holds the different templates that make up the website. In
│   │   practice we're only using two:
│   │
│   ├── documentation.html
│   │ → This is the template used to generate all files in the documentation
│   │   section.
│   │
│   └── home.html
│     → This is the website's index page. Most of the text in the index page
│       is hard-coded here. Exceptions are the _friends/ content and the
│       _documentation/ list, which are generated dynamically.
│
├── img/
│ → All images go here.
│  
└── _site/
  → This folder contains the auto-generated files that the website serves. They
    are built automatically whenever anything else on the folders above change,
    and should not be edited manually. Any changes to these files will be
    discarded. 
```

## Where should I start?

If you need more help understanding the reconciling process, take a look at the [examples](https://github.com/kris-nova/kubicorn/tree/master/examples). It should explain you which steps are taken to create a Kubernetes cluster.

To be able to better understand how project works, you should read Reconciler part of this document and follow the links to see how code looks like.

Once you get familiar with the process, find the issue you want to work on.

Our [issue tracker](https://github.com/kris-nova/kubicorn/issues) has every issue labeled, so you can easily filter and navigate through it.

If you're new to the project, we recommend taking a look at [Hacktoberfest](https://github.com/kris-nova/kubicorn/labels/Hacktoberfest)-tagged issues. You can learn more about [Hacktoberfest on its official website](https://hacktoberfest.digitalocean.com/).

Beside Hacktoberfest label, you can filter by the following labels:

* [Help (small)](https://github.com/kris-nova/kubicorn/labels/Help%20%28small%29) -- containing issues which shouldn't require a lot of effort to be addressed.
* [Help (medium)](https://github.com/kris-nova/kubicorn/labels/Help%20%28medium%29) -- issues that could require you medium amount of time to get it addressed.
* [Help (hard)](https://github.com/kris-nova/kubicorn/labels/Help%20%28large%29) -- issues that require a lot of effort to get addressed.

We also label our issue per Cloud provider, operating system and type of the problem.

## Contributing guidelines

To get your Pull Request merged, you must follow the following [Contributing Guidelines](https://github.com/kris-nova/kubicorn/blob/master/CONTRIBUTING.md).

We will try to summarize it in this document, but you should read the above linked one:

* Your Go code must be formatted using the `go fmt` tool. This can be easily done using the `gofmt` make target.
* Your Go files must contain license headers. You can check are headers correct with the `check-headers` make target, and you can append them to the files where they're missing using the `update-headers` command.
* You should write unit tests where this is possible.
* Tests must pass.

## Why we are doing stuff this way?

In this document, we'll not explain reasoning and decisions involved in this project. If you are interested in the details, you should take a look at the [Cloud Native Infrastructure](http://shop.oreilly.com/product/0636920075837.do) book by Justin Garrison and Kris Nova.
Also, if you have any question, feel free to create an issue or ask us on the `kubicorn` channel at the [Gophers Slack](https://invite.slack.golangbridge.org/).
