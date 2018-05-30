# Contributing Guidelines

1. Follow the [Getting Started]((https://github.com/kubernetes-sigs/cluster-api/blob/master/cluster-api/clusterctl/README.md)) steps to create a cluster.

# Development

Before submitting an PR you should run the unit and integration tests. Instructions for doing so are given in the [Testing](#Testing) section.

## Testing

### Unit Tests
When changing this application, you will often end up modifying other packages above this folder in the project tree. You
should run all the unit tests in the repository. To run the unit tests, run the following command from the root,
`cluster-api`, folder of the repo.

```
go test ./...
```

### Integration Tests

To run the integration tests, run the following command from this folder. The integration tests are for sanity checking
that clusterctl's basic functionality is working.
```
go test -tags=integration -v
```