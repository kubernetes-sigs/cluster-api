#!/bin/bash


rm generated/*

kustomize build --load-restrictor LoadRestrictionsNone core/global > generated/core-global.yaml
kustomize build --load-restrictor LoadRestrictionsNone core/base > generated/core-base.yaml

kustomize build --load-restrictor LoadRestrictionsNone bootstrap/global > generated/bootstrap-global.yaml
kustomize build --load-restrictor LoadRestrictionsNone bootstrap/base > generated/bootstrap-base.yaml

kustomize build --load-restrictor LoadRestrictionsNone controlplane/global > generated/controlplane-global.yaml
kustomize build --load-restrictor LoadRestrictionsNone controlplane/base > generated/controlplane-base.yaml
