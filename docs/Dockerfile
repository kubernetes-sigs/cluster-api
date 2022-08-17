# syntax=docker/dockerfile:1.4

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

# To generate diagrams run 'make diagrams' in the docs/ folder.
FROM maven:3-jdk-8

RUN apt-get update && apt-get install -y --no-install-recommends graphviz=2.42.2-5 fonts-symbola=2.60-1.1 fonts-wqy-zenhei=0.9.45-8 && rm -rf /var/lib/apt/lists/*
RUN wget -O /plantuml.jar https://github.com/plantuml/plantuml/releases/download/v1.2022.6/plantuml-1.2022.6.jar

# By default, java writes a 'hsperfdata_<username>' directory in the work dir.
# This directory is not needed; to ensure it is not written, we set `-XX:-UsePerfData`
ENTRYPOINT [ "java", "-XX:-UsePerfData", "-jar", "/plantuml.jar" ]
