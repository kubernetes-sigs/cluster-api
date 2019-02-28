# Building

## Fixup Makefile to include common Cluster API CRDs

Apply the following patch:

```bash
diff --git a/Makefile b/Makefile
index ac12c7e..cf44312 100644
--- a/Makefile
+++ b/Makefile
@@ -22,12 +22,15 @@ install: manifests
 
 # Deploy controller in the configured Kubernetes cluster in ~/.kube/config
 deploy: manifests
-	kubectl apply -f config/crds
-	kustomize build config/default | kubectl apply -f -
+	cat provider-components.yaml | kubectl apply -f -
 
 # Generate manifests e.g. CRD, RBAC etc.
 manifests:
 	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all
+	sed -i'' -e 's@^- manager_auth_proxy_patch.yaml.*@#&@' config/default/kustomization.yaml
+	kustomize build config/default/ > provider-components.yaml
+	echo "---" >> provider-components.yaml
+	kustomize build vendor/sigs.k8s.io/cluster-api/config/default/ >> provider-components.yaml
 
 # Run go fmt against code
 fmt:
```

## Build and push images

Determine a location to upload your container and then build and push it:

```bash
export IMG=<your-docker-registry>/cluster-api-provider-solas
dep ensure
make
make docker-build IMG=${IMG}
make docker-push IMG=${IMG}
```

## Start minikube and deploy the provider

```bash
minikube start
make deploy
```

## Verify deployment

**TODO**: Should deploy a sample `Cluster` and `Machine` resource to verify 
controllers are reconciling properly. This is troublesome however since before
the actuator stubs are filled in, all we will see are messages to the effect of
"TODO: Not yet implemented"..."

```bash
kubectl logs cluster-api-provider-solas-controller-manager-0 -n cluster-api-provider-solas-system  
```
