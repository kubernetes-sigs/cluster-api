workflow "Docker image for master" {
  on = "push"
  resolves = ["push"]
}

action "master" {
  uses = "actions/bin/filter@master"
  args = "branch master"
}

action "Docker Registry" {
  needs = ["master"]
  uses = "actions/docker/login@86ff551d26008267bb89ac11198ba7f1d807b699"
  secrets = ["DOCKER_USERNAME", "DOCKER_PASSWORD", "DOCKER_REGISTRY_URL"]
}

action "build" {
  needs = ["Docker Registry"]
  uses = "actions/docker/cli@master"
  args = "build -t base ."
}

action "tag" {
  needs = ["build"]
  uses = "actions/docker/tag@master"
  args = "base docker.pkg.github.com/kubernetes-sigs/cluster-api-provider-docker/manager --no-sha --no-latest"
}

action "push" {
  needs = ["tag"]
  uses = "actions/docker/cli@master"
  args = "push docker.pkg.github.com/kubernetes-sigs/cluster-api-provider-docker/manager"
}
