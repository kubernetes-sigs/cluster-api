# Rerunning your process via `live_update`
## i.e., simulating [`restart_container()`](https://docs.tilt.dev/live_update_reference.html#restart_container) on non-Docker clusters

As of 6/28/19: `restart_container()`, a command that can be passed to a `live_update`, doesn't work on non-Docker clusters. However there's a workaround available to simulate `restart_container()`'s functionality. It's used in [the onewatch integration test](https://github.com/tilt-dev/tilt/tree/master/integration/onewatch) so that the test passes on non-Docker clusters. Here's how to do it yourself:

Copy `start.sh` and `restart.sh` to your container working dir.

If your container entrypoint *was* `path-to-binary [arg1] [arg2]...`, change it to:
```
./start.sh path-to-binary [arg1] [arg2]...
```

To restart the container, instead of including Live Update step `restart_container()`, use:
`run('./restart.sh')`

So, for example:

```python
docker_build('gcr.io/windmill-test-containers/integration/onewatch',
    '.',
    dockerfile='Dockerfile',
    live_update=[
        sync('.', '/go/src/github.com/windmilleng/tilt/integration/onewatch'),
        run('go install github.com/windmilleng/tilt/integration/onewatch'),
        run('./restart.sh'),
    ])
```

This live update will cause the `go install` to be run in the container every time anything in the `.` path locally changes. After the `go install` is run, `./restart.sh` will be run. This will kill the original entrypoint, and restart it, effectively simulating the `container_restart()` functionality on Docker.
