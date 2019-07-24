workflow "Release" {
  on = "push"
  resolves = ["goreleaser"]
}

action "Setup Google Cloud" {
  uses = "actions/gcloud/auth@master"
  secrets = ["GCLOUD_AUTH"]
}

action "is-tag" {
  uses = "actions/bin/filter@master"
  args = "tag"
}

action "Set Credential Helper for Docker" {
  needs = ["Setup Google Cloud"]
  uses = "actions/gcloud/cli@master"
  args = ["auth", "configure-docker", "--quiet"]
}

action "goreleaser" {
  uses = "docker://goreleaser/goreleaser"
  secrets = ["GORELEASER_GITHUB_TOKEN"]
  args = "release"
  needs = ["is-tag", "Setup Google Cloud", "Set Credential Helper for Docker"]
}

action "push images" {
  uses = "actions/docker/cli@master"
  args = "images --filter reference=gcr.io/kubernetes1-226021/capd-manager --format '{{.Repository}}:{{.Tag}}' | xargs -n1 docker push"
  needs = ["goreleaser"]
}
