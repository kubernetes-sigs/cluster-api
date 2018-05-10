/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

const ClusterAPIDeployConfigTemplate = `
apiVersion: apiregistration.k8s.io/v1beta1
kind: APIService
metadata:
  name: v1alpha1.cluster.k8s.io
  labels:
    api: clusterapi
    apiserver: "true"
spec:
  version: v1alpha1
  group: cluster.k8s.io
  groupPriorityMinimum: 2000
  priority: 200
  service:
    name: clusterapi
    namespace: default
  versionPriority: 10
  caBundle: {{ .CABundle }}
---
apiVersion: v1
kind: Service
metadata:
  name: clusterapi
  namespace: default
  labels:
    api: clusterapi
    apiserver: "true"
spec:
  ports:
  - port: 443
    protocol: TCP
    targetPort: 443
  selector:
    api: clusterapi
    apiserver: "true"
---
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: clusterapi
  namespace: default
  labels:
    api: clusterapi
    apiserver: "true"
spec:
  replicas: 1
  template:
    metadata:
      labels:
        api: clusterapi
        apiserver: "true"
    spec:
      nodeSelector:
        node-role.kubernetes.io/master: ""
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
      - key: CriticalAddonsOnly
        operator: Exists
      - effect: NoExecute
        key: node.alpha.kubernetes.io/notReady
        operator: Exists
      - effect: NoExecute
        key: node.alpha.kubernetes.io/unreachable
        operator: Exists
      containers:
      - name: apiserver
        image: {{ .APIServerImage }}
        volumeMounts:
        - name: cluster-apiserver-certs
          mountPath: /apiserver.local.config/certificates
          readOnly: true
        - name: config
          mountPath: /etc/kubernetes
        - name: certs
          mountPath: /etc/ssl/certs
        command:
        - "./apiserver"
        args:
        - "--etcd-servers=http://etcd-clusterapi-svc:2379"
        - "--tls-cert-file=/apiserver.local.config/certificates/tls.crt"
        - "--tls-private-key-file=/apiserver.local.config/certificates/tls.key"
        - "--audit-log-path=-"
        - "--audit-log-maxage=0"
        - "--audit-log-maxbackup=0"
        - "--authorization-kubeconfig=/etc/kubernetes/admin.conf"
        - "--kubeconfig=/etc/kubernetes/admin.conf"
        resources:
          requests:
            cpu: 100m
            memory: 20Mi
          limits:
            cpu: 100m
            memory: 30Mi
      - name: controller-manager
        image: {{ .ControllerManagerImage }}
        volumeMounts:
          - name: config
            mountPath: /etc/kubernetes
          - name: certs
            mountPath: /etc/ssl/certs
        command:
        - "./controller-manager"
        args:
        - --kubeconfig=/etc/kubernetes/admin.conf
        resources:
          requests:
            cpu: 100m
            memory: 20Mi
          limits:
            cpu: 100m
            memory: 30Mi
      - name: gce-machine-controller
        image: {{ .MachineControllerImage }}
        volumeMounts:
          - name: config
            mountPath: /etc/kubernetes
          - name: certs
            mountPath: /etc/ssl/certs
          - name: credentials
            mountPath: /etc/credentials
          - name: sshkeys
            mountPath: /etc/sshkeys
          - name: machine-setup
            mountPath: /etc/machinesetup
        env:
          - name: GOOGLE_APPLICATION_CREDENTIALS
            value: /etc/credentials/service-account.json
        command:
        - "./gce-machine-controller"
        args:
        - --kubeconfig=/etc/kubernetes/admin.conf
        - --token={{ .Token }}
        - --machinesetup=/etc/machinesetup/machine_setup_configs.yaml
        resources:
          requests:
            cpu: 100m
            memory: 20Mi
          limits:
            cpu: 100m
            memory: 30Mi
      volumes:
      - name: cluster-apiserver-certs
        secret:
          secretName: cluster-apiserver-certs
      - name: config
        hostPath:
          path: /etc/kubernetes
      - name: certs
        hostPath:
          path: /etc/ssl/certs
      - name: sshkeys
        secret:
          secretName: machine-controller-sshkeys
          defaultMode: 256
      - name: credentials
        secret:
          secretName: machine-controller-credential
      - name: machine-setup
        configMap: 
          name: machine-setup
---
apiVersion: apps/v1beta1
kind: StatefulSet
metadata:
  name: etcd-clusterapi
  namespace: default
spec:
  serviceName: "etcd"
  replicas: 1
  template:
    metadata:
      labels:
        app: etcd
    spec:
      nodeSelector:
        node-role.kubernetes.io/master: ""
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
      - key: CriticalAddonsOnly
        operator: Exists
      - effect: NoExecute
        key: node.alpha.kubernetes.io/notReady
        operator: Exists
      - effect: NoExecute
        key: node.alpha.kubernetes.io/unreachable
        operator: Exists
      volumes:
      - hostPath:
          path: /var/lib/etcd2
          type: DirectoryOrCreate
        name: etcd-data-dir
      terminationGracePeriodSeconds: 10
      containers:
      - name: etcd
        image: quay.io/coreos/etcd:latest
        imagePullPolicy: Always
        resources:
          requests:
            cpu: 100m
            memory: 20Mi
          limits:
            cpu: 100m
            memory: 30Mi
        env:
        - name: ETCD_DATA_DIR
          value: /etcd-data-dir
        command:
        - /usr/local/bin/etcd
        - --listen-client-urls
        - http://0.0.0.0:2379
        - --advertise-client-urls
        - http://localhost:2379
        ports:
        - containerPort: 2379
        volumeMounts:
        - name: etcd-data-dir
          mountPath: /etcd-data-dir
        readinessProbe:
          httpGet:
            port: 2379
            path: /health
          failureThreshold: 1
          initialDelaySeconds: 10
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 2
        livenessProbe:
          httpGet:
            port: 2379
            path: /health
          failureThreshold: 3
          initialDelaySeconds: 10
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 2
---
apiVersion: v1
kind: Service
metadata:
  name: etcd-clusterapi-svc
  namespace: default
  labels:
    app: etcd
spec:
  ports:
  - port: 2379
    name: etcd
    targetPort: 2379
  selector:
    app: etcd
---
apiVersion: v1
kind: Secret
type: kubernetes.io/tls
metadata:
  name: cluster-apiserver-certs
  namespace: default
  labels:
    api: clusterapi
    apiserver: "true"
data:
  tls.crt: {{ .TLSCrt }}
  tls.key: {{ .TLSKey }}
`

const StorageClassConfigTemplate = `
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: standard
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: kubernetes.io/gce-pd
parameters:
  type: pd-standard
`

const IngressControllerConfigTemplate = `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: glbc
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/
kind: ClusterRole
metadata:
  name: system:controller:glbc
rules:
- apiGroups: [""]
  resources: ["secrets", "endpoints", "services", "pods", "nodes", "namespaces"]
  verbs: ["describe", "get", "list", "watch"]
- apiGroups: [""]
  resources: ["events", "configmaps"]
  verbs: ["describe", "get", "list", "watch", "update", "create", "patch"]
- apiGroups: ["extensions"]
  resources: ["ingresses"]
  verbs: ["get", "list", "watch", "update"]
- apiGroups: ["extensions"]
  resources: ["ingresses/status"]
  verbs: ["update"]
---
apiVersion: rbac.authorization.k8s.io/
kind: ClusterRoleBinding
metadata:
  name: system:controller:glbc
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:controller:glbc
subjects:
- kind: ServiceAccount
  name: glbc
  namespace: kube-system
---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: l7-default-backend
  namespace: kube-system
  labels:
    k8s-app: glbc
    kubernetes.io/name: "GLBC"
    kubernetes.io/cluster-service: "true"
    addonmanager.kubernetes.io/mode: Reconcile
spec:
  replicas: 1
  selector:
    matchLabels:
      k8s-app: glbc
  template:
    metadata:
      labels:
        k8s-app: glbc
        name: glbc
    spec:
      containers:
      - name: default-http-backend
        # Any image is permissible as long as:
        # 1. It serves a 404 page at /
        # 2. It serves 200 on a /healthz endpoint
        image: gcr.io/google_containers/defaultbackend:1.4
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
            scheme: HTTP
          initialDelaySeconds: 30
          timeoutSeconds: 5
        ports:
        - containerPort: 8080
        resources:
          limits:
            cpu: 10m
            memory: 20Mi
          requests:
            cpu: 10m
            memory: 20Mi
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: ingress-controller-config
  namespace: kube-system
data:
  gce.conf: |
    [global]
    token-url = nil
    network = default
    project-id = {{ .Project }}
    node-tags = {{ .NodeTag }}
---
apiVersion: v1
kind: Service
metadata:
  # This must match the --default-backend-service argument of the l7 lb
  # controller and is required because GCE mandates a default backend.
  name: default-http-backend
  namespace: kube-system
  labels:
    k8s-app: glbc
    kubernetes.io/cluster-service: "true"
    addonmanager.kubernetes.io/mode: Reconcile
    kubernetes.io/name: "GLBCDefaultBackend"
spec:
  # The default backend must be of type NodePort.
  type: NodePort
  ports:
  - port: 80
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    k8s-app: glbc
---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  namespace: kube-system
  name: l7-lb-controller
  annotations:
    scheduler.alpha.kubernetes.io/critical-pod: ''
  labels:
    k8s-app: glbc
    version: v1.1.1
    kubernetes.io/name: "GLBC"
spec:
  # There should never be more than 1 controller alive simultaneously.
  replicas: 1
  selector:
    matchLabels:
      k8s-app: glbc
      version: v1.1.1
  template:
    metadata:
      labels:
        k8s-app: glbc
        version: v1.1.1
        name: glbc
    spec:
      serviceAccountName: glbc
      terminationGracePeriodSeconds: 600
      containers:
      - image: k8s.gcr.io/ingress-gce-glbc-amd64:v1.1.1
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8086
            scheme: HTTP
          initialDelaySeconds: 30
          timeoutSeconds: 5
          successThreshold: 1
          failureThreshold: 5
        env:
        - name: GOOGLE_APPLICATION_CREDENTIALS
          value: /etc/credentials/service-account.json
        name: l7-lb-controller
        resources:
          limits:
            cpu: 100m
            memory: 100Mi
          requests:
            cpu: 100m
            memory: 50Mi
        command:
        - sh
        - -c
        - 'exec /glbc --gce-ratelimit=ga.Operations.Get,qps,10,100 --gce-ratelimit=alpha.Operations.Get,qps,10,100 --gce-ratelimit=ga.BackendServices.Get,qps,1.8,1 --gce-ratelimit=ga.HealthChecks.Get,qps,1.8,1 --gce-ratelimit=alpha.HealthChecks.Get,qps,1.8,1 --verbose --default-backend-service=kube-system/default-http-backend --sync-period=600s --running-in-cluster=true --use-real-cloud=true --config-file-path=/etc/ingress-config/gce.conf --healthz-port=8086 2>&1'
        volumeMounts:
        - mountPath: /etc/ingress-config
          name: cloudconfig
          readOnly: true
        - mountPath: /etc/credentials
          name: credentials
          readOnly: true
      volumes:
      - name: cloudconfig
        configMap:
          name: ingress-controller-config
      - name: credentials
        secret:
          secretName: glbc-gcp-key
`
