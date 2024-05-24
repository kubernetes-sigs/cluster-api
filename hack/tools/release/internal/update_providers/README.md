# Update CAPI Providers with CAPI beta releases

`provider_issues` is a go utility intended to open git issues on provider repos about the first beta minor release of CAPI.

## Pre-requisites

- Create a github token with the below access and export it in your environment as `GITHUB_ISSUE_OPENER_TOKEN`. Set the validity to the least number of days since this utility is a one time use tool per release cycle.
  - `repo:status` - Grants access to commit status on public and private repositories.
  - `repo_deployment` - Grants access to deployment statuses on public and private repositories.
  - `public_repo` - Grants access to public repositories

- Export `PROVIDER_ISSUES_DRY_RUN` environment variable to `"true"` to run the utility in dry run mode. Export it to `"false"` to create issues on the provider repositories. Example:

  ```sh
  export PROVIDER_ISSUES_DRY_RUN="true"
  ```

- Export `RELEASE_TAG` environment variable to the CAPI release version e.g. `v1.7.0-beta.0`. 
  Example:

  ```sh
  export RELEASE_TAG="v1.7.0-beta.0"
  ```

- Export `RELEASE_DATE` to the targeted CAPI release version date. Fetch the target date from latest [release file](https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/release/releases). The `RELEASE_DATE` should be in the format `YYYY-MM-DD`.
  Example:

  ```sh
  export RELEASE_DATE="2023-11-28"
  ```

## How to run the tool

- Finish the pre-requites [tasks](#pre-requisites).
- From the root of the project Cluster API, run `make release-provider-issues-tool` to create issues at provider repositories.
  - Note that the utility will  
    - do a dry run on setting `PROVIDER_ISSUES_DRY_RUN="true"` (and will not create git issues)
    - create git issues on setting `PROVIDER_ISSUES_DRY_RUN="false"`.
