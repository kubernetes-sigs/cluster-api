# Workload bootstrap using GitOps

Cluster API can be utilized in combination with the [Cluster API addon provider for helm (CAAPH)](https://github.com/kubernetes-sigs/cluster-api-addon-provider-helm/blob/main/docs/quick-start.md) to install and configure a GitOps agent and then the GitOps agent hydrates clusters automatically with various workloads.

## Prerequisites

Follow the quickstart setup guide for your provider but ensure that CAAPH is installed via including the `addon=helm` with either:

1. [clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start#initialize-the-management-cluster) using `clusterctl init --infrastructure ### --addon helm` or
1. [Cluster API Operator](https://cluster-api.sigs.k8s.io/user/quick-start-operator) using `helm install capi-operator capi-operator/cluster-api-operator ... --set infrastructure=#### --set addon=helm`

## Bootstrap ManagedCluster using ArgoCD

Add the labels `argoCDChart: enabled` and `guestbook: enabled` to your desired workload cluster yaml file in the `Cluster` metadata section, for example:

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: my-cluster
  namespace: default
  labels:
    argoCDChart: enabled
    guestbook: enabled
```

Then create and `kubectl apply -f` the following file on the management cluster to install the ArgoCD agent and the sample guestbook app to the workload cluster via the argo helm charts using CAAPH:

```yaml
apiVersion: addons.cluster.x-k8s.io/v1alpha1
kind: HelmChartProxy
metadata:
  name: argocd
spec:
  clusterSelector:
    matchLabels:
      argoCDChart: enabled
  repoURL: https://argoproj.github.io/argo-helm
  chartName: argo-cd
  options:
    waitForJobs: true
    wait: true
    timeout: 5m
    install:
      createNamespace: true
---
apiVersion: addons.cluster.x-k8s.io/v1alpha1
kind: HelmChartProxy
metadata:
  name: argocdguestbook
spec:
  clusterSelector:
    matchLabels:
      guestbook: enabled
  repoURL: https://argoproj.github.io/argo-helm
  chartName: argocd-apps
  options:
    waitForJobs: true
    wait: true
    timeout: 5m
    install:
      createNamespace: true
  valuesTemplate: |
    applications:
      - name: guestbook
        namespace: argocd
        finalizers:
        - resources-finalizer.argocd.argoproj.io
        project: default
        sources:
          - repoURL: https://github.com/argoproj/argocd-example-apps.git
            path: guestbook
            targetRevision: HEAD
        destination:
          server: https://kubernetes.default.svc
          namespace: guestbook
        syncPolicy:
          automated:
            prune: false
            selfHeal: false
          syncOptions:
          - CreateNamespace=true
        revisionHistoryLimit: null
    ignoreDifferences:
      - group: apps
        kind: Deployment
        jsonPointers:
        - /spec/replicas
    info:
    - name: url
      value: https://argoproj.github.io/
```

This will automatically install ArgoCD in the ArgoCD namespace and the guestbook application into the guestbook namespace.  Adding or labeling additional clusters with `argoCDChart: enabled` and `guestbook: enabled` will automatically install the ArgoCD agent and the guestbook application and there is no need to create additional CAAPH HelmChartProxy entries.

The ArgoCD console can be viewed by connecting to the workload cluster and then doing the following:

```bash
# Get the admin password
kubectl get secrets argocd-initial-admin-secret -n argocd --template="{{index .data.password | base64decode}}"
kubectl port-forward service/capiargo-argocd-server -n default 8080:443
# and then open the browser on http://localhost:8080 and accept the certificate
```

The Guestbook application deployment can be seen once logged into the ArgoCD console. Since the GitOps agent points to the git repository, any changes to the repository will automatically update the workload cluster.  The git repository could be configured to utilize the [App of Apps pattern](https://argo-cd.readthedocs.io/en/stable/operator-manual/cluster-bootstrapping/#app-of-apps-pattern) to install all platform requirements for the cluster. The App of Apps pattern is a single application that installs all other applications and configurations for the cluster.

This same pattern could also utilize the Flux agent using the [Flux helm charts](https://github.com/fluxcd-community/helm-charts/) being installed and configured by CAAPH.
