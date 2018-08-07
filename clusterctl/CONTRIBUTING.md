# Contributing Guidelines

Before submitting a PR you should run the unit and integration tests.

## Building

To build the go code, run

```shell
./scripts/ci-build.sh
```

To verify that the code still builds into docker images, run

```shell
./scripts/ci-make.sh
```

## Testing

When changing this application, you will often end up modifying other packages above this folder in the project tree.
You should run all the tests in the repository. To run the tests, run the following command from the root,
`cluster-api`, folder of the repo.

```shell
./scripts/ci-test.sh
```

To get all the tests to pass, it is required that you have `etcd` installed in your development environment. Specifically,
the `TestMachineSet` tests will fail if you don't have `etcd` installed and the failure will look like this

``` 
=== RUN   TestMachineSet
[::]:58989
[::]:58990
[::]:58991
panic: exec: "etcd": executable file not found in $PATH

goroutine 131 [running]:
sigs.k8s.io/cluster-api/vendor/github.com/kubernetes-incubator/apiserver-builder/pkg/test.(*TestEnvironment).startEtcd(0xc420704000, 0xc4206c8de0)
.
.
.
FAIL	sigs.k8s.io/cluster-api/pkg/controller/machineset	0.077s
```