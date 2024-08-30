# Infrastructure Provider Security Guidance

- Ensure credentials used by Cluster API are least privileged and setting access control
on Cluster API controller namespaces to prevent unauthorized access by anyone other
than cloud admin.
- Implement 2FA for all maintainer accounts on Github. Apply the second pair of eyes
principle when performing privileged actions such as image building or updates to the
contents of the machine images.
- Use short-lived credentials that are auto-renewed using node level attestation.
- Implement rate limits for creation, deletion and update of cloud resources.
- Any cloud resource not linked to a cluster after a fixed configurable period of time
created by these cloud credentials, should be auto-deleted or marked for garbage collection.
