---
name: Bug report
about: Tell us about a problem you are experiencing

---

/kind bug

**What steps did you take and what happened:**
[A clear and concise description of what the bug is.]


**What did you expect to happen:**


**Anything else you would like to add:**
[Miscellaneous information that will assist in solving the issue.]


**Environment:**

- capdctl version (use `capdctl version`):
- capd-manager version (use `kubectl -n docker-provider-system get pods/docker-provider-controller-manager-0 -ojsonpath='{.status.containerStatuses[0].image}{"\n"}{.status.containerStatuses[0].imageID}{"\n"}'`):
- Kubernetes version (use `kubectl version`):
- OS (e.g. from `/etc/os-release`):
