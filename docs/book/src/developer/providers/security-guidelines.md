# Infrastructure Provider Security Guidance

There are several critical areas that any infrastructure provider implementer must address to ensure secure operations. These include:

- **Management of cloud credentials** assigned to the infrastructure provider, including setting quotas and rate limiting.
- **Ensuring secure access to VMs** for troubleshooting, with proper authentication methods.
- **Controlling manual operations** performed on cloud infrastructure targeted by the provider.
- **Housekeeping** of the cloud infrastructure, ensuring timely cleanup and garbage collection of unused resources.
- **Securing Machine's bootstrap data** ensuring protection of oversensitive data that might be included in it.

The following list outlines high-level security recommendations. It is a community-maintained resource, and everyone’s contributions are essential to continuously improve and adapt these best practices. Each provider implementer is responsible for translating these recommendations to fit the context of their specific cloud provider:

1. **Credentials Management**:
   Ensure credentials used by Cluster API are least privileged. Apply access control to Cluster API controller namespaces, restricting unauthorized access to cloud administrators only.

2. **Two-Factor Authentication (2FA)**:
   Implement 2FA for all maintainer accounts on GitHub. For any privileged actions (e.g., image building or updates to machine images), follow the "second pair of eyes" principle to ensure review and oversight.

3. **Short-lived Credentials**:
   Use short-lived credentials that are automatically renewed via node-level attestation mechanisms, minimizing the risk of credential misuse.

4. **Rate Limiting for Cloud Resources**:
   Implement rate limits for the creation, deletion, and updating of cloud resources, protecting against potential abuse or accidental overload.

5. **Resource Housekeeping**:
   Any cloud resource not linked to a cluster after a fixed configurable period, created by cloud credentials, should be automatically deleted or marked for garbage collection to avoid resource sprawl.

6. **Securing Machine's bootstrap data**:
   Bootstrap data are usually stored in machine's metadata, and they might contain sensitive data, like e.g. Cluster secrets, user credentials, ssh certificates etc. It is important to ensure protection of this metadata, or if not possible, to clean it up immediately after machine bootstrap.
