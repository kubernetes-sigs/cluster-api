function(cfg)
  {
    apiVersion: "v1",
    kind: "Config",
    users: [{
      name: "kubelet",
      user: {
        "client-certificate-data": std.base64(cfg.kubelet_pem_cmd.stdout),
        "client-key-data": std.base64(cfg.kubelet_key_pem_cmd.stdout),
      },
    }],
    clusters: [{
      name: "local",
      cluster: {
        "certificate-authority-data": std.base64(cfg.ca_pem_cmd.stdout),
        server: "https://%(master_ip)s" % cfg,
      },
    }],
    contexts: [{
      context: {
        cluster: "local",
        user: "kubelet",
      },
      name: "service-account-context",
    }],
    "current-context": "service-account-context",
  }
