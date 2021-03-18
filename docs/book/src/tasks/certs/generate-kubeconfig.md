## Generating a Kubeconfig with your own CA

1. Create a new Certificate Signing Request (CSR) for the `system:masters` Kubernetes role, or specify any other role under CN.

   ```bash
   openssl req  -subj "/CN=system:masters" -new -newkey rsa:2048 -nodes -out admin.csr -keyout admin.key  -out admin.csr
   ```

2. Sign the CSR using the *[cluster-name]-ca* key:

   ```bash
   openssl x509 -req -in admin.csr -CA tls.crt -CAkey tls.key -CAcreateserial -out admin.crt -days 5 -sha256
   ```

3. Update your kubeconfig with the sign key:

   ```bash
   kubectl config set-credentials cluster-admin --client-certificate=admin.crt --client-key=admin.key --embed-certs=true
   ```
