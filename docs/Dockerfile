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

# Recommended usage:
#
# - Run an ephemeral container
# - Mount the current working directory into the container.
# - Run the entrypoint as the user invoking docker run. Otherwise the output
#   files will be owned by root, the default user.
#
# - Example:
# docker run \
# 	--rm \
# 	--volume ${PWD}:/figures \
# 	--user $(shell id --user):$(shell id --group) \
# 	${IMAGE_TAG} \
# 	-v /figures/*.plantuml

FROM maven:3-jdk-8

RUN apt-get update && apt-get install -y --no-install-recommends graphviz fonts-symbola fonts-wqy-zenhei && rm -rf /var/lib/apt/lists/*
RUN wget -O /plantuml.jar http://sourceforge.net/projects/plantuml/files/plantuml.1.2019.6.jar/download

# By default, java writes a 'hsperfdata_<username>' directory in the work dir.
# This directory is not needed; to ensure it is not written, we set `-XX:-UsePerfData`
ENTRYPOINT [ "java", "-XX:-UsePerfData", "-jar", "/plantuml.jar" ]
