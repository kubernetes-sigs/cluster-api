# Troubleshooting

## docker-provider-controller-manager-0 Pod stuck in ContainerCreating phase

1. Check for any warnings or errors:

```
$ kubectl -n docker-provider-system describe pod docker-provider-controller-manager-0
```

Are you running CAPD controllers in a kind cluster not created by `capdctl`? Do you see `MountVolume.SetUp` warnings like these?
```
Name:               docker-provider-controller-manager-0
Namespace:          docker-provider-system
...
Events:
  Type     Reason       Age                From                                   Message
  ----     ------       ----               ----                                   -------
  Normal   Scheduled    52s                default-scheduler                      Successfully assigned docker-provider-system/docker-provider-controller-manager-0 to capi-bootstrap-control-plane
  Warning  FailedMount  21s (x7 over 52s)  kubelet, capi-bootstrap-control-plane  MountVolume.SetUp failed for volume "dockerlib" : hostPath type check failed: /var/lib/docker is not a directory
  Warning  FailedMount  21s (x7 over 52s)  kubelet, capi-bootstrap-control-plane  MountVolume.SetUp failed for volume "dockersock" : hostPath type check failed: /var/run/docker.sock is not a socket file
```

If yes, then follow [these instructions](kind.md) to create a kind cluster that supports CAPD controllers.

