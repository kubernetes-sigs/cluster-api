Running Multi-Node Kubernetes Using Docker
------------------------------------------

## Prerequisites

The only thing you need is a linux machine with **Docker 1.10.0 or higher**

## Overview

This guide will set up a 2-node Kubernetes cluster, consisting of a _master_ node which hosts the API server and orchestrates work
and a _worker_ node which receives work from the master. You can repeat the process of adding worker nodes an arbitrary number of
times to create larger clusters.

Here's a diagram of what the final result will look like:
![Kubernetes Single Node on Docker](k8s-docker.png)

### Bootstrap Docker

This guide uses a pattern of running two instances of the Docker daemon:
   1) A _bootstrap_ Docker instance which is used to start `etcd` and `flanneld`, on which the Kubernetes components depend
   2) A _main_ Docker instance which is used for the Kubernetes infrastructure and user's scheduled containers

This pattern is necessary because the `flannel` daemon is responsible for setting up and managing the network that interconnects
all of the Docker containers created by Kubernetes. To achieve this, it must run outside of the _main_ Docker daemon. However,
it is still useful to use containers for deployment and management, so we create a simpler _bootstrap_ daemon to achieve this.


### Multi-arch solution

Yeah, it's true. You may run this deployment setup seamlessly on `amd64`, `arm`, `arm64` and `ppc64le` hosts.
See this tracking issue for more details: https://github.com/kubernetes/kubernetes/issues/17981

(Small note: arm is available with `v1.3.0-alpha.2+`. arm64 and ppc64le are available with `v1.3.0-alpha.3+`. Also ppc64le encountered problems in `v1.3.0-alpha.5+`, so ppc64le has only `v1.3.0-alpha.(3|4)` images pushed)

### Options/configuration

The scripts will output something like this when starting:

```console
+++ [0611 12:50:12] K8S_VERSION is set to: v1.2.4
+++ [0611 12:50:12] ETCD_VERSION is set to: 2.2.5
+++ [0611 12:50:12] FLANNEL_VERSION is set to: 0.5.5
+++ [0611 12:50:12] FLANNEL_IPMASQ is set to: true
+++ [0611 12:50:12] FLANNEL_NETWORK is set to: 10.1.0.0/16
+++ [0611 12:50:12] FLANNEL_BACKEND is set to: udp
+++ [0611 12:50:12] DNS_DOMAIN is set to: cluster.local
+++ [0611 12:50:12] DNS_SERVER_IP is set to: 10.0.0.10
+++ [0611 12:50:12] RESTART_POLICY is set to: unless-stopped
+++ [0611 12:50:12] MASTER_IP is set to: 192.168.1.50
+++ [0611 12:50:12] ARCH is set to: amd64
+++ [0611 12:50:12] NET_INTERFACE is set to: eth0
```

Each of these options are overridable by `export`ing the values before running the script.

## Setup the master node

The first step in the process is to initialize the master node.

Clone the `kube-deploy` repo, and run [master.sh](master.sh) on the master machine _with root_:

```console
$ git clone https://github.com/kubernetes/kube-deploy
$ cd docker-multinode
$ ./master.sh
```

First, the `bootstrap` docker daemon is started, then `etcd` and `flannel` are started as containers in the bootstrap daemon.
Then, the main docker daemon is restarted, and this is an OS/distro-specific tasks, so if it doesn't work for your distro, feel free to contribute!

Lastly, it launches `kubelet` in the main docker daemon, and the `kubelet` in turn launches the control plane (apiserver, controller-manager and scheduler) as static pods.

## Adding a worker node

Once your master is up and running you can add one or more workers on different machines.

Clone the `kube-deploy` repo, and run [worker.sh](worker.sh) on the worker machine _with root_:

```console
$ git clone https://github.com/kubernetes/kube-deploy
$ cd docker-multinode
$ export MASTER_IP=${SOME_IP}
$ ./worker.sh
```

First, the `bootstrap` docker daemon is started, then `flannel` is started as a container in the bootstrap daemon, in order to set up the overlay network.
Then, the main docker daemon is restarted and lastly `kubelet` is launched as a container in the main docker daemon.

## Addons

kube-dns and the dashboard are deployed automatically with `v1.3.0-alpha.5` and over
