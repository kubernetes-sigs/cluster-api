# Cert-manager

This extension deploys cert-manager.

## Usage

Basic usage

```
load('ext://cert_manager', 'deploy_cert_manager')

deploy_cert_manager()
```

This will deploy cert-manager to you cluster and checks it actually works.

If working with Kind, its is possible to pass `load_to_kind=True` to `deploy_cert_manager` so
all the cert-manager images will be pre-pulled to your local environment and then loaded into Kind before installing. 
This speeds up your workflow if you're repeatedly destroying and recreating your kind cluster, as it doesn't
have to pull the images over the network each time.

The full list of parameters accepted by `deploy_cert_manager` includes:
- `registry` from which images should be pulled, defaults to `quay.io/jetstack`
- `version` of cert-manager to install, defaults to `v0.16.1`
- `load_to_kind` (see above), defaults to `False`
- `kind_cluster_name`, defaults to `kind`
