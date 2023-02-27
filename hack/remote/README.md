
# TODOs:

* Test on MacOS

* go over all files in the diff, finalize + FIXME /TODOs

* Get it to work on Prow
  * Test & fixup GCP script

Backlog:
* Optimize scripting / automation
  * Implement in Go?
* MacOS: Debug why it crashes the local Docker Desktop
  * try without IPv6: docker network create -d=bridge -o com.docker.network.bridge.enable_ip_masquerade=true -o com.docker.network.driver.mtu=1500 --subnet=172.24.4.0/24 --gateway=172.24.4.1 kind
    * => re-add IPv6

# Setting up a Docker engine on AWS:

Prerequisites:
* AWS CLI must be installed & configured with credentials

Setup server on AWS with Docker engine:
```bash
./hack/remote/setup-docker-on-aws-account.sh
```

Note: The script can also be run repeatedly, e.g. to create the ssh tunnel when the server already exists. 

# Use remote Docker engine

## Docker CLI

```bash
export DOCKER_HOST=tcp://10.0.3.15:2375
docker version
docker info
```

## Local management cluster

### e2e tests via IDE

Prerequisites:
```bash
make generate-e2e-templates
make docker-build-e2e
```

Run configuration:
* Add to environment: `CAPD_DOCKER_HOST=tcp://10.0.3.15:2375`

### Tilt

tilt-settings.yaml:
```yaml
kustomize_substitutions:
  # Use remote Docker host in CAPD.
  CAPD_DOCKER_HOST: "tcp://10.0.3.15:2375"
```

```bash
tilt up
```

### Quickstart

```bash
export CAPD_DOCKER_HOST="tcp://10.0.3.15:2375"
```

## Remote management cluster

Create remote kind cluster:
```bash
# SSH to server
ssh-add ~/.ssh/aws-capi-docker
ssh cloud@${SERVER_PUBLIC_IP}
sudo su

# Note: this has to be run on the server.
# Running it locally will fails because 10.0.3.15 is not a valid IP there.
kind create cluster --name=capi-test --config=${HOME}/kind.yaml
```

### e2e tests via IDE

Prerequisites:
```bash
make generate-e2e-templates

# If local images are required (e.g. because code has changed)
export DOCKER_HOST=tcp://10.0.3.15:2375
make docker-build-e2e
kind  load docker-image --name=capi-test gcr.io/k8s-staging-cluster-api/cluster-api-controller-amd64:dev
kind  load docker-image --name=capi-test gcr.io/k8s-staging-cluster-api/kubeadm-bootstrap-controller-amd64:dev
kind  load docker-image --name=capi-test gcr.io/k8s-staging-cluster-api/kubeadm-control-plane-controller-amd64:dev
kind  load docker-image --name=capi-test gcr.io/k8s-staging-cluster-api/capd-manager-amd64:dev
kind  load docker-image --name=capi-test gcr.io/k8s-staging-cluster-api/test-extension-amd64:dev
```

Run configuration:
* Add to environment: `DOCKER_HOST=tcp://10.0.3.15:2375;CAPD_DOCKER_HOST=tcp://10.0.3.15:2375`
* Add to program arguments: `-e2e.use-existing-cluster=true`

### Tilt

tilt-settings.yaml:
```yaml
kustomize_substitutions:
  # Use remote Docker host in CAPD.
  CAPD_DOCKER_HOST: "tcp://10.0.3.15:2375"
```

```bash
export DOCKER_HOST=tcp://10.0.3.15:2375
tilt up
```

FIXME(sbueringer): enable local registry
* let's check if it is faster (as redeploy also just copies the binary over)
* copy&paste kind-install-for-capd.sh script over(?) (already done => just test it)
* ensure registry is reachable from local machine

## Getting access to workload clusters 

Retrieve kubeconfig for workload clusters via:
```bash
clusterctl get kubeconfig capi-quickstart > /tmp/kubeconfig
kubectl --kubeconfig /tmp/kubeconfig get no,po -A -A
```
Note: The kubeconfigs returned by `kind get kubeconfig` don't work.

# Troubleshooting

Verify connectivity:

```bash
# SSH to server
ssh-add ~/.ssh/aws-capi-docker
ssh cloud@${SERVER_PUBLIC_IP}

# On the server:
nc -l 10.0.3.15 8005

# Locally:
nc 10.0.3.15 8005
```

# Tested scenarios

* Local mgmt cluster:
  * Tilt:
    * works well
  * e2e tests (via Intellij):
    * works well
* Remote mgmt cluster:
  * Tilt:
    * loading images via kind load is slow
  * e2e tests (via Intellij):
    * building e2e images with make is quick
    * loading images with kind load is slow
