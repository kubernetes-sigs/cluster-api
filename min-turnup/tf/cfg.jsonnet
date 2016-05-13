{
  cloud_provider: "gce",
  project: "mikedanese-k8s",
  region: "us-central1",
  zone: "us-central1-b",
  instance_prefix: "k-0",
  network: "testing",
  num_nodes: 1,
  minion: {
    machine_type: "n1-standard-4",
    image: "ubuntu-1604-xenial-v20160420c",
    #image: "debian-8-jessie-v20160418",
    #image: "rhel-7-v20160418",
    #image: "coreos-stable-899-15-0-v20160405",
  },
  cluster: {
    docker_registry: "gcr.io/mikedanese-k8s",
    kubernetes_version: "v1.3.0-alpha.1.855-daf6be1a665651",
  },
}
