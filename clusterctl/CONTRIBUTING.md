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
