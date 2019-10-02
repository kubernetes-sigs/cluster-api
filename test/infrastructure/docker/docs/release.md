# Creating a release

Tag the repository with the version you want

`git tag -a v0.1.2 -m 'a patch release'`

then push the tag

`git push origin refs/tags/v0.1.2`

Github actions will take care of the rest.

Please edit the generated change log.

## Container Images

Container images must be pushed from your local machine.

### via Goreleaser

If you have [goreleaser](https://goreleaser.com/) installed you can run
`goreleaser --skip-publish` and push the generated images.

### manually

Manually build the Dockerfile using `docker build`. The tag pattern is:

* `:latest` - This is the latest release
* `:<major>.<minor>` - This is the latest minor release in the series
* `:<major>.<minor>.<patch>` - This is the specific version
