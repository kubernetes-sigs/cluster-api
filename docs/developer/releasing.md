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

1. We will target a branch called `release-0.x`.  If this is `v0.x.0` then we'll create a branch from master using `git push origin master:release-0.x`, otherwise simply checkout the existing branch `git checkout release-0.x`
2. Make two changes:
   1. Change [the cluster api controller manager image tag][managerimg] from `:latest` to whatever version is being released
   1. Change the `CONTROLLER_IMG` variable in the [Makefile][makefile] to the version being released (Note that we do not release the example-provider image, so we don't tag that)
3. Commit it using `git commit -m "Release v0.x.y"`
4. Submit a PR to the `release-0.x` branch, e.g. `git push $USER; hub pull-request -b release-0.x`
5. Get the pull request merged
6. Switch to the release branch and update to pick up the commit.  (e.g. `git checkout release 0.x && git pull`).  
7. Create an annotated tag from this same commit `git tag -a v0.x.y -m v0.x.y` and push the tag to the github repository `git push origin v0.x.y`
8. Build and push the container images and fat manifest with `REGISTRY="gcr.io/k8s-staging-cluster-api" make all-push` (on the 0.1 release branch, we do `make docker-push`)
9. Follow the [Image Promotion process](https://github.com/kubernetes/k8s.io/tree/master/k8s.gcr.io#image-promoter) to promote the image from the staging repo to `k8s.gcr.io/cluster-api`
10. Create a release in github based on the tag created above
11. Manually create the release notes by going through the merged PRs since the last release

[managerimg]: https://github.com/kubernetes-sigs/cluster-api/blob/fab4c07ea9fb0f124a5abe3dd7fcfffc23f2a1b3/config/default/manager_image_patch.yaml
[makefile]: https://github.com/kubernetes-sigs/cluster-api/blob/fab4c07ea9fb0f124a5abe3dd7fcfffc23f2a1b3/Makefile

### Permissions

Releasing requires a particular set of permissions.

* push access to the gcr bucket
* tag push access to the github repository
* release creation
