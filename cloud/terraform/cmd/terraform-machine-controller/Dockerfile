# Copyright 2018 The Kubernetes Authors.
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

# Reproducible builder image
FROM golang:1.10.0 as builder
WORKDIR /go/src/sigs.k8s.io/cluster-api
# This expects that the context passed to the docker build command is
# the cluster-api directory.
# e.g. docker build -t <tag> -f <this_Dockerfile> <path_to_cluster-api>
COPY . . 

RUN CGO_ENABLED=0 GOOS=linux go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api/cloud/terraform/cmd/terraform-machine-controller

# Final container
FROM debian:stretch-slim

ENV TERRAFORM_VERSION=0.11.7
ENV TERRAFORM_ZIP=terraform_${TERRAFORM_VERSION}_linux_amd64.zip
ENV TERRAFORM_SHA256SUM=6b8ce67647a59b2a3f70199c304abca0ddec0e49fd060944c26f666298e23418
ENV TERRAFORM_SHAFILE=terraform_${TERRAFORM_VERSION}_SHA256SUMS

RUN apt-get update && apt-get install -y ca-certificates curl openssh-server unzip && \
    curl https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/${TERRAFORM_ZIP} > ${TERRAFORM_ZIP} && \
    echo "${TERRAFORM_SHA256SUM}  ${TERRAFORM_ZIP}" > ${TERRAFORM_SHAFILE} && \
    sha256sum --quiet -c ${TERRAFORM_SHAFILE} && \
    unzip ${TERRAFORM_ZIP} -d /bin && \
    rm -f ${TERRAFORM_ZIP} ${TERRAFORM_SHAFILE} && \
    echo 'plugin_cache_dir = "$HOME/.terraform.d/plugin-cache"' >> ~/.terraformrc && \
    rm -rf /var/lib/apt/lists/*
    
COPY --from=builder /go/bin/terraform-machine-controller .
