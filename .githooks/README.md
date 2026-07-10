# Git hooks

Pre-commit hooks run the same stack checks as CI (`.github/workflows/cicd.yml`, `contract-drift.yml`, `sqlc-drift.yml`) for **staged files only**. Integration tests (`*-integration` jobs) are not run — they require Postgres and are slower than a commit hook should be.

All checks run **inside the pinned toolchain container** (SPEC-009), same as CI:

- **Host**: the hook re-execs itself via `docker compose run tools` (Docker required).
- **Devcontainer / `IN_TOOLBOX=1`**: runs directly in the current session (no Docker-in-Docker).

## Setup

From the repository root:

```bash
make setup-hooks
```

This sets `core.hooksPath` to `.githooks` for this clone.

## Manual run

Run the same logic without committing:

```bash
make hook-check
```

Inside a devcontainer session, `make hook-check` and `git commit` use the same in-container path.

## Skip

To bypass the hook for one commit:

```bash
git commit --no-verify
```

Or:

```bash
SKIP_PRE_COMMIT=1 git commit
```

## Requirements

- **Host**: Docker (to enter the toolchain container)
- **Devcontainer**: no extra setup; `IN_TOOLBOX=1` is set by `compose.tools.yml`
- Staged changes determine which stacks are checked; docs-only or `.claude/`-only commits skip checks
