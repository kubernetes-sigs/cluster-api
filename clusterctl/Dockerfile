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

# Reproducible builder image
FROM golang:${GOLANG_VERSION} as builder
WORKDIR /go/src/sigs.k8s.io/cluster-api-provider-ssh
# This expects that the context passed to the docker build command is
# the cluster-api directory.
# e.g. docker build -t <tag> -f <this_Dockerfile> <path_to_cluster-api>
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go install -a -ldflags '-extldflags "-static"' sigs.k8s.io/cluster-api-provider-ssh/clusterctl

# Final container
FROM docker:18.06-dind

ARG MINIKUBE_VERSION
ARG KUBECTL_VERSION

COPY --from=builder /go/bin/clusterctl .

ADD https://storage.googleapis.com/kubernetes-release/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl /usr/local/bin/kubectl
ADD https://github.com/kubernetes/minikube/releases/download/v${MINIKUBE_VERSION}/minikube-linux-amd64 /usr/local/bin/minikube

RUN md5sum -c ./md5sum && \
    chmod +x /usr/local/bin/kubectl /usr/local/bin/minikube

CMD [ "/clusterctl" ]
