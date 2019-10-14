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

FROM golang:1.12.9 as builder

# default the go proxy
ARG goproxy=https://proxy.golang.org

# run this with docker build --build_arg $(go env GOPROXY) to override the goproxy
ENV GOPROXY=$goproxy

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY ./ ./

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager main.go

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
