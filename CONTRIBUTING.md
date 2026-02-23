# Contributing to OpenConductor

Thanks for your interest in contributing. This document covers the basics.

## Development setup

1. **Clone the repo:**
   ```bash
   git clone https://github.com/openconductorhq/openconductor.git
   cd openconductor
   ```

2. **Requirements:**
   - Go 1.24+
   - [golangci-lint](https://golangci-lint.run/welcome/install/) (for `make lint`)
   - At least one AI coding agent for end-to-end testing

3. **Build and test:**
   ```bash
   make build    # compile
   make test     # run tests with race detector
   make lint     # run linter
   make check    # all of the above
   ```

## Making changes

1. Fork the repo and create a branch from `master`.
2. Make your changes. Add or update tests for any new behavior.
3. Ensure all source files have the SPDX license header:
   ```go
   // SPDX-License-Identifier: MIT
   // Copyright (c) 2025 The OpenConductor Authors.
   ```
4. Run `make check` to verify everything passes.
5. Open a pull request against `master`.

## Commit messages

Use short, descriptive commit messages. Prefix with a type when it makes sense:

- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation only
- `test:` test additions or fixes
- `refactor:` code changes that don't fix bugs or add features
- `ci:` CI/CD changes
- `deps:` dependency updates

## Code style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Keep packages focused. New functionality usually belongs in a new package
  under `internal/`.
- Prefer table-driven tests.
- Avoid global state; pass dependencies explicitly.

## Reporting bugs

Use the [bug report template](https://github.com/openconductorhq/openconductor/issues/new?template=bug_report.yml).
Include logs from `~/.openconductor/openconductor.log` (run with `--debug`).

## Feature requests

Use the [feature request template](https://github.com/openconductorhq/openconductor/issues/new?template=feature_request.yml).
Describe the problem before proposing a solution.

## License

By contributing, you agree that your contributions will be licensed under the
[MIT License](LICENSE).
