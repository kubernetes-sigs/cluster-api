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
FROM alpine:3.7
RUN apk --no-cache add --update ca-certificates bash openssh terraform
RUN echo 'plugin_cache_dir = "$HOME/.terraform.d/plugin-cache"' >> ~/.terraformrc
COPY --from=builder /go/bin/terraform-machine-controller .
