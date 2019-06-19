#!/usr/bin/env bash

set -o errexit
set -o xtrace

REGISTRY=$(gcloud config get-value project)
TAG=${TAG:-latest}

IMAGE="gcr.io/${REGISTRY}/capd-manager:${TAG}"

docker build --file Dockerfile -t "${IMAGE}" .
gcloud docker -- push "${IMAGE}"