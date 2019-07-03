#!/usr/bin/env bash
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

set -o errexit
set -o nounset
set -o pipefail

# fetch k8s API gen tools and make it available under kb_root_dir/bin.
function fetch_tools {
  header_text "fetching tools"
  kb_tools_archive_name="kubebuilder-tools-${k8s_version}-${goos}-${goarch}.tar.gz"
  kb_tools_download_url="https://storage.googleapis.com/kubebuilder-tools/${kb_tools_archive_name}"

  kb_tools_archive_path="${tmp_root}/${kb_tools_archive_name}"
  if [[ ! -f ${kb_tools_archive_path} ]]; then
    curl -fsL ${kb_tools_download_url} -o "${kb_tools_archive_path}"
  fi
  tar -zvxf "${kb_tools_archive_path}" -C "${tmp_root}/"
}

# fetch kubebuilder binary
tmp_root=/tmp
kb_root_dir=${tmp_root}/kubebuilder

fetch_tools

# shellcheck source=/dev/null
source "$(dirname "$0")/utils.sh"
# cd to the root path
cd_root_path

# run go test
export GO111MODULE=on
go test ./...
