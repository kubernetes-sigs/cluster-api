# AGENTS.md

Guidance for AI coding agents working in the Cluster API repository.

## Communication

- Be concise and direct. No flattery, no preambles.

## Constraints

- Never hand-edit generated files (`zz_generated.*`, `config/crd/bases`, `config/rbac`, `config/webhook`).
  Change the Go types or kubebuilder markers and run `make generate`.
- There are five Go modules: root, `api/`, `utils/`, `test/`, `hack/tools/`. Run `go build`/`go test` from
  the module that owns the package, and use `make generate-modules` instead of running `go mod tidy` by hand.
- Core controllers never import provider types (KCP, CABPK, CAPD). They go through `internal/contract` and
  `controllers/external`. Core tests use the generic providers in `util/test/builder`, not a concrete provider.
- API types live in `api/`. v1beta2 is the hub/storage version. Every field added to v1beta2 is added identically
  to v1beta1 so that no custom conversion code is needed. Run `make test-conversions` after you change API types.
  Don't add tests for conversion code: the fuzz tests already cover it.
- `util/` and `utils/` are public APIs used by providers, so don't break them without a reason. `exp/` and
  `internal/` have no compatibility guarantees.
- Use the import aliases enforced by `importas` in `.golangci.yml` (`clusterv1`, `clusterv1beta1`, `corev1`,
  `apierrors`, `pkgerrors`, ...).
- New files need the Apache 2.0 header without a copyright year (`make verify-boilerplate`).

## Contributing

- Read [CONTRIBUTING.md](CONTRIBUTING.md#ai-guidance).
- Keep changes focused and reviewable. Don't touch unrelated lines, files or tooling. Mention unrelated issues
  instead of fixing them.
- Add tests that cover new behaviour, not tests that only increase coverage. Extend existing table-driven tests
  instead of adding new ones.
- When creating or submitting a pull request, disclose whether AI was used and briefly describe how.
- Remind the human author that they are responsible for all submitted changes and refer them to `CONTRIBUTING.md`.
- PR titles start with `:warning:` (breaking), `:sparkles:` (feature), `:bug:` (fix), `:book:` (docs) or
  `:seedling:` (other). 
- PR descriptions and commit messages follow the communication guidelines above.
- Don't put `@mentions` or `fixes #...` keywords in commit messages.
- Don't add `Co-authored-by:` lines to commit messages.
- Review comments must be answered by the human author, not by an agent.

## Commands

`make help` lists all targets.

```bash
make lint                # golangci-lint + kube-api-linter
make lint-fix            # same as lint and also fix findings when possible
make test                # unit + envtest tests for the root and api modules  
make test-no-race        # like make test, but without -race and without the separate TestFuzzyConversion run
make test-conversions    # fuzz conversion tests
make generate            # manifests, deepcopy, conversions, openapi, docs
make verify              # everything CI verifies
```

Run a single test:

```bash
# envtest packages need KUBEBUILDER_ASSETS. `make setup-envtest` prints the correct value.
go test ./core/reconcilers/machine/... -run '^TestName$'
# Tests that only use the fake client can skip starting envtest.
CAPI_DISABLE_TEST_ENV=1 go test ./path/... -run '^TestName$'
```

## Developer docs

Before you change code in one of these areas, read at least the `Rules` section at the top of the matching doc in
`docs/book/src/developer/`:

| When you change ...                                                       | Read                                                          |
|---------------------------------------------------------------------------|---------------------------------------------------------------|
| any code in the repository                                                | `core/overview.md`                                            |
| API types (`api/`)                                                        | `core/API.md`                                                 |
| a controller (`<provider>/reconcilers/<kind>/`)                           | `core/controllers/overview.md`, then `core/controllers/<kind>.md` |
| how core talks to providers (`internal/contract`, `controllers/external`) | `providers/contracts/overview.md`, then the page of the resource |
| Runtime SDK hooks (`api/runtime/hooks`, `pkg/runtimesdk`)                 | `runtime-extensions/`                                         |
| log lines                                                                 | `core/logging.md`                                             |
| unit or integration tests                                                 | `core/testing.md`                                             |
| e2e tests (`test/e2e`, `test/framework`)                                  | `core/e2e.md`                                                 |

If your change makes a doc incorrect, update the doc in the same PR.

## Style

- Comments explain *why*, not *what*; use `// Note:` for rationale and non-obvious coupling. Godoc starts with the
  name of the identifier. Comments describe the current state of the code, not the change that led to it.
- Return errors instead of swallowing them or panicking, also for branches that "should never happen".
- Error messages are specific: `failed to <verb> <Kind> <klog.KObj(obj)>`, using the real Kind. Match the wrapping
  style of the surrounding file (`pkgerrors.Wrapf` or `fmt.Errorf("...: %w")`).
- After copying or renaming code, re-read identifiers, comments, godoc and messages, and check they match the new context.
- Name things after the concept, not an implementation detail.
- When removing code, remove it completely: tests, import aliases in `.golangci.yml` and doc references included.
