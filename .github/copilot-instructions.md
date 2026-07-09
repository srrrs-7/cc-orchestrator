# GitHub Copilot instructions — cc-orchestrator

Monorepo where each `app/<stack>` is developed with its own toolchain and rules.
Follow the conventions below when suggesting or editing code.

## Repository layout

| Path | Stack | Tooling |
|---|---|---|
| `app/web` | Frontend (TypeScript / React) | Bun, Biome, tsgo, Vitest, Vite |
| `app/api` | Backend API (Go, DDD) | Go 1.24 (stdlib + pgx/v5 for Postgres only), Make, goose, sqlc |
| `app/auth` | Auth API (Go, OAuth 2.0 + OIDC) | Go 1.24 (stdlib + pgx/v5 for Postgres only), Make, goose, sqlc |
| `app/iac` | Infrastructure (Terraform / AWS) | Terraform `>= 1.10`, tflint, trivy |
| `docs/` | Specs / issues / plans | Markdown |

Do not change code outside the stack you are working in.

## Commands (per stack — do not invent others)

- **web** (`app/web`): `bun install`, `bun run format:check` / `format` / `lint` / `typecheck` / `test` / `build`.
- **api / auth** (`app/api`, `app/auth`): `make check` (= `fmt-check` + `lint` + `vet` + `build` + `test`); individual targets `make fmt` / `lint` / `vet` / `build` / `test` / `test-race`. Postgres persistence (SPEC-005, not part of `make check`): `make sqlc` (generate `infra/postgres/sqlcgen` from `db/queries`; commit the output), `make migrate-up` / `migrate-down` / `migrate-status` / `migrate-create name=<slug>` (goose, `db/migrations`), `make test-integration` (build tag `integration`, requires a migrated Postgres reachable via `DB_*` env vars).
- **iac** (`app/iac`): `make check ENV=dev` (= `fmt-check` + `validate` + `lint` + `security`). Never run `terraform apply`.

## Go (`app/api`, `app/auth`)

- DDD layered architecture with a one-way dependency `route → service → domain`. `domain` depends on nothing and is framework-free and unit-testable.
- Standard library only, with one deliberate exception: `infra/postgres` depends on `github.com/jackc/pgx/v5` (the sole runtime dependency in `go.mod`/`go.sum`) as a `database/sql` driver for the Postgres-backed `Repository` implementations (SPEC-005). `domain` / `service` / `route` remain stdlib-only; `infra/memory` (the default/test persistence) is unaffected. Migration (goose) and codegen (sqlc) tools run via `go run pkg@pinned` (see each stack's `Makefile`) and never become `go.mod` dependencies. `cmd/<binary>/main.go` is the composition root (wiring only, no logic) — including the memory/Postgres persistence selection.
- Package by domain (split packages per aggregate/domain, not per technical layer). Both `app/api` and `app/auth` use top-level layer directories (`domain` / `service` / `infra` / `route` / `cmd`) and do not use an `internal/` tree.
- `context.Context` is the first argument, never stored on a struct.
- Wrap errors with `fmt.Errorf("...: %w", err)`. For branchable errors define sentinel (`var ErrX = errors.New(...)`) or custom types and match with `errors.Is` / `errors.As`. Never use `panic` for error handling.
- Abstract DB / external calls behind interfaces (defined by the consumer) so tests can substitute fakes. Every goroutine must have a termination condition.
- Tests: standard `go test`, table-driven; cover success / error / boundary cases.
- `app/auth` (auth server): never hardcode keys/secrets; demo values are generated or seeded at startup and injected from the environment in production. Enforce PKCE (S256), single-use short-lived auth codes, RS256-signed JWTs with `kid` + JWKS (reject `alg: none`), and full `iss` / `aud` / `exp` / signature validation.

## TypeScript / React (`app/web`)

- TypeScript strict mode. **`any` is forbidden** — use `unknown` + type guards. `as` casts only at boundaries (e.g. parsing API responses).
- Validate all external data (API / localStorage / URL params) with a schema (zod) before typing it.
- **Named exports only** (no `default` export unless a routing convention requires it).
- Function components + hooks only (no class components). Separate server state (API data via TanStack Query) from client/UI state; do not copy server state into `useState`.
- One-way layering `components → hooks → (api | domain)`. Business rules (state transitions, invariants, derived values, filter/sort) live in `features/<feature>/domain/` as pure, React/DOM/fetch-free functions; components and hooks call them and hold no logic. Isolate side effects in hooks.
- Do not use array index as a list `key` (except static, never-reordered lists).
- Lint/format with **Biome** (not ESLint/Prettier); type-check with **tsgo** (not `tsc`). Runtime and package manager is **Bun**.
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
