# Git hooks

Pre-commit hooks run the same stack checks as CI (`.github/workflows/cicd.yml`, `contract-drift.yml`, `sqlc-drift.yml`) for **staged files only**.

**SPEC-013 (テストの実 DB 一本化)**: a staged `app/api`/`app/auth` change now runs that stack's tests against a real Postgres *test* database (`api_test`/`auth_test`, never the dev `api`/`auth` databases) — the same DB-backed tests CI's `api`/`auth` jobs run, no longer a separate offline-only lane. The hook provisions Postgres and both test databases itself (root `make db-up` + `make migrate-test`) before running any stack's check. Only `app/migrator`'s own separate Postgres-permission-boundary suite (CI's `migrator-integration` job, ISSUE-016 R-c) is not run here — that is a distinct, slower concern unrelated to api/auth's own persistence-layer tests.

All checks run **inside the pinned toolchain container** (SPEC-009), same as CI. On the host, the hook re-execs itself via `docker compose run` in **two separate phases** (SPEC-013 R6 — see `.githooks/lib/common.sh`'s `githooks_reexec_phase` for the full rationale), so that DB-backed test *execution* never shares a container with internet access:

- **`offline` phase** (service `tools`, network enabled): dependency warming (`go mod download` / `bun install`), web's own check, iac's check (including `validate`'s provider fetch), migrator's check, contract/sqlc drift, and api/auth's own **offline-only** `fmt-check`/`lint`/`vet`/`build` — none of this executes dependency/test code against the internet in the sense the next phase is scoped to guard against.
- **`db-test` phase** (service `tools-db`, Postgres-reachable, **no internet route at all**): only api/auth's own `go test` run — the phase that actually executes arbitrary dependency/test code, now with no network path to reach out over even if it tried.

- **Host**: the hook re-execs itself via `docker compose run` into each phase above in turn (Docker required).
- **Devcontainer / `IN_TOOLBOX=1`**: runs both phases directly, natively, in the current session (no Docker-in-Docker — that session has no Docker socket of its own to start either phase container with, so it cannot enforce the same network split; see Requirements below).

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

- **Host**: Docker (to enter the toolchain container, and — for a staged `app/api`/`app/auth` change — to run root `make db-up`/`make migrate-test` first)
- **Devcontainer**: no extra setup; `IN_TOOLBOX=1` is set by `compose.tools.yml`. A staged `app/api`/`app/auth` change still needs Postgres reachable at `DB_HOST=postgres` — since a devcontainer session has no Docker socket of its own (SPEC-009), start it from a separate host terminal first (`make db-up && make migrate-test`)
- Staged changes determine which stacks are checked; docs-only or `.claude/`-only commits skip checks
