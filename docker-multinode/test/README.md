# Running e2e conformance tests

The purpose of this project is to easily fire up a VMs with a running Kubernetes cluster
and test machine to run e2e conformance tests.

## Prerequisites

- Vagrant 1.8.5+

Go to [Vagrant downloads page](https://www.vagrantup.com/downloads.html) and get the appropriate installer or package for your platform.

## Configuration

All global variables are stored in `enviroment.sh` file.
You can setup:
 - MASTER_IP - master IP address used in all VMs. This value depends on Vagrant network configuration in Vagrantfile.
 - USE_CNI - boolean flag indicates if CNI plugin must be used for cluster network.
 - K8S_VERSION - sets released version of kubernetes which must be used in cluster.

## Start VMs

There are three VMs called: *master*, *node* and *tests*

To start each machine execute commands:

```
vagrant up master
vagrant up node
vagrant up tests
```

### Libvirt provider

Test cluster can be started with libvirt provider. Go to [vagrant-libvirt-installation](https://github.com/vagrant-libvirt/vagrant-libvirt#installation)
for further instruction.

```
vagrant up master --provider=libvirt
vagrant up node --provider=libvirt
vagrant up tests --provider=libvirt
```

## Getting tests result

When all VMs are up and running you can check e2e tests result. Logs are stored in file
`/home/vagrant/e2e.txt` on `tests` VM. You can get the logs content using the following command:

```
vagrant ssh -c 'tail -f /home/vagrant/e2e.txt' tests
```
