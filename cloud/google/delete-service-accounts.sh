#!/bin/sh
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
