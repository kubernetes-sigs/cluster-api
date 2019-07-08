workflow "New workflow" {
  on = "push"
  resolves = ["push"]
}

action "Docker Registry" {
  uses = "actions/docker/login@86ff551d26008267bb89ac11198ba7f1d807b699"
  secrets = ["DOCKER_USERNAME", "DOCKER_PASSWORD", "DOCKER_REGISTRY_URL"]
}

action "build" {
  uses = "actions/docker/cli@master"
  needs = ["Docker Registry"]
  args = "build -t docker.pkg.github.com/kubernetes-sigs/cluster-api-provider-docker:latest ."
}

action "push" {
  uses = "actions/docker/cli@master"
  needs = ["build"]
  args = "push docker.pkg.github.com/kubernetes-sigs/cluster-api-provider-docker:latest"
}
