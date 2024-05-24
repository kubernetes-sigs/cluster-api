# Verification of CAPI artifacts

## Requirements 
You will need to have the following tools installed:
- cosign [(install guide)](https://docs.sigstore.dev/system_config/installation/)
- jq [(download jq)](https://stedolan.github.io/jq/download/)

## CAPI Images
Each [release](https://github.com/kubernetes-sigs/cluster-api/releases) of the Cluster API project includes the following container images:
- cluster-api-controller
- kubeadm-bootstrap-controller
- kubeadm-control-plane-controller
- clusterctl

## Verifying Image Signatures
All of the four images are hosted by [registry.k8s.io](https://registry.k8s.io). In order to verify the authenticity of the images, you can use `cosign verify` command with the appropriate image name and version: 

```bash
$ cosign verify registry.k8s.io/cluster-api/cluster-api-controller:v1.5.0 --certificate-identity krel-trust@k8s-releng-prod.iam.gserviceaccount.com --certificate-oidc-issuer https://accounts.google.com | jq .
```
```text
Verification for registry.k8s.io/cluster-api/cluster-api-controller:v1.5.0 --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - The code-signing certificate was verified using trusted certificate authority certificates
[
  {
    "critical": {
      "identity": {
        "docker-reference": "registry.k8s.io/cluster-api/cluster-api-controller"
      },
      "image": {
        "docker-manifest-digest": "sha256:f34016d3a494f9544a16137c9bba49d8756c574a0a1baf96257903409ef82f77"
      },
      "type": "cosign container image signature"
    },
    "optional": {
      "1.3.6.1.4.1.57264.1.1": "https://accounts.google.com",
      "Bundle": {
        "SignedEntryTimestamp": "MEYCIQDtxr/v3uRl2QByVfYo1oopruADSaH3E4wThpmkibJs8gIhAIe0odbk99na5GBdYGjJ6IwpFzhlTlicgWOrsgxZH8LC",
        "Payload": {
          "body": "eyJhcGlWZXJzaW9uIjoiMC4wLjEiLCJraW5kIjoiaGFzaGVkcmVrb3JkIiwic3BlYyI6eyJkYXRhIjp7Imhhc2giOnsiYWxnb3JpdGhtIjoic2hhMjU2IiwidmFsdWUiOiIzMDMzNzY0MTQwZmI2OTE5ZjRmNDg2MDgwMDZjYzY1ODU2M2RkNjE0NWExMzVhMzE5MmQyYTAzNjE1OTRjMTRlIn19LCJzaWduYXR1cmUiOnsiY29udGVudCI6Ik1FUUNJQ3RtcGdHN3RDcXNDYlk0VlpXNyt6Rm5tYWYzdjV4OTEwcWxlWGppdTFvbkFpQS9JUUVSSDErdit1a0hrTURSVnZnN1hPdXdqTTN4REFOdEZyS3NUMHFzaUE9PSIsInB1YmxpY0tleSI6eyJjb250ZW50IjoiTFMwdExTMUNSVWRKVGlCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2sxSlNVTTJha05EUVc1SFowRjNTVUpCWjBsVldqYzNUbGRSV1VacmQwNTVRMk13Y25GWWJIcHlXa3RyYURjMGQwTm5XVWxMYjFwSmVtb3dSVUYzVFhjS1RucEZWazFDVFVkQk1WVkZRMmhOVFdNeWJHNWpNMUoyWTIxVmRWcEhWakpOVWpSM1NFRlpSRlpSVVVSRmVGWjZZVmRrZW1SSE9YbGFVekZ3WW01U2JBcGpiVEZzV2tkc2FHUkhWWGRJYUdOT1RXcE5kMDU2U1RGTlZHTjNUa1JOTlZkb1kwNU5hazEzVG5wSk1VMVVZM2hPUkUwMVYycEJRVTFHYTNkRmQxbElDa3R2V2tsNmFqQkRRVkZaU1V0dldrbDZhakJFUVZGalJGRm5RVVZ4VEdveFJsSmhLM2RZTUVNd0sxYzFTVlZWUW14UmRsWkNWM2xLWTFRcmFWaERjV01LWTA4d1prVmpNV2s0TVUxSFQwRk1lVXB2UXpGNk5TdHVaRGxFUnpaSGNFSmpOV0ZJYXpoU1QxaDBOV2h6U21wa1VVdFBRMEZhUVhkblowZE5UVUUwUndwQk1WVmtSSGRGUWk5M1VVVkJkMGxJWjBSQlZFSm5UbFpJVTFWRlJFUkJTMEpuWjNKQ1owVkdRbEZqUkVGNlFXUkNaMDVXU0ZFMFJVWm5VVlYxTVRoMENqWjVWMWxNVlU5RVR5dEVjek52VVU1RFNsYzNZMUJWZDBoM1dVUldVakJxUWtKbmQwWnZRVlV6T1ZCd2VqRlphMFZhWWpWeFRtcHdTMFpYYVhocE5Ga0tXa1E0ZDFGQldVUldVakJTUVZGSUwwSkVXWGRPU1VWNVlUTktiR0pETVRCamJsWjZaRVZDY2s5SVRYUmpiVlp6V2xjMWJreFlRbmxpTWxGMVlWZEdkQXBNYldSNldsaEtNbUZYVG14WlYwNXFZak5XZFdSRE5XcGlNakIzUzFGWlMwdDNXVUpDUVVkRWRucEJRa0ZSVVdKaFNGSXdZMGhOTmt4NU9XaFpNazUyQ21SWE5UQmplVFZ1WWpJNWJtSkhWWFZaTWpsMFRVTnpSME5wYzBkQlVWRkNaemM0ZDBGUlowVklVWGRpWVVoU01HTklUVFpNZVRsb1dUSk9kbVJYTlRBS1kzazFibUl5T1c1aVIxVjFXVEk1ZEUxSlIwdENaMjl5UW1kRlJVRmtXalZCWjFGRFFraDNSV1ZuUWpSQlNGbEJNMVF3ZDJGellraEZWRXBxUjFJMFl3cHRWMk16UVhGS1MxaHlhbVZRU3pNdmFEUndlV2RET0hBM2J6UkJRVUZIU21wblMxQmlkMEZCUWtGTlFWSjZRa1pCYVVKSmJXeGxTWEFyTm05WlpVWm9DbWRFTTI1Uk5sazBSV2g2U25SVmMxRTRSSEJrWTFGeU5FSk1XRE41ZDBsb1FVdFhkV05tYmxCUk9GaExPWGRZYkVwcVNWQTBZMFpFT0c1blpIazRkV29LYldreGN6RkRTamczTW1zclRVRnZSME5EY1VkVFRUUTVRa0ZOUkVFeVkwRk5SMUZEVFVoaU9YRjBSbGQxT1VGUU1FSXpaR3RKVkVZNGVrazRZVEkxVUFwb2IwbFBVVlJLVWxKeGFsVmlUMkUyVnpOMlRVZEJOWFpKTlZkVVJqQkZjREZwTWtGT2QwbDNSVko0TW5ocWVtWjNjbmRPYmxoUVpEQjRjbmd3WWxoRENtUmpOV0Z4WWxsWlVsRXdMMWhSVVdONFRFVnRkVGwzUnpGRlYydFNNWE01VEdaUGVHZDNVMjRLTFMwdExTMUZUa1FnUTBWU1ZFbEdTVU5CVkVVdExTMHRMUW89In19fX0=",
          "integratedTime": 1690304684,
          "logIndex": 28719030,
          "logID": "c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d"
        }
      },
      "Issuer": "https://accounts.google.com",
      "Subject": "krel-trust@k8s-releng-prod.iam.gserviceaccount.com",
      "org.kubernetes.kpromo.version": "kpromo-v4.0.3-5-ge99897c"
    }
  }
]
```
