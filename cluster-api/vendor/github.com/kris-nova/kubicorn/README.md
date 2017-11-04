<p align="center"><img src="docs/img/kubicorn-trans.png" width="360"></p>
<p align="center"><b>Create, manage, snapshot, and scale Kubernetes infrastructure in the public cloud.</b></p>
<p align="center">
  <a href="https://travis-ci.org/kris-nova/kubicorn"><img src="https://travis-ci.org/kris-nova/kubicorn.svg?branch=master" alt="Build Status"></img></a>
  <a href="https://goreportcard.com/report/github.com/kris-nova/kubicorn"><img src="https://goreportcard.com/badge/github.com/kris-nova/kubicorn" alt="Go Report Card"></img></a>
</p>

**Phonetic pronunciation**: `KEW - BHIK - OH - AR - IN`

## About

`kubicorn` is an **unofficial** project that solves the Kubernetes infrastructure problem and gives users a rich golang library to work with infrastructure.

`kubicorn` is a project that helps a user manage cloud infrastructure for Kubernetes.
With `kubicorn` a user can create new clusters, modify and scale them, and take a snapshot of their cluster at any time.

**NOTE:** This is a work-in-progress, we do not consider it production ready.
Use at your own risk and if you're as excited about it as we are, maybe you want to join us on the #kubicorn channel in the [Gophers Slack community](https://invite.slack.golangbridge.org/).


<img src="https://github.com/ashleymcnamara/gophers/blob/master/NERDY.png" width="60"> Proudly packaged with Golang [dep](https://github.com/golang/dep)

# Core Values

#### Community first.

This is a community driven project. We love you, and respect you. We are here to help you learn, help you grow, and help you succeed. If you have an idea, please share it.

#### Developer empathy. 

We are all software engineers, and we all work in many different code bases. We want the code to be stable, and approachable. We strive for clean and simple software, and we encourage refactoring and fixing technical debt.

#### Operational empathy. 

We want our tool to work, and work well. If an operator is running `kubicorn` it should feel comfortable and make sense to them. We want operators to feel empowered.

#### Infrastructure as software.

We believe that the oh-so important layer of infrastructure should be represented as software (not as code!). We hope that our project demonstrates this idea, so the community can begin thinking in the way of the new paradigm.

#### Rainbows and Unicorns 

We believe that sharing is important, and encouraging our peers is even more important. Part of contributing to `kubicorn` means respecting, encouraging, and welcoming others to the project. 

# Installing

```bash
$ go get github.com/kris-nova/kubicorn
``` 

..or read the [Install Guide](http://kubicorn.io/documentation/install.html).

## Quickstart

This asciicast shows how to get a Kubernetes cluster on Digital Ocean using kubicorn in less than 5 minutes:

[![asciicast](https://asciinema.org/a/7JKtK7RSNSjznOYpX1rOprRRq.png)](https://asciinema.org/a/7JKtK7RSNSjznOYpX1rOprRRq)

## How is Kubicorn different?

1) We use kubeadm to bootstrap our clusters
2) We strive for developer empathy, and clean and simple code
3) We strive for operational empathy, and clean and simple user experience
4) We start with struct literals for profiles, and then marshal into an object
5) We offer the tooling as a library, more than a command line tool
6) We are atomic, and will un-do any work if there is an error
7) We run on many operating systems
8) We allow users to define their own arbitrary bootstrap logic
9) We have no guarantee that anything works, ever, use at your own risk
10) We have no dependency on DNS
11) We believe in snapshots, and that a user should be able to capture a cluster, and move it

# Concepts

### Create

`kubicorn` lets a user create a Kubernetes cluster in a cloud of their choice.

### Apply

Define what you want, then apply it. That simple.

### Adopt

`kubicorn` can adopt any Kubernetes cluster at any time.

### Scale

`kubicorn` is powered by a state enforcement pattern.
A user defines the intended state of Kubernetes infrastructure, and `kubicorn` can enforce the intended state.

### Snapshot

`kubicorn` allows a user to take a snapshot of a Kubernetes cluster, and run the image in any cloud at any time.
A snapshot is compressed file that will represent intended infrastructure **and** intended application definitions.
Take a snap, save a snap, deploy a snap.

### Enforce

`kubicorn` is built as a library and a framework. Thus allowing it to be easily vendored into operator and controller patterns to enforce intended state of infrastructure.

# Documentation

| Name                                 | Description                                                 | Link                                                                            |
| ------------------------------------- | ----------------------------------------------------------- |:-------------------------------------------------------------------------------:|
| **Install**                           | Install guide for Kubicorn CLI                              | [install](docs/docs_old/INSTALL.md)                                                      |
| **Environmental Variables**           | Master list of supported environmental variables            | [envvars](docs/docs_old/envar.md)                                                        |
| **Kops vs Kubicorn**                  | Blog about kubicorn with comparison table                   | [nivenly.com/kubicorn](https://nivenly.com/kubicorn)                            |
| **Azure Walkthrough**                 | A walkthrough guide on installing Kubernetes on Azure    | [walkthrough](docs/docs_old/azure/walkthrough.md)      
| **AWS Walkthrough**                   | A walkthrough guide on installing Kubernetes on AWS     | [walkthrough](docs/docs_old/aws/walkthrough.md)                                          |
| **DigitalOcean Walkthrough**          | A walkthrough guide on installing Kubernetes on D.O.    | [walkthrough](docs/docs_old/do/walkthrough.md)                                           |
| **DigitalOcean Quickstart**          | A quickstart asciicast on installing Kubernetes on D.O.    | [asciinema](https://asciinema.org/a/7JKtK7RSNSjznOYpX1rOprRRq)                                           |
| **Google Compute Engine Walkthrough** | A walkthrough guide on installing Kubernetes on GCE     | [walkthrough](docs/docs_old/google/walkthrough.md)                                       |
| **AWS Video**                         | A step by step video of using Kubicorn in AWS               | [video](https://www.useloom.com/share/a0afd5034e654b0b8d6785a5fa8ec754)         |
| **DigitalOcean Video**                | A step by step video of using Kubicorn in DigitalOcean      | [video](https://youtu.be/XpxgSZ3dspE)         |
| **Tech N Talk Deep Dive**             | A technical deep dive courtesy of RedHat                    | [youtube](https://youtu.be/2DmUG0RgS70?list=PLaR6Rq6Z4IqfwXtKT7KeARRvxdvyLqG72) |
