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

## Promote images from the staging repo to `registry.k8s.io/cluster-api`

Images are built by the [post push images job](https://testgrid.k8s.io/sig-cluster-lifecycle-image-pushes#post-cluster-api-push-images). This will push the image to a [staging repository](https://console.cloud.google.com/gcr/images/k8s-staging-cluster-api).

1. If you don't have a GitHub token, create one by going to your GitHub settings, in [Personal access tokens](https://github.com/settings/tokens). Make sure you give the token the `repo` scope.
1. Wait for the above job to complete for the tag commit and for the image to exist in the staging repository, then create a PR to promote the image and tag:
   - `export GITHUB_TOKEN=<your GH token>`
   - `make promote-images`

*Note*: `kpromo` uses `git@github.com:...` as remote to push the branch for the PR. If you don't have `ssh` set up you can configure 
        git to use `https` instead via `git config --global url."https://github.com/".insteadOf git@github.com:`.

This will automatically create a PR in [k8s.io](https://github.com/kubernetes/k8s.io) and assign the CAPI maintainers.

## Release in GitHub

1. Review the draft release on GitHub. Pay close attention to the `## :question: Sort these by hand` section, as it contains items that need to be manually sorted.
1. Publish the release

## Update Netlify for Cluster API book

_Note: Only requried for new major and minor releases._

Get someone with Netlify access to do the following:

1. Trigger `renew certificate` on Netlify UI.
1. Change production branch on Netlify UI to the newest release branch.
1. Trigger a re-deploy on Netlify UI for book to point to the new release-branch.

## Homebrew

1. Publish `clusterctl` to Homebrew by bumping the version in [clusterctl.rb](https://github.com/Homebrew/homebrew-core/blob/master/Formula/clusterctl.rb).
   For an example please see: [PR: clusterctl 1.1.5](https://github.com/Homebrew/homebrew-core/pull/105075/files).

   **Note**: Homebrew has [conventions for commit messages](https://docs.brew.sh/Formula-Cookbook#commit) usually 
   the commit message for us should be e.g. "clusterctl 1.1.5"

### Versioning

See the [versioning documentation](./../../CONTRIBUTING.md#versioning) for more information.

### Permissions

Releasing requires a particular set of permissions.

* Tag push access to the GitHub repository
* GitHub Release creation access
