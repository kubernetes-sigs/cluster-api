#!/bin/sh
set -e

GCLOUD_PROJECT=$(gcloud config get-value project)

TEMPLATE_FILE=machines.yaml.template
GENERATED_FILE=machines.yaml
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

if [ $OVERWRITE -ne 1 ] && [ -f $GENERATED_FILE ]; then
  echo File $GENERATED_FILE already exists. Delete it manually before running this script.
  exit 1
fi

sed -e "s/\$GCLOUD_PROJECT/$GCLOUD_PROJECT/" $TEMPLATE_FILE > $GENERATED_FILE
