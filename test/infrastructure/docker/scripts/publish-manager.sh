#!/usr/bin/env bash
# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

readonly GCP_PROJECT=$(gcloud config get-value project)

set -o errexit
set -o xtrace

readonly IMAGE_NAME="manager"

readonly GCR_REGISTRY="gcr.io/${GCP_PROJECT}"
readonly TAG=${TAG:-dev}

readonly REGISTRY=${REGISTRY:-$GCR_REGISTRY}

readonly IMAGE=${REGISTRY}/${IMAGE_NAME}:${TAG}

docker build --file Dockerfile -t "${IMAGE}" .
docker push "${IMAGE}"
