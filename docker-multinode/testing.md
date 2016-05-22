## Testing your Kubernetes cluster.

To validate that your node(s) have been added, run:

```console
$ kubectl get nodes
```

That should show something like:

```console
NAME        LABELS                             STATUS    AGE
127.0.0.1   kubernetes.io/hostname=127.0.0.1   Ready     2h
```

If the status of any node is `Unknown` or `NotReady` your cluster is broken, double check that all containers are running properly, and if all else fails, contact us on [Slack](../../troubleshooting.md#slack).
You may also check if for `docker` [troubleshooting](../docker.md#troubleshooting)

### Run an application

```console
$ kubectl run nginx --image=nginx --port=80
```

Now run `docker ps` you should see nginx running.  You may need to wait a few minutes for the image to get pulled.

### Expose it as a service

```console
$ kubectl expose rc nginx --port=80
```

Run the following command to obtain the IP of this service we just created. There are two IPs, the first one is internal (CLUSTER_IP), and the second one is the external load-balanced IP (if a LoadBalancer is configured)

```console
$ kubectl get svc nginx
```

Alternatively, you can obtain only the first IP (CLUSTER_IP) by running:

```console
$ kubectl get svc nginx --template={{.spec.clusterIP}}
```

Hit the webserver with the first IP (CLUSTER_IP):

```console
$ curl <insert-cluster-ip-here>
```

Note that you will need run this curl command on your boot2docker VM if you are running on OS X.

### Scaling

Now try to scale up the nginx you created before:

```console
$ kubectl scale rc nginx --replicas=3
```

And list the pods

```console
$ kubectl get pods
```

You should see pods landing on the newly added machine.
