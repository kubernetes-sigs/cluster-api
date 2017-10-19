#!/usr/bin/env bash

# Copyright © 2017 The Kubicorn Authors
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

VERSION=$(cat ./VERSION)
MAJOR=$(echo $VERSION | cut -d. -f1)
MINOR=$(echo $VERSION | cut -d. -f2)
PATCH=$(echo $VERSION | cut -d. -f3)

if [ "$1" = "major" ]; then
    MAJOR=$(expr $MAJOR + 1)
    MINOR=0
    PATCH=0
fi

if [ "$1" = "minor" ]; then
    MINOR=$(expr $MINOR + 1)
    PATCH=0
fi

if [ "$1" = "patch" ]; then
    PATCH=$(expr $PATCH + 1)
fi

VER=$(printf "%d.%.d.%.3d" $MAJOR $MINOR $PATCH)
echo $VER > ./VERSION