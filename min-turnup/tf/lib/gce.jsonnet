function(cfg)
  local names = {
    instance_template: "%s-minion-instance-template" % cfg.instance_prefix,
    instance_group: "%(instance_prefix)s-minion-group" % cfg,
    master_instance: "%(instance_prefix)s-master" % cfg,
    master_ip: "%(instance_prefix)s-master-ip" % cfg,
    master_firewall_rule: "%(instance_prefix)s-master-https" % cfg,
    minion_firewall_rule: "%(instance_prefix)s-minion-all" % cfg,
    release_bucket: "%(project)s-kube-deploy-%(instance_prefix)s" % cfg,
  };
  local instance_defaults = {
    machine_type: cfg.minion.machine_type,
    can_ip_forward: true,
    scheduling: {
      automatic_restart: true,
      on_host_maintenance: "MIGRATE",
    },
    network_interface: [{
      network: "${google_compute_network.network.name}",
      access_config: {},
    }],
  };
  local config_metadata_template = std.toString(cfg {
    master_ip: "${google_compute_address.%s.address}",
    role: "%s",
  });
  {
    provider: {
      google: {
        credentials: "${file(\"account.json\")}",
        project: cfg.project,
        region: cfg.region,
      },
    },
    resource: {
      google_compute_network: {
        network: {
          name: cfg.network,
          auto_create_subnetworks: true,
        },
      },
      google_compute_address: {
        [names.master_ip]: {
          name: names.master_ip,
          region: cfg.region,
          provisioner: [
            {
              "local-exec": {
                command: |||
                  cat <<EOF > ../crypto/san-extras
                  DNS.1 = kubernetes
                  DNS.2 = kubernetes.default
                  DNS.3 = kubernetes.default.svc
                  DNS.4 = kubernetes.default.svc.cluster.local
                  DNS.5 = %(master_instance)s
                  IP.1 = ${google_compute_address.%(master_ip)s.address}
                  IP.2 = 10.0.0.1
                  EOF
                ||| % names,
              },
            },
          ],
        },
      },
      google_compute_firewall: {
        ssh_all: {
          name: "ssh-all",
          network: "${google_compute_network.network.name}",
          allow: [{
            protocol: "tcp",
            ports: ["22"],
          }],
          source_ranges: ["0.0.0.0/0"],
        },
        [names.master_firewall_rule]: {
          name: names.master_firewall_rule,
          network: "${google_compute_network.network.name}",
          allow: [{
            protocol: "tcp",
            ports: ["443"],
          }],
          source_ranges: ["0.0.0.0/0"],
          target_tags: ["%(instance_prefix)s-master" % cfg],
        },
        [names.minion_firewall_rule]: {
          name: names.minion_firewall_rule,
          network: "${google_compute_network.network.name}",
          allow: [
            { protocol: "tcp" },
            { protocol: "udp" },
            { protocol: "icmp" },
            { protocol: "ah" },
            { protocol: "sctp" },
          ],
          source_ranges: [
            "10.0.0.0/8",
            "172.16.0.0/12",
            "192.168.0.0/16",
          ],
          target_tags: ["%(instance_prefix)s-node" % cfg],
        },
      },
      google_compute_instance: {
        [names.master_instance]: instance_defaults {
          name: names.master_instance,
          zone: cfg.zone,
          tags: [
            "%(instance_prefix)s-master" % cfg,
            "%(instance_prefix)s-node" % cfg,
          ],
          network_interface: [{
            network: "${google_compute_network.network.name}",
            access_config: {
              nat_ip: "${google_compute_address.%(master_ip)s.address}" % names,
            },
          }],
          metadata_startup_script: std.escapeStringDollars(importstr "../configure-vm.sh"),
          metadata: {
            "k8s-role": "master",
            "k8s-config": config_metadata_template % [names.master_ip, "master"],
          },
          disk: [{
            image: cfg.minion.image,
          }],
          service_account: [
            { scopes: ["compute-rw", "storage-ro"] },
          ],
        },
      },
      google_compute_instance_template: {
        [names.instance_template]: instance_defaults {
          name: names.instance_template,
          tags: ["%(instance_prefix)s-node" % cfg],
          metadata: {
            "startup-script": std.escapeStringDollars(importstr "../configure-vm.sh"),
            "k8s-role": "node",
            "k8s-config": config_metadata_template % [names.master_ip, "node"],
          },
          disk: [{
            source_image: cfg.minion.image,
            auto_delete: true,
            boot: true,
          }],
          service_account: [
            { scopes: ["compute-rw", "storage-ro"] },
          ],
        },
      },
      google_compute_instance_group_manager: {
        [names.instance_group]: {
          name: names.instance_group,
          instance_template: "${google_compute_instance_template.%(instance_template)s.self_link}" % names,
          update_strategy: "NONE",
          base_instance_name: "%(instance_prefix)s-minion" % cfg,
          zone: cfg.zone,
          target_size: cfg.num_nodes,
        },
      },
      null_resource: {
        crypto_assets: {
          depends_on: [
            "google_compute_address.%(master_ip)s" % names,
          ],
          provisioner: [{
            "local-exec": {
              # clean is covering up a bug, perhaps in the makefile?
              command: "make -C ../crypto clean && make -C ../crypto",
            },
          }],
        },
      },
      google_storage_bucket: {
        [names.release_bucket]: {
          name: names.release_bucket,
        },
      },
      google_storage_bucket_object: {
        crypto_all: {
          name: "crypto/all.tar",
          source: "../crypto/all.tar",
          bucket: names.release_bucket,
          depends_on: [
            "google_storage_bucket.%(release_bucket)s" % names,
            "null_resource.crypto_assets",
          ],
        },
        crypto_apiserver: {
          name: "crypto/apiserver.tar",
          source: "../crypto/apiserver.tar",
          bucket: names.release_bucket,
          depends_on: [
            "google_storage_bucket.%(release_bucket)s" % names,
            "null_resource.crypto_assets",
          ],
        },
        crypto_kubelet: {
          name: "crypto/kubelet.tar",
          source: "../crypto/kubelet.tar",
          bucket: names.release_bucket,
          depends_on: [
            "google_storage_bucket.%(release_bucket)s" % names,
            "null_resource.crypto_assets",
          ],
        },
        crypto_root: {
          name: "crypto/root.tar",
          source: "../crypto/root.tar",
          bucket: names.release_bucket,
          depends_on: [
            "google_storage_bucket.%(release_bucket)s" % names,
            "null_resource.crypto_assets",
          ],
        },
      },
    },
  }
