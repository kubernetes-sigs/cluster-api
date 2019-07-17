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

FROM golang:1.12.6
WORKDIR /cluster-api-provider-docker
ADD go.mod .
ADD go.sum .
RUN go mod download
RUN  curl -L https://dl.k8s.io/v1.14.3/kubernetes-client-linux-amd64.tar.gz | tar xvz
ADD cmd cmd
ADD actuators actuators
ADD kind kind
ADD third_party third_party

RUN go install -v ./cmd/capd-manager
RUN curl https://get.docker.com | sh

FROM golang:1.12.5
COPY --from=0 /cluster-api-provider-docker/kubernetes/client/bin/kubectl /usr/local/bin
COPY --from=0 /go/bin/capd-manager /usr/local/bin
COPY --from=0 /usr/bin/docker /usr/local/bin
ENTRYPOINT ["capd-manager"]
