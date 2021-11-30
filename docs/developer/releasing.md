# Release Process

## Create a tag

1. Create an annotated tag
   > NOTE: To use your GPG signature when pushing the tag, use `git tag -s [...]` instead)
   - `export RELEASE_TAG=<the tag of the release to be cut>` (eg. `export RELEASE_TAG=v1.0.1`)
   - `git tag -a ${RELEASE_TAG} -m ${RELEASE_TAG}`
   - `git tag test/${RELEASE_TAG}` (:warning: MUST NOT be an annotated tag)
1. Push the tag to the GitHub repository. This will automatically trigger a [Github Action](https://github.com/kubernetes-sigs/cluster-api/actions) to create a draft release.
   > NOTE: `origin` should be the name of the remote pointing to `github.com/kubernetes-sigs/cluster-api`
   - `git push origin ${RELEASE_TAG}`
   - `git push origin test/${RELEASE_TAG}`

## Promote images from the staging repo to `k8s.gcr.io/cluster-api`

Images are built by the [post push images job](https://testgrid.k8s.io/sig-cluster-lifecycle-image-pushes#post-cluster-api-push-images). This will push the image to a [staging repository](https://console.cloud.google.com/gcr/images/k8s-staging-cluster-api).

1. If you don't have a GitHub token, create one by going to your GitHub settings, in [Personal access tokens](https://github.com/settings/tokens). Make sure you give the token the `repo` scope.
1. Wait for the above job to complete for the tag commit and for the image to exist in the staging repository, then create a PR to promote the image and tag:
   - `export GITHUB_TOKEN=<your GH token>`
   - `make promote-images`

This will automatically create a PR in [k8s.io](https://github.com/kubernetes/k8s.io) and assign the CAPI maintainers.

## Release in GitHub

1. Review the draft release on GitHub. Pay close attention to the `## :question: Sort these by hand` section, as it contains items that need to be manually sorted.
1. Publish the release

### Versioning

See the [versioning documentation](./../../CONTRIBUTING.md#versioning) for more information.

### Permissions

Releasing requires a particular set of permissions.

* Tag push access to the GitHub repository
* GitHub Release creation access
