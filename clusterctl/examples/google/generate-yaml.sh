#!/bin/sh
set -e

GCLOUD_PROJECT=$(gcloud config get-value project)
ZONE=$(gcloud config get-value compute/zone)
ZONE="${ZONE:-us-central1-f}"
CLUSTER_NAME=test1

OUTPUT_DIR=out

MACHINE_TEMPLATE_FILE=machines.yaml.template
MACHINE_GENERATED_FILE=${OUTPUT_DIR}/machines.yaml
CLUSTER_TEMPLATE_FILE=cluster.yaml.template
CLUSTER_GENERATED_FILE=${OUTPUT_DIR}/cluster.yaml
PROVIDERCOMPONENT_TEMPLATE_FILE=provider-components.yaml.template
PROVIDERCOMPONENT_GENERATED_FILE=${OUTPUT_DIR}/provider-components.yaml
ADDON_TEMPLATE_FILE=addons.yaml.template
ADDON_GENERATED_FILE=${OUTPUT_DIR}/addons.yaml

MACHINE_CONTROLLER_SA_FILE=${OUTPUT_DIR}/machine-controller-serviceaccount.json
MACHINE_CONTROLLER_SA_NAME="machine-controller-$CLUSTER_NAME"
MACHINE_CONTROLLER_SA_EMAIL="$MACHINE_CONTROLLER_SA_NAME@$GCLOUD_PROJECT.iam.gserviceaccount.com"
MACHINE_CONTROLLER_SA_KEY=
LOADBALANCER_SA_FILE=${OUTPUT_DIR}/loadbalancer-serviceaccount.json
LOADBALANCER_SA_NAME="loadbalancer-$CLUSTER_NAME"
LOADBALANCER_SA_EMAIL="$LOADBALANCER_SA_NAME@$GCLOUD_PROJECT.iam.gserviceaccount.com"
LOADBALANCER_SA_KEY=

# TODO: The following service accounts will eventually be provisioned by the cluster controller. In the meanwhile, they are provisioned here.
MASTER_SA_NAME="master-$CLUSTER_NAME"
MASTER_SA_EMAIL="$MASTER_SA_NAME@$GCLOUD_PROJECT.iam.gserviceaccount.com"
WORKER_SA_NAME="worker-$CLUSTER_NAME"
WORKER_SA_EMAIL="$WORKER_SA_NAME@$GCLOUD_PROJECT.iam.gserviceaccount.com"

MACHINE_CONTROLLER_SSH_PUBLIC_FILE=${OUTPUT_DIR}/machine-controller-key.pub
MACHINE_CONTROLLER_SSH_PUBLIC=
MACHINE_CONTROLLER_SSH_PRIVATE_FILE=${OUTPUT_DIR}/machine-controller-key
MACHINE_CONTROLLER_SSH_PRIVATE=
MACHINE_CONTROLLER_SSH_USER_PLAIN=clusterapi
MACHINE_CONTROLLER_SSH_USER=$(echo -n "$MACHINE_CONTROLLER_SSH_USER_PLAIN" | base64 -w0)


OVERWRITE=0

SCRIPT=$(basename $0)
while test $# -gt 0; do
        case "$1" in
          -h|--help)
            echo "$SCRIPT - generates input yaml files for Cluster API on Google Cloud Platform"
            echo " "
            echo "$SCRIPT [options]"
            echo " "
            echo "options:"
            echo "-h, --help                show brief help"
            echo "-f, --force-overwrite     if file to be generated already exists, force script to overwrite it"
            exit 0
            ;;
          -f)
            OVERWRITE=1
            shift
            ;;
          --force-overwrite)
            OVERWRITE=1
            shift
            ;;
          *)
            break
            ;;
        esac
done

if [ $OVERWRITE -ne 1 ] && [ -f $MACHINE_GENERATED_FILE ]; then
  echo File $PROVIDERCOMPONENT_GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

if [ $OVERWRITE -ne 1 ] && [ -f $CLUSTER_GENERATED_FILE ]; then
  echo File $PROVIDERCOMPONENT_GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

if [ $OVERWRITE -ne 1 ] && [ -f $PROVIDERCOMPONENT_GENERATED_FILE ]; then
  echo File $PROVIDERCOMPONENT_GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

if [ $OVERWRITE -ne 1 ] && [ -f $ADDON_GENERATED_FILE ]; then
  echo File $ADDON_GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

mkdir -p ${OUTPUT_DIR}

if [ ! -f $MACHINE_CONTROLLER_SA_FILE ]; then
  echo Generating $MACHINE_CONTROLLER_SA_EMAIL service account for machine controller
  gcloud iam service-accounts create --display-name="machine controller service account" $MACHINE_CONTROLLER_SA_NAME
  gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$MACHINE_CONTROLLER_SA_EMAIL --role=roles/compute.instanceAdmin.v1
  gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$MACHINE_CONTROLLER_SA_EMAIL --role=roles/iam.serviceAccountActor
  gcloud iam service-accounts keys create $MACHINE_CONTROLLER_SA_FILE --iam-account $MACHINE_CONTROLLER_SA_EMAIL
fi
MACHINE_CONTROLLER_SA_KEY=$(cat $MACHINE_CONTROLLER_SA_FILE|base64 -w0)

if [ ! -f $LOADBALANCER_SA_FILE ]; then
  echo Generating $LOADBALANCER_SA_EMAIL service account for loadbalancers
  gcloud iam service-accounts create --display-name="loadbalancer service account" $LOADBALANCER_SA_NAME
  gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$LOADBALANCER_SA_EMAIL --role=roles/compute.instanceAdmin.v1
  gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$LOADBALANCER_SA_EMAIL --role=roles/compute.networkAdmin
  gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$LOADBALANCER_SA_EMAIL --role=roles/compute.securityAdmin
  gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$LOADBALANCER_SA_EMAIL --role=roles/iam.serviceAccountActor
  gcloud iam service-accounts keys create $LOADBALANCER_SA_FILE --iam-account $LOADBALANCER_SA_EMAIL
fi
LOADBALANCER_SA_KEY=$(cat $LOADBALANCER_SA_FILE|base64 -w0)

echo Generating $MASTER_SA_EMAIL service account for masters
gcloud iam service-accounts create --display-name="master service account" $MASTER_SA_NAME
gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$MASTER_SA_EMAIL --role=roles/compute.instanceAdmin
gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$MASTER_SA_EMAIL --role=roles/compute.networkAdmin
gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$MASTER_SA_EMAIL --role=roles/compute.securityAdmin
gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$MASTER_SA_EMAIL --role=roles/compute.viewer
gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$MASTER_SA_EMAIL --role=roles/storage.admin
gcloud projects add-iam-policy-binding $GCLOUD_PROJECT --member=serviceAccount:$MASTER_SA_EMAIL --role=roles/storage.objectViewer

echo Generating $WORKER_SA_EMAIL service account for workers
gcloud iam service-accounts create --display-name="worker service account" $WORKER_SA_NAME

if [ ! -f $MACHINE_CONTROLLER_SSH_PRIVATE_FILE ]; then
  echo Generate SSH key files fo machine controller
  ssh-keygen -t rsa -f $MACHINE_CONTROLLER_SSH_PRIVATE_FILE -C $MACHINE_CONTROLLER_SSH_USER_PLAIN -N ""
fi

MACHINE_CONTROLLER_SSH_PUBLIC=$(cat $MACHINE_CONTROLLER_SSH_PUBLIC_FILE|base64 -w0)
MACHINE_CONTROLLER_SSH_PRIVATE=$(cat $MACHINE_CONTROLLER_SSH_PRIVATE_FILE|base64 -w0)

cat $MACHINE_TEMPLATE_FILE \
  | sed -e "s/\$ZONE/$ZONE/" \
  > $MACHINE_GENERATED_FILE

cat $CLUSTER_TEMPLATE_FILE \
  | sed -e "s/\$GCLOUD_PROJECT/$GCLOUD_PROJECT/" \
  | sed -e "s/\$CLUSTER_NAME/$CLUSTER_NAME/" \
  > $CLUSTER_GENERATED_FILE

cat $PROVIDERCOMPONENT_TEMPLATE_FILE \
  | sed -e "s/\$MACHINE_CONTROLLER_SA_KEY/$MACHINE_CONTROLLER_SA_KEY/" \
  | sed -e "s/\$CLUSTER_NAME/$CLUSTER_NAME/" \
  | sed -e "s/\$MACHINE_CONTROLLER_SSH_USER/$MACHINE_CONTROLLER_SSH_USER/" \
  | sed -e "s/\$MACHINE_CONTROLLER_SSH_PUBLIC/$MACHINE_CONTROLLER_SSH_PUBLIC/" \
  | sed -e "s/\$MACHINE_CONTROLLER_SSH_PRIVATE/$MACHINE_CONTROLLER_SSH_PRIVATE/" \
  > $PROVIDERCOMPONENT_GENERATED_FILE

cat $ADDON_TEMPLATE_FILE \
  | sed -e "s/\$GCLOUD_PROJECT/$GCLOUD_PROJECT/" \
  | sed -e "s/\$CLUSTER_NAME/$CLUSTER_NAME/" \
  | sed "s/\$LOADBALANCER_SA_KEY/$LOADBALANCER_SA_KEY/" \
  > $ADDON_GENERATED_FILE