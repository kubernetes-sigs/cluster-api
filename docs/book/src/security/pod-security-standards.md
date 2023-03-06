# Pod Security Standards

Pod Security Admission allows applying [Pod Security Standards] during creation of pods at the cluster level.

The flavor `development-topology` for the Docker provider used in [Quick Start](../user/quick-start.md) already includes a basic Pod Security Standard configuration.
It is using ClusterClass variables and patches to inject the configuration.

## Adding a basic Pod Security Standards configuration to a ClusterClass

By adding the following variables and patches Pod Security Standards can be added to every ClusterClass which references a [Kubeadm based control plane](../tasks/control-plane/kubeadm-control-plane.md).

### Adding the variables to a ClusterClass

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
spec:
  variables:
  - name: podSecurityStandard
    required: false
    schema:
      openAPIV3Schema:
        type: object
        properties: 
          enabled: 
            type: boolean
            default: true
            description: "enabled enables the patches to enable Pod Security Standard via AdmissionConfiguration."
          enforce:
            type: string
            default: "baseline"
            description: "enforce sets the level for the enforce PodSecurityConfiguration mode. One of privileged, baseline, restricted."
            pattern: "privileged|baseline|restricted"
          audit:
            type: string
            default: "restricted"
            description: "audit sets the level for the audit PodSecurityConfiguration mode. One of privileged, baseline, restricted."
            pattern: "privileged|baseline|restricted"
          warn:
            type: string
            default: "restricted"
            description: "warn sets the level for the warn PodSecurityConfiguration mode. One of privileged, baseline, restricted."
            pattern: "privileged|baseline|restricted"
  ...
```

* The version field in Pod Security Admission Config defaults to `latest`.
* The `kube-system` namespace is exempt from Pod Security Standards enforcement, because it runs control-plane pods that need higher privileges.

### Adding the patches to a ClusterClass

The following snippet contains the patch to be added to the ClusterClass.

Due to [limitations of ClusterClass with patches](../tasks/experimental-features/cluster-class/write-clusterclass.md#json-patches-tips--tricks) there are two versions for this patch.

{{#tabs name:"tab-configuration-patches" tabs:"Add to existing slice,Create slice"}}
{{#tab Append}}

Use this patch if the following keys **already exist** inside the `KubeadmControlPlaneTemplate` referred by the ClusterClass:

- `.spec.template.spec.kubeadmConfigSpec.clusterConfiguration.apiServer.extraVolumes`
- `.spec.template.spec.kubeadmConfigSpec.files`

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
spec:
  ...
  patches:
  - name: podSecurityStandard
    description: "Adds an admission configuration for PodSecurity to the kube-apiserver."
    definitions:
    - selector:
        apiVersion: controlplane.cluster.x-k8s.io/v1beta1
        kind: KubeadmControlPlaneTemplate
        matchResources:
          controlPlane: true
      jsonPatches:
      - op: add
        path: "/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/apiServer/extraArgs"
        value:
          admission-control-config-file: "/etc/kubernetes/kube-apiserver-admission-pss.yaml"
      - op: add
        path: "/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/apiServer/extraVolumes/-"
        value:
          name: admission-pss
          hostPath: /etc/kubernetes/kube-apiserver-admission-pss.yaml
          mountPath: /etc/kubernetes/kube-apiserver-admission-pss.yaml
          readOnly: true
          pathType: "File"
      - op: add
        path: "/spec/template/spec/kubeadmConfigSpec/files/-"
        valueFrom:
          template: |
            content: |
              apiVersion: apiserver.config.k8s.io/v1
              kind: AdmissionConfiguration
              plugins:
              - name: PodSecurity
                configuration:
                  apiVersion: pod-security.admission.config.k8s.io/v1{{ if semverCompare "< v1.25" .builtin.controlPlane.version }}beta1{{ end }}
                  kind: PodSecurityConfiguration
                  defaults:
                    enforce: "{{ .podSecurity.enforce }}"
                    enforce-version: "latest"
                    audit: "{{ .podSecurity.audit }}"
                    audit-version: "latest"
                    warn: "{{ .podSecurity.warn }}"
                    warn-version: "latest"
                  exemptions:
                    usernames: []
                    runtimeClasses: []
                    namespaces: [kube-system]
            path: /etc/kubernetes/kube-apiserver-admission-pss.yaml
    enabledIf: "{{ .podSecurityStandard.enabled }}"
...
```

{{#/tab }}
{{#tab Create}}


Use this patches if the following keys **do not** exist inside the `KubeadmControlPlaneTemplate` referred by the ClusterClass:

- `.spec.template.spec.kubeadmConfigSpec.clusterConfiguration.apiServer.extraVolumes`
- `.spec.template.spec.kubeadmConfigSpec.files`

> **Attention:** Existing values inside the `KubeadmControlPlaneTemplate` at the mentioned keys will be replaced by this patch.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: ClusterClass
spec:
  ...
  patches:
  - name: podSecurityStandard
    description: "Adds an admission configuration for PodSecurity to the kube-apiserver."
    definitions:
    - selector:
        apiVersion: controlplane.cluster.x-k8s.io/v1beta1
        kind: KubeadmControlPlaneTemplate
        matchResources:
          controlPlane: true
      jsonPatches:
      - op: add
        path: "/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/apiServer/extraArgs"
        value:
          admission-control-config-file: "/etc/kubernetes/kube-apiserver-admission-pss.yaml"
      - op: add
        path: "/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/apiServer/extraVolumes"
        value:
        - name: admission-pss
          hostPath: /etc/kubernetes/kube-apiserver-admission-pss.yaml
          mountPath: /etc/kubernetes/kube-apiserver-admission-pss.yaml
          readOnly: true
          pathType: "File"
      - op: add
        path: "/spec/template/spec/kubeadmConfigSpec/files"
        valueFrom:
          template: |
            - content: |
                apiVersion: apiserver.config.k8s.io/v1
                kind: AdmissionConfiguration
                plugins:
                - name: PodSecurity
                  configuration:
                    apiVersion: pod-security.admission.config.k8s.io/v1{{ if semverCompare "< v1.25" .builtin.controlPlane.version }}beta1{{ end }}
                    kind: PodSecurityConfiguration
                    defaults:
                      enforce: "{{ .podSecurity.enforce }}"
                      enforce-version: "latest"
                      audit: "{{ .podSecurity.audit }}"
                      audit-version: "latest"
                      warn: "{{ .podSecurity.warn }}"
                      warn-version: "latest"
                    exemptions:
                      usernames: []
                      runtimeClasses: []
                      namespaces: [kube-system]
              path: /etc/kubernetes/kube-apiserver-admission-pss.yaml
    enabledIf: "{{ .podSecurityStandard.enabled }}"
...
```

{{#/tab }}
{{#/tabs }}


[Pod Security Standards]: https://kubernetes.io/docs/concepts/security/pod-security-standards

### Create a secure Cluster using the ClusterClass

After adding the variables and patches the Pod Security Standards would be applied by default.
It is also possible to disable this patch or configure different levels for the configuration 
using variables.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: "my-cluster"
spec:
  ...
  topology:
    ...
    class: my-secure-cluster-class
    variables:
    - name: podSecurityStandard
      value: 
        enabled: true
        enforce: "restricted"
```
