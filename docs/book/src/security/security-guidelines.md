# Security Guidelines for Cluster API Users

This document compiles security best practices for using Cluster API. We recommend that organizations adapt these guidelines to their specific infrastructure and security requirements to ensure safe operations.

## Comprehensive auditing

To ensure comprehensive auditing, the following components require audit configuration:

- **Cluster-level Auditing**
  - Auditing on the management cluster
  - API server auditing for all workload clusters

- **Node/VM-level Auditing**
  - Audit KubeConfig files access that are located on the node
  - Audit access or edits to CA private keys and cert files located on the node

- **Cloud Provider Auditing**
  - Cloud API auditing to log all actions performed using cloud credentials

After configuring these audit sources, centralize the logs using aggregation tools and implement real-time monitoring and alerting to detect suspicious activities and security incidents.

## Use least privileges

To minimize security risks related to cloud provider access, create dedicated cloud credentials that have only the necessary permissions to manage the lifecycle of a cluster. Avoid using administrative or root accounts for Cluster API operations, and use separate credentials for different purposes such as management cluster versus workload clusters.

## Limit access

Implement access restrictions to protect cluster infrastructure.

### Control Plane Protection

Limit who can create pods on control plane nodes through multiple methods:

- **Taints and Tolerations**: Apply `NoSchedule` taints to control plane nodes to prevent general workload scheduling. See [Kubernetes Taints and Tolerations documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/)
- **RBAC Policies**: Restrict pod creation permissions using Role-Based Access Control. See [Kubernetes RBAC documentation](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- **Admission Controllers**: Implement admission webhooks to enforce pod placement policies. See [Dynamic Admission Control](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)

### SSH Access

Disable or restrict SSH access to nodes in a cluster to prevent unauthorized modifications and access to sensitive files.

## Second pair of eyes

Implement a review process where at least two people must approve privileged actions such as creating, deleting, or updating clusters. GitOps provides an effective way to enforce this requirement through pull request workflows, where changes to cluster configurations must be reviewed and approved by another team member before being merged and applied to the infrastructure.

## Implement comprehensive alerting

Configure alerts in the centralized audit log system to detect security incidents and resource anomalies.

### Security Event Monitoring

- Alert when cluster API components are modified, restarted, or experience unexpected state changes
- Monitor and alert on unauthorized changes to sensitive files on machine images
- Alert on unexpected machine restarts or shutdowns
- Monitor deletion or modification of Elastic Load Balancers (ELB) for API servers

### Resource Activity Monitoring

- Alert on all cloud resource creation, update, and deletion activities
- Identify anomalous patterns such as mass resource creation or deletion
- Monitor for resources created outside expected boundaries

### Resource Limit Monitoring

- Alert when the number of clusters approaches or exceeds defined soft limits
- Monitor node creation rates and alert when approaching capacity limits
- Track usage against cloud provider quotas and organizational limits
- Alert on excessive API calls or resource creation requests

## Cluster isolation and segregation

Implement multiple layers of isolation to prevent privilege escalation from workload clusters to management cluster.

### Account/Subscription Separation

Separate workload clusters into different AWS accounts or Azure subscriptions, and use dedicated accounts for management cluster and production workloads. This approach provides a strong security boundary at the cloud provider level.

### Network Boundaries

Separate workload and management clusters at the network level through VPC boundaries. Use dedicated VPC/VNet for each cluster type to prevent lateral movement between clusters.

### Certificate Authority Isolation

Do not build a chain of trust for cluster CAs. Each cluster must have its own independent CA to ensure that workload cluster CA compromise does not provide access to the management cluster. See [Kubernetes PKI certificates and requirements](https://kubernetes.io/docs/setup/best-practices/certificates/) for best practices.

## Prevent runtime updates

Implement controls to prevent tampering of machine images at runtime. Disable or restrict updates to machine images at runtime and prevent unauthorized modifications through SSH access restrictions. Following [immutable infrastructure](https://glossary.cncf.io/immutable-infrastructure/) practices ensures that any changes require deploying new images rather than modifying running systems.
