# Printer Columns

All our CRD objects should have the following `additionalPrinterColumns` order (if the respective field exists in the CRD):
* Namespace (added automatically)
* Name (added automatically)
* ClusterClass and/or Cluster owning this resource
* Available or Ready condition
* Replica-related fields
* Other fields for -o wide (fields with priority `1` are only shown with `-o wide` and not per default)
* Paused (only shows with -o wide)
* Phase
* Age (mandatory field for all CRDs)
* Version

***NOTE***: The columns can be configured via the `kubebuilder:printcolumn` annotation on root objects. For examples, please see the `./api` package.

Examples:
```bash
kubectl get kubeadmcontrolplane
```
```bash
NAMESPACE            NAME                               INITIALIZED   API SERVER AVAILABLE   REPLICAS   READY   UPDATED   UNAVAILABLE   AGE     VERSION
quick-start-d5ufye   quick-start-ntysk0-control-plane   true          true                   1          1       1                       2m44s   v1.23.3
```
```bash
kubectl get machinedeployment
```
```bash
NAMESPACE            NAME                      CLUSTER              REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE       AGE     VERSION
quick-start-d5ufye   quick-start-ntysk0-md-0   quick-start-ntysk0   1                  1         1             ScalingUp   3m28s   v1.23.3
```