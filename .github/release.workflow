workflow "Release" {
  on = "push"
  resolves = ["Setup Google Cloud"]
}

action "Setup Google Cloud" {
  uses = "actions/gcloud/auth@master"
  secrets = ["GCLOUD_AUTH"]
}

action "is-tag" {
  uses = "actions/bin/filter@master"
  args = "tag"
}

action "goreleaser" {
  uses = "docker://goreleaser/goreleaser"
  secrets = []
  args = "release"
  needs = ["is-tag", "Setup Google Cloud"]
}
