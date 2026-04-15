# Contributing

This project uses [uv](https://docs.astral.sh/uv/) for dependency management.

## Setup

```sh
uv sync
```

## Quality checks

```sh
uv run ruff check .          # lint
uv run ruff format --check . # format check
uv run pyright               # type check
uv run pytest --cov=thermocktat_client  # tests with coverage
```

## Pre-commit hooks

We use [prek](https://github.com/j178/prek) (a Rust-based drop-in replacement for `pre-commit`) to run lint, format and type-check on commit.

Install the git hook once:

```sh
prek install
```

Run the hooks manually against all files:

```sh
prek run --all-files
```

Hooks are defined in `.pre-commit-config.yaml`.

## Releasing

Releases are published to PyPI by `.github/workflows/release-python-client.yaml` when a `python-client/vX.Y.Z` tag is pushed. The workflow promotes the wheel built by CI on the tagged commit (build-once, promote pattern) and verifies that the wheel version matches the tag — so `pyproject.toml` and the tag must agree.

Versioning follows [semver](https://semver.org/). While in `0.x`, MINOR bumps may include breaking changes.

### Steps

1. Open a PR bumping `version` in `clients/python/pyproject.toml` to `X.Y.Z`.
2. Merge to `main`. CI runs and uploads the `thermocktat-client-dist` artifact for the merge commit.
3. Tag the merge commit and push:

   ```sh
   git checkout main && git pull
   git tag python-client/vX.Y.Z
   git push origin python-client/vX.Y.Z
   ```

4. The release workflow runs automatically: finds the CI artifact, verifies version match, publishes to PyPI, creates a GitHub Release.

If the version-match step fails, you tagged a commit whose `pyproject.toml` doesn't match the tag — delete the tag (`git push --delete origin python-client/vX.Y.Z`), fix the version, and retry.
