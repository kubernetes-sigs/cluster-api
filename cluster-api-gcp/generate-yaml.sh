#!/bin/sh
set -e

GCLOUD_PROJECT=$(gcloud config get-value project)

TEMPLATE_FILE=machines.yaml.template
GENERATED_FILE=machines.yaml

if [ -f $GENERATED_FILE ]; then
  echo File $GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

sed -e "s/\$GCLOUD_PROJECT/$GCLOUD_PROJECT/" $TEMPLATE_FILE > $GENERATED_FILE
