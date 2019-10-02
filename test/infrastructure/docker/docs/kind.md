# Deploy CAPD to an existing kind cluster

The CAPD controllers need access to docker storage and docker socket on the host. Typically, these are `/var/lib/docker` and `/var/run/docker.sock`, respectively. These locations must be propagated from the host to the kind node(s) where CAPD controllers run. By default, kind does not propagate them. To create a kind cluster that supports CAPD controllers, propagate these using `extraMounts`

```
cat > kind-cluster-with-extramounts.yaml <<EOF
kind: Cluster
apiVersion: kind.sigs.k8s.io/v1alpha3
nodes:
  - role: control-plane
    extraMounts:
      - hostPath: /var/lib/docker
        containerPath: /var/lib/docker
      - hostPath: /var/run/docker.sock
        containerPath: /var/run/docker.sock
EOF
kind create cluster --config kind-cluster-with-extramounts.yaml --name management-cluster
```

