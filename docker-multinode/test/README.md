# Running e2e conformance tests

The purpose of this project is to easily fire up a VMs with a running Kubernetes cluster
and test machine to run e2e conformance tests.

## Prerequisites

- Vagrant 1.8.1+
- Ansible 1.9+

## Configuration

All global variables are stored in `group_vars/all.yaml` file.
You can setup:
 - master_ip - master IP address used in all VMs. This value depends on Vagrant network configuration in Vagrantfile.
 - use_cni - boolean flag indicates if CNI plugin must be used for cluster network.
 - k8s_version - sets released version of kubernetes which must be used in cluster.

## Start VMs

There are three VMs called: *master*, *node* and *tests*

To start each machine execute commands:

```
vagrant up master
vagrant up node
vagrant up tests
```

## Getting tests result

When all VMs are up and running you can check e2e tests result. Logs are stored in file
`/home/vagrant/e2e.txt` on `tests` VM. You can get the logs content using the following command:

```
vagrant ssh -c 'tail -f /home/vagrant/e2e.txt' tests
```
