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

RUN CGO_ENABLED=0 GOOS=linux go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api/cloud/vsphere/cmd/vsphere-machine-controller

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
    rm -rf /var/lib/apt/lists/*

# Setup template provider
ENV TEMPLATE_PROVIDER_VERSION=1.0.0
ENV TEMPLATE_PROVIDER_ZIP=terraform-provider-template_${TEMPLATE_PROVIDER_VERSION}_linux_amd64.zip
ENV TEMPLATE_PROVIDER_SHA256SUM=f54c2764bd4d4c62c1c769681206dde7aa94b64b814a43cb05680f1ec8911977
ENV TEMPLATE_PROVIDER_SHAFILE=terraform-provider-template_${TEMPLATE_PROVIDER_VERSION}_SHA256SUMS

RUN curl https://releases.hashicorp.com/terraform-provider-template/${TEMPLATE_PROVIDER_VERSION}/${TEMPLATE_PROVIDER_ZIP} > ${TEMPLATE_PROVIDER_ZIP} && \
  echo "${TEMPLATE_PROVIDER_SHA256SUM}  ${TEMPLATE_PROVIDER_ZIP}" > ${TEMPLATE_PROVIDER_SHAFILE} && \
  sha256sum --quiet -c ${TEMPLATE_PROVIDER_SHAFILE} && \
  mkdir -p ~/.terraform.d/plugins/linux_amd64/ && \
  unzip ${TEMPLATE_PROVIDER_ZIP} -d ~/.terraform.d/plugins/linux_amd64/ && \
  rm -f ${TEMPLATE_PROVIDER_ZIP} ${TEMPLATE_PROVIDER_SHAFILE}

# Setup vsphere provider
ENV VSPHERE_PROVIDER_VERSION=1.5.0
ENV VSPHERE_PROVIDER_ZIP=terraform-provider-vsphere_${VSPHERE_PROVIDER_VERSION}_linux_amd64.zip
ENV VSPHERE_PROVIDER_SHA256SUM=6dd495feeb83aa8b098d4e9b0224b9e18b758153504449ff4ac2c6510ed4bb52
ENV VSPHERE_PROVIDER_SHAFILE=terraform-provider-vsphere_${VSPHERE_PROVIDER_VERSION}_SHA256SUMS

RUN curl https://releases.hashicorp.com/terraform-provider-vsphere/${VSPHERE_PROVIDER_VERSION}/${VSPHERE_PROVIDER_ZIP} > ${VSPHERE_PROVIDER_ZIP} && \
  echo "${VSPHERE_PROVIDER_SHA256SUM}  ${VSPHERE_PROVIDER_ZIP}" > ${VSPHERE_PROVIDER_SHAFILE} && \
  sha256sum --quiet -c ${VSPHERE_PROVIDER_SHAFILE} && \
  mkdir -p ~/.terraform.d/plugins/linux_amd64/ && \
  unzip ${VSPHERE_PROVIDER_ZIP} -d ~/.terraform.d/plugins/linux_amd64/ && \
  rm -f ${VSPHERE_PROVIDER_ZIP} ${VSPHERE_PROVIDER_SHAFILE}

COPY --from=builder /go/bin/vsphere-machine-controller .
