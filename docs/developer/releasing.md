# Releasing
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Output](#output)
  - [Expected artifacts](#expected-artifacts)
  - [Artifact locations](#artifact-locations)
- [Process](#process)
  - [Permissions](#permissions)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Output

### Expected artifacts

1. A container image of the shared cluster-api controller manager
2. A git tag for providers to use

### Artifact locations

1. The container image is found in the registry `us.gcr.io/k8s-artifacts-prod/cluster-api/` with an image
   name of `cluster-api-controller` and a tag that matches the release version. For
   example, in the `v0.1.5` release, the container image location is
   `us.gcr.io/k8s-artifacts-prod/cluster-api/cluster-api-controller:v0.1.5`

2. Prior to the `v0.1.5` release, the container image is found in the registry
   `gcr.io/k8s-cluster-api` with an image name of `cluster-api-controller` and a tag
   that matches the release version. For example, in the `v0.1.4` release, the container
   image location is `gcr.io/k8s-cluster-api/cluster-api-controller:v0.1.4`

3. Prior to the `v0.1.4` release, the container image is found in the
   registry `gcr.io/k8s-cluster-api` with an image name of `cluster-api-controller`
   and a tag that matches the release version. For example, in the `0.1.3` release,
   the container image location is `gcr.io/k8s-cluster-api/cluster-api-controller:0.1.3`

## Process

For version v0.x.y:

1. Create an annotated tag `git tag -a v0.x.y -m v0.x.y`
    1. To use your GPG signature when pushing the tag, use `git tag -s [...]` instead
1. Push the tag to the GitHub repository `git push origin v0.x.y`
    1. NB: `origin` should be the name of the remote pointing to `github.com/kubernetes-sigs/cluster-api`
1. Run `make release` to build artifacts and push the images to the staging bucket
1. Follow the [Image Promotion process](https://github.com/kubernetes/k8s.io/tree/master/k8s.gcr.io#image-promoter) to promote the image from the staging repo to `us.gcr.io/k8s-artifacts-prod/cluster-api`
1. Create a release in GitHub based on the tag created above
1. Manually create the release notes by going through the merged PRs since the last release

### Permissions

Releasing requires a particular set of permissions.

* Push access to the staging gcr bucket
* Tag push access to the GitHub repository
* GitHub Release creation access
