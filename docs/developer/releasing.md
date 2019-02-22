# Releasing

## Output

### Expected artifacts

1. A container image of the shared cluster-api controller manager
2. A git tag for providers to use

### Artifact locations

1. The container image is found in the registry `gcr.io/k8s-cluster-api` with an
   image name of `cluster-api-controller` and a tag that matches the release
   version. For example, in the `0.0.0-alpha.4` release, the container image
   location is `gcr.io/k8s-cluster-api/cluster-api-controller:0.0.0-alpha.4`

## Process

1. Create a pull request that contains two changes:
   1. Change [the cluster api controller manager image
   tag][managerimg] from `:latest` to whatever version is being released
   2. Change the `CONTROLLER_IMAGE` variable in the [Makefile][makefile] to the
      version being released
2. Get the pull request merged
3. From the commit in step 1 (that is now in the master branch), build and push
   the container image with `make docker-push`
4. Create a tag from this same commit and push the tag to the github repository
5. Revert the commit made in step 1
6. Open a pull request with the revert change
7. Get that pull request merged
8. Create a release in github based on the tag created above
9. Manually create the release notes by going through the merged PRs since the
   last release

[managerimg]: https://github.com/kubernetes-sigs/cluster-api/blob/fab4c07ea9fb0f124a5abe3dd7fcfffc23f2a1b3/config/default/manager_image_patch.yaml
[makefile]: https://github.com/kubernetes-sigs/cluster-api/blob/fab4c07ea9fb0f124a5abe3dd7fcfffc23f2a1b3/Makefile

### Permissions

Releasing requires a particular set of permissions.

* push access to the gcr bucket
* tag push access to the github repository
* release creation
