workflow "New workflow" {
  on = "push"
  resolves = ["./scripts/publish-manager.sh"]
}

action "Docker Registry" {
  uses = "actions/docker/login@86ff551d26008267bb89ac11198ba7f1d807b699"
}

action "./scripts/publish-manager.sh" {
  uses = "./scripts/publish-manager.sh"
  needs = ["Docker Registry"]
  env = {
    TAG = "latest"
    REGISTRY = "docker.pkg.github.com/kubernetes-sigs/cluster-api-provider-docker"
  }
}
