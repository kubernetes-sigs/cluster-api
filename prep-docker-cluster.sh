#!/bin/bash

CLUSTER_NAME=${1:-my-cluster}

./bin/clusterctl get kubeconfig $CLUSTER_NAME > $CLUSTER_NAME.kubeconfig

# Point the kubeconfig to the exposed port of the load balancer, rather than the inaccessible container IP.
sed -i -e "s/server:.*/server: https:\/\/$(docker port $CLUSTER_NAME-lb 6443/tcp | sed "s/0.0.0.0/127.0.0.1/")/g" ./$CLUSTER_NAME.kubeconfig

# Ignore the CA, because it is not signed for 127.0.0.1
sed -i -e "s/certificate-authority-data:.*/insecure-skip-tls-verify: true/g" ./$CLUSTER_NAME.kubeconfig

# Add a CNI solution for the cluster
kubectl --kubeconfig=./$CLUSTER_NAME.kubeconfig apply -f https://docs.projectcalico.org/v3.21/manifests/calico.yaml
