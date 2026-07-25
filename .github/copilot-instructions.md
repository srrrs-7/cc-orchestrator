# GitHub Copilot instructions — cc-orchestrator

Monorepo where each `app/<stack>` is developed with its own toolchain and rules.
Follow the conventions below when suggesting or editing code.

## Repository layout

| Path | Stack | Tooling |
|---|---|---|
| `app/web` | Frontend (TypeScript / React) | Bun, Biome, tsc, Vitest, Vite, TanStack Router/Query |
| `app/api` | Backend API (Go, DDD) | Go 1.26 (stdlib + pgx/v5 for Postgres only), Make, goose, sqlc |
| `app/auth` | Auth API (Go, OAuth 2.0 + OIDC) | Go 1.26 (stdlib + pgx/v5 for Postgres, golang.org/x/crypto for bcrypt), Make, goose, sqlc |
| `app/iac` | Infrastructure (Terraform / AWS) | Terraform `>= 1.10`, tflint, trivy |
| `docs/` | Specs / issues / plans | Markdown |

Do not change code outside the stack you are working in.

## Commands (per stack — do not invent others)

- **web** (`app/web`): `make install`, `make format-check` / `format` / `lint` / `typecheck` / `test` / `build` / `generate` / `check`.
- **api / auth** (`app/api`, `app/auth`): `make check` (= `fmt-check` + `lint` + `vet` + `build` + `test`); individual targets `make fmt` / `lint` / `vet` / `build` / `test` / `test-race`. `make test` (and thus `make check`) is DB-backed by default (SPEC-013 — the old `integration` build tag is gone): it runs against the real `api_test` / `auth_test` Postgres databases and requires `REQUIRE_DB=1`; provision them first via repo-root `make migrate-test` (or run the whole flow with repo-root `make test-db`). Postgres codegen/schema tooling (SPEC-005, not part of `make check`): `make sqlc` (generate `infra/postgres/sqlcgen` from `infra/postgres/schema/queries`; commit the output), `make migrate-create name=<slug>` (goose, `infra/postgres/schema/migrations`).
- **iac** (`app/iac`): `make check ENV=dev` (= `fmt-check` + `validate` + `lint` + `security`). Never run `terraform apply`.

## Git hooks (pre-commit)

Optional local gate before commit (`.githooks/`). Run once per clone: `make setup-hooks`. Manual run: `make hook-check`. Skip: `git commit --no-verify`.

Runs CI-equivalent checks for **staged files only** (same path filters as `cicd.yml` / `contract-drift.yml` / `sqlc-drift.yml`): per-stack `make check` plus contract/sqlc drift when relevant. A staged `app/api`/`app/auth` change provisions Postgres and both test databases itself (repo-root `make db-up` + `make migrate-test`) before running that stack's `make check` with `REQUIRE_DB=1` (SPEC-013) — that's no longer a separate lane. Only `app/migrator`'s own separate `migrator-integration` job (CI) stays out of scope here.

Execution: on the **host**, the hook re-enters the SPEC-009 toolchain container in three phases — `tools` (network enabled, warm-up: `go mod download` / `bun install` / iac `validate`'s provider fetch), `tools-offline` (`--network none`: fmt-check/lint/vet/build), `tools-db` (Postgres-reachable, no internet egress: the DB-backed `go test` run) — so static analysis and DB test execution stay as network-isolated as CI's own jobs (SPEC-013 R6 / ISSUE-029). Inside **devcontainer** (`IN_TOOLBOX=1`), checks run directly without nested Docker.

## Go (`app/api`, `app/auth`)

- DDD layered architecture with a one-way dependency `route → service → domain`. `domain` depends on nothing and is framework-free and unit-testable.
- Standard library only, with narrow deliberate exceptions: `infra/postgres` depends on `github.com/jackc/pgx/v5` as a `database/sql` driver for the Postgres-backed `Repository` implementations (SPEC-005) in both stacks, and `app/auth` additionally depends on `golang.org/x/crypto` (bcrypt password hashing). `domain` / `service` / `route` remain stdlib-only. Migration (goose) and codegen (sqlc) tools run via `go run pkg@pinned` (see each stack's `Makefile`) and never become `go.mod` dependencies. `cmd/<binary>/main.go` is the composition root (wiring only, no logic) — always wires Postgres (fail-closed; SPEC-011 removed the memory/Postgres persistence-selection fallback — unrelated to `app/auth`'s own `infra/memory`, which is an intentionally in-memory IdP session store, not a persistence layer, and still exists).
- Package by domain (split packages per aggregate/domain, not per technical layer). Both `app/api` and `app/auth` use top-level layer directories (`domain` / `service` / `infra` / `route` / `cmd`) and do not use an `internal/` tree.
- `context.Context` is the first argument, never stored on a struct.
- Wrap errors with `fmt.Errorf("...: %w", err)`. For branchable errors define sentinel (`var ErrX = errors.New(...)`) or custom types and match with `errors.Is` / `errors.As`. Never use `panic` for error handling.
- Abstract DB / external calls behind interfaces (defined by the consumer) so tests can substitute fakes. Every goroutine must have a termination condition.
- Tests: standard `go test`, table-driven; cover success / error / boundary cases.
- `app/auth` (auth server): never hardcode keys/secrets. RSA signing keys are a persisted multi-key ring (`.secrets/auth-signing-keys.json`, gitignored; `make auth-signing-keys` / `rotate-signing-keys` — rotation demotes the old active key to verify-only so JWKS keeps an overlap period), falling back to ephemeral generation only if unset; other demo values are seeded at startup, production values injected from the environment. Implements the authorization code grant (PKCE S256) plus refresh tokens (rotation + reuse/family revocation, RFC 6749 §6 / RFC 9700), login/consent UI, RP-Initiated Logout, revoke (RFC 7009), introspect (RFC 7662), and an API-key-gated (fail-closed if unset) admin API. Enforce RS256-signed JWTs with `kid` + JWKS (reject `alg: none`) and full `iss` / `aud` / `exp` / signature validation.

## TypeScript / React (`app/web`)

- TypeScript strict mode. **`any` is forbidden** — use `unknown` + type guards. `as` casts only at boundaries (e.g. parsing API responses).
- Validate all external data (API / localStorage / URL params) with a schema (zod) before typing it.
- **Named exports only** (no `default` export unless a routing convention requires it).
- Function components + hooks only (no class components). Separate server state (API data via TanStack Query) from client/UI state; do not copy server state into `useState`.
- One-way layering `components → hooks → (api | domain)`. Business rules (state transitions, invariants, derived values, filter/sort) live in `features/<feature>/domain/` as pure, React/DOM/fetch-free functions; components and hooks call them and hold no logic. Isolate side effects in hooks.
- Do not use array index as a list `key` (except static, never-reordered lists).
- Lint/format with **Biome** (not ESLint/Prettier); type-check with **tsc** (TypeScript 7 native). Runtime and package manager is **Bun**.
- Supply-chain: `bunfig.toml` sets `minimumReleaseAge` (21 days) to avoid freshly published (potentially compromised) versions. Pin new dependencies to a known-good version.
- Tests: Vitest + React Testing Library; query by role/label (user-facing), not implementation details.

## Terraform (`app/iac`)

- Resource / variable names in `snake_case`. Every `variable` needs a `type` and `description`.
- Pin `required_version`, `required_providers`, and module `version`. Prefer `for_each` over `count`.
- Remote state backend (S3) with locking. Never write secrets or account IDs in `.tf` / `.tfvars` — reference Secrets Manager / SSM. Tag resources with `ManagedBy = "terraform"` and the environment.
- Reusable code goes in `modules/<module>/`; environment roots in `envs/<env>/` (express differences via variables/tfvars, do not copy resource definitions per env).
- **Never run `terraform apply`** — produce a `plan` and leave the apply decision to a human.

## General conventions

- **Language**: write docs, specs, and issues under `docs/` in Japanese. Write code identifiers, comments, and commit messages in English.
- Specs in `docs/specs` are the source of truth; if implementation and spec conflict, the spec wins — flag the discrepancy rather than silently reinterpreting.
- Never commit secrets (API keys, credentials, account IDs) to code, docs, or tfvars.
