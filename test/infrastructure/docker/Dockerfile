# syntax=docker/dockerfile:1.1-experimental

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

# Run this with docker build --build-arg builder_image=<golang:x.y.z>
ARG builder_image
FROM ${builder_image} as builder

# Run this with docker build --build-arg goproxy=$(go env GOPROXY) to override the goproxy
ARG goproxy=https://proxy.golang.org
ENV GOPROXY=$goproxy

# Gets additional CAPD dependencies
WORKDIR /tmp

RUN curl -LO https://dl.k8s.io/release/v1.22.0/bin/linux/${ARCH}/kubectl && \
    chmod +x ./kubectl && \
    mv ./kubectl /usr/bin/kubectl

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum

# Essentially, change directories into the test go module
WORKDIR /workspace/test
# Copy the Go Modules manifests
COPY test/go.mod go.mod
COPY test/go.sum go.sum

# Cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# This needs to build with the entire Cluster API context
WORKDIR /workspace
# Copy in cluster-api (which includes the test/infrastructure/docker subdirectory)
COPY . .

# Essentially, change directories into CAPD
WORKDIR /workspace/test/infrastructure/docker

# Build the CAPD manager using the compiler cache folder
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -trimpath -a -o /workspace/manager main.go

# NOTE: CAPD can't use non-root because docker requires access to the docker socket
FROM gcr.io/distroless/static:latest

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /usr/bin/kubectl /usr/bin/kubectl

ENTRYPOINT ["/manager"]
