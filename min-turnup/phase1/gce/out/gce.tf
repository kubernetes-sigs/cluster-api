{
   "provider": {
      "google": {
         "credentials": "${file(\"account.json\")}",
         "project": "placeholder",
         "region": "us-central1"
      }
   },
   "resource": {
      "google_compute_address": {
         "kuberentes-master-ip": {
            "name": "kuberentes-master-ip",
            "provisioner": [
               {
                  "local-exec": {
                     "command": "cat <<EOF > ../crypto/san-extras\nDNS.1 = kubernetes\nDNS.2 = kubernetes.default\nDNS.3 = kubernetes.default.svc\nDNS.4 = kubernetes.default.svc.cluster.local\nDNS.5 = kuberentes-master\nIP.1 = ${google_compute_address.kuberentes-master-ip.address}\nIP.2 = 10.0.0.1\nEOF\n"
                  }
               }
            ],
            "region": "us-central1"
         }
      },
      "google_compute_firewall": {
         "kuberentes-master-https": {
            "allow": [
               {
                  "ports": [
                     "443"
                  ],
                  "protocol": "tcp"
               }
            ],
            "name": "kuberentes-master-https",
            "network": "${google_compute_network.network.name}",
            "source_ranges": [
               "0.0.0.0/0"
            ],
            "target_tags": [
               "kuberentes-master"
            ]
         },
         "kuberentes-minion-all": {
            "allow": [
               {
                  "protocol": "tcp"
               },
               {
                  "protocol": "udp"
               },
               {
                  "protocol": "icmp"
               },
               {
                  "protocol": "ah"
               },
               {
                  "protocol": "sctp"
               }
            ],
            "name": "kuberentes-minion-all",
            "network": "${google_compute_network.network.name}",
            "source_ranges": [
               "10.0.0.0/8",
               "172.16.0.0/12",
               "192.168.0.0/16"
            ],
            "target_tags": [
               "kuberentes-node"
            ]
         },
         "ssh_all": {
            "allow": [
               {
                  "ports": [
                     "22"
                  ],
                  "protocol": "tcp"
               }
            ],
            "name": "ssh-all",
            "network": "${google_compute_network.network.name}",
            "source_ranges": [
               "0.0.0.0/0"
            ]
         }
      },
      "google_compute_instance": {
         "kuberentes-master": {
            "can_ip_forward": true,
            "disk": [
               {
                  "image": "ubuntu-1604-xenial-v20160420c"
               }
            ],
            "machine_type": "n1-standard-2",
            "metadata": {
               "k8s-config": "{\"cloud_provider\": {\"gce\": true}, \"docker_registry\": \"gcr.io/google-containers\", \"gce\": {\"instance_type\": \"n1-standard-2\", \"network\": \"placeholder\", \"os_image\": \"ubuntu-1604-xenial-v20160420c\", \"project\": \"placeholder\", \"region\": \"us-central1\", \"zone\": \"us-central1-b\"}, \"instance_prefix\": \"kuberentes\", \"kubernetes_version\": \"v1.2.4\", \"master_ip\": \"${google_compute_address.kuberentes-master-ip.address}\", \"num_nodes\": 4, \"role\": \"master\"}",
               "k8s-role": "master"
            },
            "metadata_startup_script": "#! /bin/bash\n\nset -o errexit\nset -o pipefail\nset -o nounset\n\nROLE=$$(curl \\\n  -H \"Metadata-Flavor: Google\" \\\n  \"metadata/computeMetadata/v1/instance/attributes/k8s-role\")\n\nmkdir -p /etc/systemd/system/docker.service.d/\ncat <<EOF > /etc/systemd/system/docker.service.d/clear_mount_propagtion_flags.conf\n[Service]\nMountFlags=shared\nEOF\n\nmkdir -p /etc/kubernetes/\ncurl -H 'Metadata-Flavor:Google' \\\n  \"metadata/computeMetadata/v1/instance/attributes/k8s-config\" \\\n  -o /etc/kubernetes/k8s_config.json\n\ncurl -sSL https://get.docker.com/ | sh\napt-get install bzip2\nsystemctl start docker || true\n\ndocker run \\\n  --net=host \\\n  -v /:/host_root \\\n  -v /etc/kubernetes/k8s_config.json:/opt/playbooks/config.json:ro \\\n  gcr.io/mikedanese-k8s/install-k8s:v1 \\\n  /opt/do_role.sh \"$${ROLE}\"\n",
            "name": "kuberentes-master",
            "network_interface": [
               {
                  "access_config": {
                     "nat_ip": "${google_compute_address.kuberentes-master-ip.address}"
                  },
                  "network": "${google_compute_network.network.name}"
               }
            ],
            "scheduling": {
               "automatic_restart": true,
               "on_host_maintenance": "MIGRATE"
            },
            "service_account": [
               {
                  "scopes": [
                     "compute-rw",
                     "storage-ro"
                  ]
               }
            ],
            "tags": [
               "kuberentes-master",
               "kuberentes-node"
            ],
            "zone": "us-central1-b"
         }
      },
      "google_compute_instance_group_manager": {
         "kuberentes-minion-group": {
            "base_instance_name": "kuberentes-minion",
            "instance_template": "${google_compute_instance_template.kuberentes-minion-instance-template.self_link}",
            "name": "kuberentes-minion-group",
            "target_size": 4,
            "update_strategy": "NONE",
            "zone": "us-central1-b"
         }
      },
      "google_compute_instance_template": {
         "kuberentes-minion-instance-template": {
            "can_ip_forward": true,
            "disk": [
               {
                  "auto_delete": true,
                  "boot": true,
                  "source_image": "ubuntu-1604-xenial-v20160420c"
               }
            ],
            "machine_type": "n1-standard-2",
            "metadata": {
               "k8s-config": "{\"cloud_provider\": {\"gce\": true}, \"docker_registry\": \"gcr.io/google-containers\", \"gce\": {\"instance_type\": \"n1-standard-2\", \"network\": \"placeholder\", \"os_image\": \"ubuntu-1604-xenial-v20160420c\", \"project\": \"placeholder\", \"region\": \"us-central1\", \"zone\": \"us-central1-b\"}, \"instance_prefix\": \"kuberentes\", \"kubernetes_version\": \"v1.2.4\", \"master_ip\": \"${google_compute_address.kuberentes-master-ip.address}\", \"num_nodes\": 4, \"role\": \"node\"}",
               "k8s-role": "node",
               "startup-script": "#! /bin/bash\n\nset -o errexit\nset -o pipefail\nset -o nounset\n\nROLE=$$(curl \\\n  -H \"Metadata-Flavor: Google\" \\\n  \"metadata/computeMetadata/v1/instance/attributes/k8s-role\")\n\nmkdir -p /etc/systemd/system/docker.service.d/\ncat <<EOF > /etc/systemd/system/docker.service.d/clear_mount_propagtion_flags.conf\n[Service]\nMountFlags=shared\nEOF\n\nmkdir -p /etc/kubernetes/\ncurl -H 'Metadata-Flavor:Google' \\\n  \"metadata/computeMetadata/v1/instance/attributes/k8s-config\" \\\n  -o /etc/kubernetes/k8s_config.json\n\ncurl -sSL https://get.docker.com/ | sh\napt-get install bzip2\nsystemctl start docker || true\n\ndocker run \\\n  --net=host \\\n  -v /:/host_root \\\n  -v /etc/kubernetes/k8s_config.json:/opt/playbooks/config.json:ro \\\n  gcr.io/mikedanese-k8s/install-k8s:v1 \\\n  /opt/do_role.sh \"$${ROLE}\"\n"
            },
            "name": "kuberentes-minion-instance-template",
            "network_interface": [
               {
                  "access_config": { },
                  "network": "${google_compute_network.network.name}"
               }
            ],
            "scheduling": {
               "automatic_restart": true,
               "on_host_maintenance": "MIGRATE"
            },
            "service_account": [
               {
                  "scopes": [
                     "compute-rw",
                     "storage-ro"
                  ]
               }
            ],
            "tags": [
               "kuberentes-node"
            ]
         }
      },
      "google_compute_network": {
         "network": {
            "auto_create_subnetworks": true,
            "name": "placeholder"
         }
      },
      "google_storage_bucket": {
         "placeholder-kube-deploy-kuberentes": {
            "name": "placeholder-kube-deploy-kuberentes"
         }
      },
      "google_storage_bucket_object": {
         "crypto_all": {
            "bucket": "placeholder-kube-deploy-kuberentes",
            "depends_on": [
               "google_storage_bucket.placeholder-kube-deploy-kuberentes",
               "null_resource.crypto_assets"
            ],
            "name": "crypto/all.tar",
            "source": "../crypto/all.tar"
         },
         "crypto_apiserver": {
            "bucket": "placeholder-kube-deploy-kuberentes",
            "depends_on": [
               "google_storage_bucket.placeholder-kube-deploy-kuberentes",
               "null_resource.crypto_assets"
            ],
            "name": "crypto/apiserver.tar",
            "source": "../crypto/apiserver.tar"
         },
         "crypto_kubelet": {
            "bucket": "placeholder-kube-deploy-kuberentes",
            "depends_on": [
               "google_storage_bucket.placeholder-kube-deploy-kuberentes",
               "null_resource.crypto_assets"
            ],
            "name": "crypto/kubelet.tar",
            "source": "../crypto/kubelet.tar"
         },
         "crypto_root": {
            "bucket": "placeholder-kube-deploy-kuberentes",
            "depends_on": [
               "google_storage_bucket.placeholder-kube-deploy-kuberentes",
               "null_resource.crypto_assets"
            ],
            "name": "crypto/root.tar",
            "source": "../crypto/root.tar"
         }
      },
      "null_resource": {
         "crypto_assets": {
            "depends_on": [
               "google_compute_address.kuberentes-master-ip"
            ],
            "provisioner": [
               {
                  "local-exec": {
                     "command": "make -C ../crypto clean && make -C ../crypto"
                  }
               }
            ]
         }
      }
   }
}
