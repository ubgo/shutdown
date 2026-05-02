# Contributing to ubgo/shutdown

Thanks for your interest in `ubgo/shutdown`. This repository is licensed under the **Apache License 2.0**. Pull requests are welcome.

## Workflow

1. Open an issue first for anything beyond a tiny fix. Discussing the design upfront avoids wasted work.
2. Fork + branch named after the issue: `fix/123-watchdog-race`, `feat/456-actor-grouping`.
3. Run local checks: `task ci`.
4. Use Conventional Commits for the PR title.

## Code conventions

- **Zero third-party deps in the core.** The `zero-dep-gate` CI check fails if `go.mod` of the core module gains any non-stdlib `require` line. Adapter modules under `contrib/` are free to depend on their target ecosystems.
- **Race detector mandatory.** Every test must pass under `-race`.
- **Coverage target.** ≥ 90% line coverage for the core. Adapters: ≥ 80%.
- **Public API stability.** Once the module reaches v1.0.0, breaking changes require a major version bump and a strong rationale.
- **No comments explaining what the code does.** Names should make that clear. Reserve comments for the *why* — non-obvious invariants, hidden constraints, surprising tradeoffs.

## Testing locally

```sh
task test           # standard tests
task test:race      # with race detector
task test:coverage  # with coverage report
task lint           # golangci-lint
task ci             # everything
```

## License of contributions

By submitting a pull request, you agree that your contribution is provided under the same Apache License 2.0 as the rest of the repository.
