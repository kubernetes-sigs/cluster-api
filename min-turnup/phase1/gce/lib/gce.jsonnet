function(cfg)
  local p1 = cfg.phase1;
  local gce = p1.gce;
  local names = {
    instance_template: "%(instance_prefix)s-minion-instance-template" % p1,
    instance_group: "%(instance_prefix)s-minion-group" % p1,
    master_instance: "%(instance_prefix)s-master" % p1,
    master_ip: "%(instance_prefix)s-master-ip" % p1,
    master_firewall_rule: "%(instance_prefix)s-master-https" % p1,
    minion_firewall_rule: "%(instance_prefix)s-minion-all" % p1,
    release_bucket: "%s-kube-deploy-%s" % [gce.project, p1.instance_prefix],
  };
  local instance_defaults = {
    machine_type: gce.instance_type,
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
        project: gce.project,
        region: gce.region,
      },
    },
    resource: {
      google_compute_network: {
        network: {
          name: gce.network,
          auto_create_subnetworks: true,
        },
      },
      google_compute_address: {
        [names.master_ip]: {
          name: names.master_ip,
          region: gce.region,
          provisioner: [
            {
              "local-exec": {
                command: |||
                  cat <<EOF > ../../phase1b/crypto/san-extras
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
          target_tags: ["%(instance_prefix)s-master" % p1],
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
          target_tags: ["%(instance_prefix)s-node" % p1],
        },
      },
      google_compute_instance: {
        [names.master_instance]: instance_defaults {
          name: names.master_instance,
          zone: gce.zone,
          tags: [
            "%(instance_prefix)s-master" % p1,
            "%(instance_prefix)s-node" % p1,
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
            image: gce.os_image,
          }],
          service_account: [
            { scopes: ["compute-rw", "storage-ro"] },
          ],
        },
      },
      google_compute_instance_template: {
        [names.instance_template]: instance_defaults {
          name: names.instance_template,
          tags: ["%(instance_prefix)s-node" % p1],
          metadata: {
            "startup-script": std.escapeStringDollars(importstr "../configure-vm.sh"),
            "k8s-role": "node",
            "k8s-config": config_metadata_template % [names.master_ip, "node"],
          },
          disk: [{
            source_image: gce.os_image,
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
          base_instance_name: "%(instance_prefix)s-minion" % p1,
          zone: gce.zone,
          target_size: p1.num_nodes,
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
              command: "make -C ../../phase1b/crypto clean && make -C ../../phase1b/crypto",
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
          source: "../../phase1b/crypto/all.tar",
          bucket: names.release_bucket,
          depends_on: [
            "google_storage_bucket.%(release_bucket)s" % names,
            "null_resource.crypto_assets",
          ],
        },
        crypto_apiserver: {
          name: "crypto/apiserver.tar",
          source: "../../phase1b/crypto/apiserver.tar",
          bucket: names.release_bucket,
          depends_on: [
            "google_storage_bucket.%(release_bucket)s" % names,
            "null_resource.crypto_assets",
          ],
        },
        crypto_kubelet: {
          name: "crypto/kubelet.tar",
          source: "../../phase1b/crypto/kubelet.tar",
          bucket: names.release_bucket,
          depends_on: [
            "google_storage_bucket.%(release_bucket)s" % names,
            "null_resource.crypto_assets",
          ],
        },
        crypto_root: {
          name: "crypto/root.tar",
          source: "../../phase1b/crypto/root.tar",
          bucket: names.release_bucket,
          depends_on: [
            "google_storage_bucket.%(release_bucket)s" % names,
            "null_resource.crypto_assets",
          ],
        },
      },
    },
  }
