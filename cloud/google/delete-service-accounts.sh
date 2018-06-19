#!/bin/sh
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

set -e

#Delete service accounts

gcloud iam service-accounts delete $MASTER_SERVICE_ACCOUNT
gcloud iam service-accounts delete $WORKER_SERVICE_ACCOUNT
gcloud iam service-accounts delete $INGRESS_CONTROLLER_SERVICE_ACCOUNT
gcloud iam service-accounts delete $MACHINE_CONTROLLER_SERVICE_ACCOUNT

#Delete roles

export PROJECT_ID=$(gcloud config get-value project)

gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$MASTER_SERVICE_ACCOUNT --role='roles/compute.instanceAdmin'
gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$MASTER_SERVICE_ACCOUNT --role='roles/compute.networkAdmin'
gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$MASTER_SERVICE_ACCOUNT --role='roles/compute.securityAdmin'
gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$MASTER_SERVICE_ACCOUNT --role='roles/compute.viewer'
gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$MASTER_SERVICE_ACCOUNT --role='roles/iam.serviceAccountUser'
gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$MASTER_SERVICE_ACCOUNT --role='roles/storage.admin'
gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$MASTER_SERVICE_ACCOUNT --role='roles/storage.objectViewer'

gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$INGRESS_CONTROLLER_SERVICE_ACCOUNT --role='roles/compute.instanceAdmin.v1'
gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$INGRESS_CONTROLLER_SERVICE_ACCOUNT --role='roles/compute.networkAdmin'
gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$INGRESS_CONTROLLER_SERVICE_ACCOUNT --role='roles/compute.securityAdmin'
gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$INGRESS_CONTROLLER_SERVICE_ACCOUNT --role='roles/iam.serviceAccountActor'

gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$MACHINE_CONTROLLER_SERVICE_ACCOUNT --role='roles/compute.instanceAdmin.v1'
gcloud projects remove-iam-policy-binding $PROJECT_ID --member=serviceAccount:$MACHINE_CONTROLLER_SERVICE_ACCOUNT --role='roles/iam.serviceAccountActor'
