---
layout: documentation
title: Global Environmental Variables list
date: 2017-08-17
doctype: general
---

{:.table}
Variable | Type | Description
--- | --- | ---
KUBICORN_STATE_STORE | string | The state store type to use for the cluster
KUBICORN_STATE_STORE_PATH | string | The state store path to use
KUBICORN_NAME | string | The name of the cluster to use
KUBICORN_PROFILE | string | The profile name to create new clusters APIs with
KUBICORN_SET | string | Set custom property for the cluster
KUBICORN_TRUECOLOR | bool | Always run `kubicorn` with lolgopher truecolor
KUBICORN_FORCE_DELETE_KEY | bool | Force delete key for AWS
KUBICORN_FORCE_DISABLE_SSH_AGENT | bool | Force SCP and SSH to never use SSH agent
--- | --- | ---
AWS_ACCESS_KEY_ID | string | The AWS access key to use with AWS profiles - Optional, see [AWS Walkthrough](http://kubicorn.io/documentation/aws-walkthrough.html)
AWS_SECRET_ACCESS_KEY | string | The AWS secret to use with AWS profiles - Optional, see [AWS Walkthrough](http://kubicorn.io/documentation/aws-walkthrough.html)
--- | --- | ---
DIGITALOCEAN_ACCESS_TOKEN | string | The DigitalOcean access token used to authenticate with the API
--- | --- | ---
GOOGLE_APPLICATION_CREDENTIALS | string | The location of the Google service account key file
