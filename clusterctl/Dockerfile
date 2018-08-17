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

ARG GOLANG_VERSION=1.10.3
ARG MINIKUBE_VERSION=0.28.0
ARG KUBECTL_VERSION=1.11.1
ARG DOCKER_VERSION=17.03.2

# Reproducible builder image
FROM golang:${GOLANG_VERSION} as builder
WORKDIR /go/src/sigs.k8s.io/cluster-api
# This expects that the context passed to the docker build command is
# the cluster-api directory.
# e.g. docker build -t <tag> -f <this_Dockerfile> <path_to_cluster-api>
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api/clusterctl

# Final container
FROM debian:stretch-slim

ARG MINIKUBE_VERSION
ARG KUBECTL_VERSION
ARG DOCKER_VERSION

ADD https://storage.googleapis.com/kubernetes-release/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl /usr/local/bin/kubectl
ADD https://github.com/kubernetes/minikube/releases/download/v${MINIKUBE_VERSION}/minikube-linux-amd64 /usr/local/bin/minikube
ADD https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}-ce.tgz /tmp/docker.tgz

COPY --from=builder /go/src/sigs.k8s.io/cluster-api/clusterctl/sha256sums /tmp/

RUN sha256sum -c /tmp/sha256sums && \
    tar xzf /tmp/docker.tgz -C /usr/local/bin docker/docker && \
    rm /tmp/sha256sums /tmp/docker.tgz && \
    chmod 755 /usr/local/bin/kubectl /usr/local/bin/minikube /usr/local/bin/docker

RUN apt-get update -y && \
    apt-get install -y ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /go/bin/clusterctl .

CMD [ "/clusterctl" ]
