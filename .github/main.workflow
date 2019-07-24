workflow "Release" {
  on = "push"
  resolves = ["push images"]
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
  needs = ["is-tag"]
}

action "tag images" {
  uses = "actions/docker/tag@master"
  args = "capd-manager gcr.io/kubernetes1-226021/capd-manager"
  needs = ["goreleaser"]
}

action "push images" {
  uses = "actions/docker/cli@master"
  runs = "sh -c"
  env = {
    IMAGE_NAME = "gcr.io/kubernetes1-226021/capd-manager"
  }
  args = "source $HOME/.profile && docker push $IMAGE_NAME:latest && docker push $IMAGE_NAME:$IMAGE_REF && docker push $IMAGE_NAME:$IMAGE_SHA && docker push $IMAGE_NAME:$IMAGE_VERSION"
  needs = ["tag images", "Set Credential Helper for Docker"]
}
