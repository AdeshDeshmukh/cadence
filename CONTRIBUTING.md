# Contributing to Cadence

Thank you for your interest in contributing to Cadence! This document explains how to get started.

## Code of Conduct

Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md).

## Ways to Contribute

- **Bug reports** — open a [bug report issue](https://github.com/AdeshDeshmukh/cadence/issues/new?template=bug_report.yml)
- **Feature requests** — open a [feature request issue](https://github.com/AdeshDeshmukh/cadence/issues/new?template=feature_request.yml)
- **Code contributions** — fork, branch, PR (see below)
- **Documentation** — improve docs, fix typos, add examples
- **Testing** — add test fixtures, improve coverage
- **Translations** — add filler words or ITN rules for new languages

## Good First Issues

New to the codebase? Start here:

- Add filler words for a language in `internal/postprocess/filler.go`
- Add an ITN rule (number/date/currency) for a locale
- Add a vocabulary test fixture
- Add a CLI usage example
- Add a model preset to the default config
- Improve an error message
- Add post-processing unit tests

Issues tagged [`good-first-issue`](https://github.com/AdeshDeshmukh/cadence/labels/good%20first%20issue) are specifically chosen for newcomers.

## Development Setup

### Prerequisites

- Go 1.23+
- Python 3.12+
- Git

### Setup

```bash
git clone https://github.com/AdeshDeshmukh/cadence
cd cadence

# Install Go dependencies
go mod download

# Install Python dependencies
python -m pip install .

# Verify code formatting and linting
make fmt
make vet
```

### Build & Test

```bash
make build          # Build binaries
make test           # Run unit tests
make test-race      # Run tests with race detection
make fmt            # Format Go code
make vet            # Vet Go code
```

## Branching Strategy

```
main          → always releasable, protected
dev           → integration branch
feat/*        → new features
fix/*         → bug fixes
chore/*       → maintenance
docs/*        → documentation only
```

**Always branch from `dev`, not `main`.**

## Commit Convention

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(asr): add streaming transcription support
fix(paste): handle permission denied on macOS Sequoia
refactor(pipeline): extract session fencing into separate type
docs(cli): add cadence transcribe examples
chore(deps): update faster-whisper to 1.1.0
test(vad): add regression fixture for silence detection
perf(asr): reduce worker spawn time by 40%
```

## Pull Request Requirements

Every PR must:

1. Pass all CI checks (`go vet`, `golangci-lint`, `go test ./...`, `pytest ml/`)
2. Include tests for new behaviour
3. Update `CHANGELOG.md` under `[Unreleased]`
4. Not introduce new `//nolint` directives without explanation
5. Not break existing CLI interface without a deprecation notice
6. Include a clear description of what changed and why

## PR Template

When you open a PR, the template will guide you through what to include.

## Code Style

- **Go**: `gofmt` formatted, `golangci-lint` clean
- **Python**: `ruff` formatted, type hints on all public functions
- **TypeScript**: `prettier` formatted, `eslint` clean

## Testing

- All new behaviour must have tests
- Use table-driven tests in Go
- Use `pytest` fixtures in Python
- Audio fixtures go in `tests/e2e/fixtures/`

## Security

If you discover a security vulnerability, please follow the process in [SECURITY.md](SECURITY.md). **Do not open a public issue for security vulnerabilities.**

## Questions

Open a [Discussion](https://github.com/AdeshDeshmukh/cadence/discussions) for questions about the codebase or architecture.
