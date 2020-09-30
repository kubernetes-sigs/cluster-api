# Configure

## YAML

`kubebuilder` generates most of the YAML you'll need to deploy a container.
We just need to modify it to add our new secrets.

First, let's add our secret as a [patch] to the manager yaml.

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
patchesStrategicMerge
- manager_image_patch.yaml
# Protect the /metrics endpoint by putting it behind auth.
# Only one of manager_auth_proxy_patch.yaml and
# manager_prometheus_metrics_patch.yaml should be enabled.
- manager_auth_proxy_patch.yaml
# If you want your controller-manager to expose the /metrics
# endpoint w/o any authn/z, uncomment the following line and
# comment manager_auth_proxy_patch.yaml.
# Only one of manager_auth_proxy_patch.yaml and
# manager_prometheus_metrics_patch.yaml should be enabled.
- manager_config.yaml
```

[kustomizeyaml]: https://github.com/kubernetes-sigs/kustomize/blob/master/docs/glossary.md#kustomization
[patch]: https://git.k8s.io/community/contributors/devel/sig-api-machinery/strategic-merge-patch.md

## Our configuration

There's many ways to manage configuration in production.
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

```
kustomize build config/
```

## RBAC Role

The default [RBAC role][role] contains permissions for accessing your cluster infrastructure CRDs, but not for accessing Cluster API resources.
You'll need to add these to `config/rbac/role.yaml`

[role]: https://kubernetes.io/docs/reference/access-authn-authz/rbac/

```diff
diff --git a/config/rbac/role.yaml b/config/rbac/role.yaml
index e9352ce..29008db 100644
--- a/config/rbac/role.yaml
+++ b/config/rbac/role.yaml
@@ -6,6 +6,24 @@ metadata:
   creationTimestamp: null
   name: manager-role
 rules:
+- apiGroups:
+  - cluster.x-k8s.io
+  resources:
+  - clusters
+  - clusters/status
+  verbs:
+  - get
+  - list
+  - watch
+- apiGroups:
+  - cluster.x-k8s.io
+  resources:
+  - machines
+  - machines/status
+  verbs:
+  - get
+  - list
+  - watch
 - apiGroups:
   - infrastructure.cluster.x-k8s.io
   resources:
```

## EnvSubst

_A tool like [direnv](https://direnv.net/) can be used to help manage environment variables._


`kustomize` does not handle replacing those `${VARIABLES}` with actual values.
For that, we use [`envsubst`][envsubst].

You'll need to have those environment variables (`MAILGUN_API_KEY`, `MAILGUN_DOMAIN`, `MAILGUN_RECIPIENT`) in your environment when you generate the final yaml file.

Then we call envsubst in line, like so:

```
kustomize build config/ | envsubst
```

[envsubst]: https://linux.die.net/man/1/envsubst

