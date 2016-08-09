#!/bin/bash

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

# exit on any error
set -e

cd /home/vagrant/kubernetes
. /etc/profile.d/go-path.sh
go get -u github.com/jteeuwen/go-bindata/go-bindata
make all WHAT=cmd/kubectl
make all WHAT=vendor/github.com/onsi/ginkgo/ginkgo
make all WHAT=test/e2e/e2e.test
cluster/kubectl.sh config set-cluster local --server=http://${MASTER_IP}:8080 --insecure-skip-tls-verify=true
cluster/kubectl.sh config set-context local --cluster=local
cluster/kubectl.sh config use-context local
nohup go run hack/e2e.go --v --test -check_version_skew=false --test_args="--ginkgo.skip=\[Serial\]|\[Flaky\]|\[Feature:.+\]" > /home/vagrant/e2e.txt &
