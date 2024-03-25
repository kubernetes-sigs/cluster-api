# Diagnostics

## Introduction

With CAPI v1.6 we introduced new flags to allow serving metrics, the pprof endpoint and an endpoint to dynamically change log levels securely in production.

This feature is enabled per default via:
```yaml
          args:
            - "--diagnostics-address=${CAPI_DIAGNOSTICS_ADDRESS:=:8443}"
```

As soon as the feature is enabled the metrics endpoint is served via https and protected via authentication and authorization. This works the same way as 
metrics in core Kubernetes components: [Metrics in Kubernetes](https://kubernetes.io/docs/concepts/cluster-administration/system-metrics/).

To continue serving metrics via http the following configuration can be used:
```yaml
          args:
            - "--diagnostics-address=localhost:8080"
            - "--insecure-diagnostics"
```

The same can be achieved via clusterctl:
```bash
export CAPI_DIAGNOSTICS_ADDRESS: "localhost:8080"
export CAPI_INSECURE_DIAGNOSTICS: "true"
clusterctl init ...
```

**Note**: If insecure serving is configured the pprof and log level endpoints are disabled for security reasons.

## Scraping metrics

A ServiceAccount token is now required to scrape metrics. The corresponding ServiceAccount needs permissions on the `/metrics` path.
This can be achieved e.g. by following the [Kubernetes documentation](https://kubernetes.io/docs/concepts/cluster-administration/system-metrics/).

### via Prometheus

With the Prometheus Helm chart it is as easy as using the following config for the Prometheus job scraping the Cluster API controllers:
```yaml
    scheme: https
    authorization:
      type: Bearer
      credentials_file: /var/run/secrets/kubernetes.io/serviceaccount/token
    tls_config:
      ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
      # The diagnostics endpoint is using a self-signed certificate, so we don't verify it.
      insecure_skip_verify: true
```

For more details please see our Prometheus development setup: [Prometheus](https://github.com/kubernetes-sigs/cluster-api/tree/main/hack/observability/prometheus) 

**Note**: The Prometheus Helm chart deploys the required ClusterRole out-of-the-box.

### via kubectl

First deploy the following RBAC configuration:
```yaml
cat << EOT | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: default-metrics
rules:
- nonResourceURLs:
  - "/metrics"
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: default-metrics
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: default-metrics
subjects:
- kind: ServiceAccount
  name: default
  namespace: default
EOT
```

Then let's open a port-forward, create a ServiceAccount token and scrape the metrics:
```bash
# Terminal 1
kubectl -n capi-system port-forward deployments/capi-controller-manager 8443

# Terminal 2
TOKEN=$(kubectl create token default)
curl https://localhost:8443/metrics --header "Authorization: Bearer $TOKEN" -k
```

## Collecting profiles

### via Parca

Parca can be used to continuously scrape profiles from CAPI providers. For more details please see our Parca 
development setup: [parca](https://github.com/kubernetes-sigs/cluster-api/tree/main/hack/observability/parca)

### via kubectl

First deploy the following RBAC configuration:
```yaml
cat << EOT | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: default-pprof
rules:
- nonResourceURLs:
  - "/debug/pprof/*"
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: default-pprof
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: default-pprof
subjects:
- kind: ServiceAccount
  name: default
  namespace: default
EOT
```

Then let's open a port-forward, create a ServiceAccount token and scrape the profile:
```bash
# Terminal 1
kubectl -n capi-system port-forward deployments/capi-controller-manager 8443

# Terminal 2
TOKEN=$(kubectl create token default)

# Get a goroutine dump
curl "https://localhost:8443/debug/pprof/goroutine?debug=2" --header "Authorization: Bearer $TOKEN" -k > ./goroutine.txt

# Get a profile
curl "https://localhost:8443/debug/pprof/profile?seconds=10" --header "Authorization: Bearer $TOKEN" -k > ./profile.out
go tool pprof -http=:8080 ./profile.out
```

## Changing the log level

### via kubectl

First deploy the following RBAC configuration:
```yaml
cat << EOT | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: default-loglevel
rules:
- nonResourceURLs:
  - "/debug/flags/v"
  verbs:
  - put
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: default-loglevel
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: default-loglevel
subjects:
- kind: ServiceAccount
  name: default
  namespace: default
EOT
```

Then let's open a port-forward, create a ServiceAccount token and change the log level to `8`:
```bash
# Terminal 1
kubectl -n capi-system port-forward deployments/capi-controller-manager 8443

# Terminal 2
TOKEN=$(kubectl create token default)
curl "https://localhost:8443/debug/flags/v" --header "Authorization: Bearer $TOKEN" -X PUT -d '8' -k
```
