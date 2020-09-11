# syntax=docker/dockerfile:experimental

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

FROM golang:1.13.15 as builder

# Run this with docker build --build_arg goproxy=$(go env GOPROXY) to override the goproxy
ARG goproxy=https://proxy.golang.org
ENV GOPROXY=$goproxy

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum

# Essentially, change directories into CAPD
WORKDIR /workspace/test/infrastructure/docker
# Copy the Go Modules manifests
COPY test/infrastructure/docker/go.mod go.mod
COPY test/infrastructure/docker/go.sum go.sum

# Cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# This needs to build with the entire Cluster API context
WORKDIR /workspace
# Copy in cluster-api (which includes the test/infrastructure/docker subdirectory)
COPY . .

# Essentially, change directories into CAPD
WORKDIR /workspace/test/infrastructure/docker

# Build the CAPD manager using the compiler cache folder
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o /workspace/manager main.go

# Use alpine:latest as minimal base image to package the manager binary and its dependencies
FROM alpine:latest

# install a couple of dependencies
WORKDIR /tmp
RUN apk add --update \
    curl \
    && rm -rf /var/cache/apk/*

RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/v1.15.0/bin/linux/amd64/kubectl && \
    chmod +x ./kubectl && \
    mv ./kubectl /usr/local/bin/kubectl

RUN curl -LO https://download.docker.com/linux/static/stable/x86_64/docker-19.03.1.tgz && \
    tar zxvf docker-19.03.1.tgz --strip 1 -C /usr/bin docker/docker && \
    rm docker-19.03.1.tgz

WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]
