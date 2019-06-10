# Cluster API Provider Kind

A temporary home for CAPK

## Building the binaries

Requires go 1.12? Probably less strict than that.

* `go build ./cmd/...`
* `go build ./cmd/`

## Building the image

Requires `gcloud` authenticated and configured.

Requires a google cloud project

`./scripts/publish-capk-manager.sh`

