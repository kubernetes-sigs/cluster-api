#!/bin/sh
set -e

GCLOUD_PROJECT=$(gcloud config get-value project)
ZONE=$(gcloud config get-value compute/zone)
ZONE="${ZONE:-us-central1-f}"

MACHINE_TEMPLATE_FILE=machines.yaml.template
MACHINE_GENERATED_FILE=machines.yaml
CLUSTER_TEMPLATE_FILE=cluster.yaml.template
CLUSTER_GENERATED_FILE=cluster.yaml
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
  echo File $MACHINE_GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

if [ $OVERWRITE -ne 1 ] && [ -f $CLUSTER_GENERATED_FILE ]; then
  echo File $CLUSTER_GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

sed -e "s/\$ZONE/$ZONE/" $MACHINE_TEMPLATE_FILE > $MACHINE_GENERATED_FILE

sed -e "s/\$GCLOUD_PROJECT/$GCLOUD_PROJECT/" $CLUSTER_TEMPLATE_FILE > $CLUSTER_GENERATED_FILE
