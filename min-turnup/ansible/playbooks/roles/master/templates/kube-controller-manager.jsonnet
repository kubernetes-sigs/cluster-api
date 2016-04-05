function(cfg)
  {
    apiVersion: "v1",
    kind: "Pod",
    metadata: {
      name: "kube-controller-manager",
      namespace: "kube-system",
      labels: {
        tier: "control-plane",
        component: "kube-controller-manager",
      },
    },
    spec: {
      hostNetwork: true,
      containers: [
        {
          name: "kube-controller-manager",
          image: "%(docker_registry)s/kube-controller-manager:%(kubernetes_version)s" % cfg.cluster,
          resources: {
            requests: {
              cpu: "200m",
            },
          },
          command: [
            "/bin/sh",
            "-c",
            |||
              /usr/local/bin/kube-controller-manager \
                --master=127.0.0.1:8080 \
                --cluster-name=k-1 \
                --cluster-cidr=10.244.0.0/16 \
                --allocate-node-cidrs=true \
                --cloud-provider=gce \
                --service-account-private-key-file=/srv/kubernetes/apiserver-key.pem \
                --root-ca-file=/srv/kubernetes/ca.pem \
                --v=2
            |||,
          ],
          livenessProbe: {
            httpGet: {
              host: "127.0.0.1",
              port: 10252,
              path: "/healthz",
            },
            initialDelaySeconds: 15,
            timeoutSeconds: 15,
          },
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
