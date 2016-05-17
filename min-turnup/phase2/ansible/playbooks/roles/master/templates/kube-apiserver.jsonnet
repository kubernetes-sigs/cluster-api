function(cfg)
  {
    apiVersion: "v1",
    kind: "Pod",
    metadata: {
      name: "kube-apiserver",
      namespace: "kube-system",
      labels: {
        tier: "control-plane",
        component: "kube-apiserver",
      },
    },
    spec: {
      hostNetwork: true,
      containers: [
        {
          name: "kube-apiserver",
          image: "%(docker_registry)s/kube-apiserver:%(kubernetes_version)s" % cfg.cluster,
          resources: {
            requests: {
              cpu: "250m",
            },
          },
          command: [
            "/bin/sh",
            "-c",
            |||
              /usr/local/bin/kube-apiserver \
                --address=127.0.0.1 \
                --etcd-servers=http://127.0.0.1:2379 \
                --cloud-provider=gce \
                --admission-control=NamespaceLifecycle,LimitRanger,ServiceAccount,PersistentVolumeLabel,ResourceQuota \
                --service-cluster-ip-range=10.0.0.0/16 \
                --client-ca-file=/srv/kubernetes/ca.pem \
                --tls-cert-file=/srv/kubernetes/apiserver.pem \
                --tls-private-key-file=/srv/kubernetes/apiserver-key.pem \
                --secure-port=443 \
                --allow-privileged \
                --v=4
            |||,
            # --basic-auth-file=/srv/kubernetes/basic_auth.csv \
            # --token-auth-file=/srv/kubernetes/known_tokens.csv \
          ],
          livenessProbe: {
            httpGet: {
              host: "127.0.0.1",
              port: 8080,
              path: "/healthz",
            },
            initialDelaySeconds: 15,
            timeoutSeconds: 15,
          },
          ports: [
            {
              name: "https",
              containerPort: 443,
              hostPort: 443,
            },
            {
              name: "local",
              containerPort: 8080,
              hostPort: 8080,
            },
          ],
          volumeMounts: [
            {
              name: "srvkube",
              mountPath: "/srv/kubernetes",
              readOnly: true,
            },
            {
              name: "etcssl",
              mountPath: "/etc/ssl",
              readOnly: true,
            },
          ],
        },
      ],
      volumes: [
        {
          name: "srvkube",
          hostPath: {
            path: "/srv/kubernetes",
          },
        },
        {
          name: "etcssl",
          hostPath: {
            path: "/etc/ssl",
          },
        },
      ],
    },
  }
