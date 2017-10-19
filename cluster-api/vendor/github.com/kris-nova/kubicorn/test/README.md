# Kubicorn e2e tests

The e2e tests are designed to introduce confidence that `kubicorn` is working as expected. 
We use the infrastructure testing suite [charlie](https://github.com/kris-nova/charlie) to test our infrastructure.

At this time e2e tests are not ran automatically for CI, but rather ran ad-hoc until we can harden our process. 
All cloud credentials will need to be valid, and set in order to run the e2e tests. 
You can find out more about which environmental variables need to be set in the [environmental variables cheat sheet](../docs/envar.md).

### Running the e2e tests

```bash
$ make test
```

### e2e vs ci tests

By design CI tests require no input from the user, and should just run out of the box. If you write a CI test that requires an account setup, it should really be an e2e test and live in this directory.

CI tests are regular go tests, that are scattered throughout the rest of the repository.
This directory is special and does not get tested while running `make ci`.