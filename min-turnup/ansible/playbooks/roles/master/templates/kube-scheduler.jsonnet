function(cfg)
  {
    apiVersion: "v1",
    kind: "Pod",
    metadata: {
      name: "kube-scheduler",
      namespace: "kube-system",
      labels: {
        tier: "control-plane",
        component: "kube-scheduler",
      },
    },
    spec: {
      hostNetwork: true,
      containers: [
        {
          name: "kube-scheduler",
          image: "%(docker_registry)s/kube-scheduler:%(kubernetes_version)s" % cfg.cluster,
          resources: {
            requests: {
              cpu: "100m",
            },
          },
          command: [
            "/bin/sh",
            "-c",
            "/usr/local/bin/kube-scheduler --master=127.0.0.1:8080 --v=2",
          ],
          livenessProbe: {
            httpGet: {
              host: "127.0.0.1",
              port: 10251,
              path: "/healthz",
            },
            initialDelaySeconds: 15,
            timeoutSeconds: 15,
          },
        },
      ],
    },
  }
