# How to release this project

0. Make sure the manger image patch is pointed to the image tag you will be releasing.
    0. If it is not, update it and open a PR. This is the commit you will use to release.
1. `git pull` to get the latest merge commit.
1. Tag the merge commit.
1. Push the tag.
2. GithubActions take over from here.
3. Update the release notes.
4. Send notifications to relevant slack channels and mailing lists.

## Expected artifacts

1. A release with no binaries.
2. A container image pushed to the staging bucket tagged with the released version.
