
# POC

Start dev-env + controller:

```bash
./hack/kind-install-for-capd.sh
tilt up

controller=capi
mkdir -p /tmp/k8s-webhook-server-${controller}
k -n ${controller}-system get secret ${controller}-webhook-service-cert -o json | jq '.data."tls.crt"' -r | base64 -d > /tmp/k8s-webhook-server-${controller}/tls.crt
k -n ${controller}-system get secret ${controller}-webhook-service-cert -o json | jq '.data."tls.key"' -r | base64 -d > /tmp/k8s-webhook-server-${controller}/tls.key

# Start controller with:
# --webhook-cert-dir=/tmp/k8s-webhook-server-capi/
# --feature-gates=MachinePool=true,ClusterResourceSet=true,ClusterTopology=true,RuntimeSDK=true
# --metrics-bind-addr=localhost:8080
# --metrics-bind-addr=0.0.0.0:8080
# --logging-format=json
# --v=2
```

# Deploy secure

```sh
# To create service and certificate
k apply -f ./poc/test/local/secure-infra.yaml

# fetch certificate
mkdir -p /tmp/k8s-webhook-server/serving-certs
for f in $(kubectl get secret webhook-service-cert -o json | jq '.data | keys | .[]' -r); do 
  kubectl get secret my-local-extension-cert -o json | jq '.data["'$f'"]' -r | base64 -d > "/tmp/k8s-webhook-server/serving-certs/$f"
done

export CA_BUNDLE="$(cat /tmp/k8s-webhook-server/serving-certs/ca.crt | base64)"
envsubst < ./poc/test/local/secure-extension.yaml | k apply -f -
# replace caBundle in `secure-extension.yaml` with base64 encoded content of /tmp/k8s-webhook-server/serving-certs/ca.crt

# start webserver now (IDE?) rte-implementation-v1alpha1-secure

# Deploy Extension
kubectl apply -f secure-extension.yaml
```


# Deploy secure (part of e2e test)

```sh
# fetch certificate
mkdir -p /tmp/k8s-webhook-server/serving-certs
for f in $(k -n test-extension-system  get secret webhook-service-cert -o json | jq '.data | keys | .[]' -r); do 
  k -n test-extension-system get secret webhook-service-cert -o json | jq '.data["'$f'"]' -r | base64 -d > "/tmp/k8s-webhook-server/serving-certs/$f"
done

# Change ExtensionConfig to url: https://localhost

# start webserver now (IDE?) rte-implementation-v1alpha1-secure

# Deploy Extension
kubectl apply -f secure-extension.yaml
```

# Deploy insecure

```bash
# Start poc/test/runtime-extension

# Deploy Extension
kubectl apply -f ./extension.yaml
```
