# Configure the controller manifest

`kubebuilder` generates most of the YAML you'll need to deploy your controller into Kubernetes by using a Deployment.
You just need to modify it to add the `MAILGUN_DOMAIN`, `MAILGUN_API_KEY` and `MAIL_RECIPIENT` environment variables
introduced in the previous steps.

First, let's add our environment variables as a [patch] to the manager yaml.

`config/manager/manager_config.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: system
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: MAILGUN_API_KEY
          valueFrom:
            secretKeyRef:
              name: mailgun-secret
              key: api_key
        - name: MAILGUN_DOMAIN
          valueFrom:
            configMapKeyRef:
              name: mailgun-config
              key: mailgun_domain
        - name: MAIL_RECIPIENT
          valueFrom:
            configMapKeyRef:
              name: mailgun-config
              key: mail_recipient
```

And then, we have to add that patch to [`config/kustomization.yaml`][kustomizeyaml]:

```yaml
patches:
- path: manager_image_patch.yaml
- path: manager_config.yaml
```

[kustomizeyaml]: https://kubectl.docs.kubernetes.io/references/kustomize/kustomization
[patch]: https://git.k8s.io/community/contributors/devel/sig-api-machinery/strategic-merge-patch.md

As you might have noticed, we are reading variable values from a `ConfigMap` and a `Secret`.

You now have to add those to the manifest, but how to inject configuration in production?
The convention many Cluster-API projects use is environment variables.

`config/manager/configuration.yaml`

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: mailgun-config
  namespace: system
type: Opaque
stringData:
  api_key: ${MAILGUN_API_KEY}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: mailgun-config
  namespace: system
data:
  mailgun_domain: ${MAILGUN_DOMAIN}
  mail_recipient: ${MAILGUN_RECIPIENT}
```

And add this to `config/manager/kustomization.yaml`

```yaml
resources:
- manager.yaml
- credentials.yaml
```

You can now (hopefully) generate your yaml!

```bash
kustomize build config/default
```

## EnvSubst

_A tool like [direnv](https://direnv.net/) can be used to help manage environment variables._

`kustomize` does not handle replacing those `${VARIABLES}` with actual values.
For that, we use [`envsubst`][envsubst].

You'll need to have those environment variables (`MAILGUN_API_KEY`, `MAILGUN_DOMAIN`, `MAILGUN_RECIPIENT`) in your environment when you generate the final yaml file.

Change `Makefile` to include the call to `envsubst`:

```diff
-	$(KUSTOMIZE) build config/default | kubectl apply -f -
+	$(KUSTOMIZE) build config/default | envsubst | kubectl apply -f -
```

To generate the manifests, call envsubst in line, like so:

```bash
kustomize build config/default | envsubst
```

Or to build and deploy the CRDs and manifests directly:

```bash
make install deploy
```

[envsubst]: https://github.com/drone/envsubst
