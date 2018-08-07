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

.PHONY: image push dev_image dev_push fix_gcs_permissions

GCR_BUCKET = k8s-cluster-api
PREFIX = gcr.io/$(GCR_BUCKET)
DEV_PREFIX ?= gcr.io/$(shell gcloud config get-value project)
NAME = cluster-apiserver
TAG = 0.0.6

image:
	docker build -t "$(PREFIX)/$(NAME):$(TAG)" -f ./Dockerfile ../..

push: image
	docker push "$(PREFIX)/$(NAME):$(TAG)"
	$(MAKE) fix_gcs_permissions

fix_gcs_permissions:
	gsutil acl ch -u AllUsers:READ gs://artifacts.$(GCR_BUCKET).appspot.com
	gsutil -m acl ch -r -u AllUsers:READ gs://artifacts.$(GCR_BUCKET).appspot.com

dev_image: 
	docker build -t "$(DEV_PREFIX)/$(NAME):$(TAG)-dev" -f ./Dockerfile ../..

dev_push: dev_image
	docker push "$(DEV_PREFIX)/$(NAME):$(TAG)-dev"
