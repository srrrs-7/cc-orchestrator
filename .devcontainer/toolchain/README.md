# .devcontainer/toolchain

Single polyglot developer/CI toolchain image (SPEC-009: `docs/specs/20260710-009-containerized-toolchain-no-host-runtime.md`).
Bakes every pinned tool this monorepo's Makefiles need — go, bun, golangci-lint, terraform, tflint, trivy, plus the CLIs
each Go stack's Makefile shells out to (sqlc / goose / swag / goimports)
— into one image, so local dev, CI, and the devcontainer all run the
exact same pinned toolchain instead of each installing (and potentially
drifting from) their own copies.

## Why

See the spec's "1. ユーザー価値" for the full threat model. In short:
`bun install`'s postinstall scripts, `go run tool@version`, and similar
dependency-install-time code execution are the highest-risk moment for a
supply-chain attack (e.g. a Shai-Hulud-style npm worm) to read host
secrets (`~/.aws`, `~/.ssh`, `~/.npmrc`, ambient env vars) and exfiltrate
or self-propagate. Running that code inside a disposable container that
holds no host secrets, with normal execution additionally running
`--network none`, removes that access entirely — see `.devcontainer/compose.tools.yml`
for how the two services (`tools` / `tools-offline`) apply this.

## Versions

Every tool version baked into this image is an ARG, supplied from
`../versions.env` (the repo's single source of truth for tool
versions) via `.devcontainer/compose.tools.yml`'s `build.args`. **Do not hardcode a
version in this Dockerfile** — bump it in `versions.env` instead, then
rebuild.

| Tool | ARG | Install method |
|---|---|---|
| go | `GO_VERSION` | base image (`golang:${GO_VERSION}-bookworm`) |
| bun | `BUN_VERSION` | official install script (`bun.sh/install`, same as `oven-sh/setup-bun`) |
| golangci-lint | `GOLANGCI_LINT_VERSION` | official install script (same as the CI job this image replaces) |
| terraform | `TERRAFORM_VERSION` | release `.zip` + published `SHA256SUMS`, verified before unzip |
| tflint | `TFLINT_VERSION` | release `.zip` + published `checksums.txt`, verified before unzip |
| trivy | `TRIVY_VERSION` | official install script (verifies its own checksum internally) |
| sqlc | `SQLC_VERSION` | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@<version>` |
| goose (CLI) | `GOOSE_VERSION` | `go install github.com/pressly/goose/v3/cmd/goose@<version>` (kept in sync with `app/migrator/go.mod`'s library require — see `.claude/rules/db.md`) |
| swag v2 | `SWAG_VERSION` | `go install github.com/swaggo/swag/v2/cmd/swag@<version>` |
| goimports | `GOIMPORTS_VERSION` | `go install golang.org/x/tools/cmd/goimports@<version>` |

`TRIVY_VERSION` is **not** `v0.58.1` even though every other tool above
matches `.github/workflows/cicd.yml`'s current pin verbatim — see
`versions.env`'s own comment: aquasecurity has deleted the GitHub
Release (and its binary/checksum assets) for that tag, confirmed via
direct `404` responses during this implementation, so it cannot be
built regardless of image design. Pinned to the nearest still-resolvable
release instead (`v0.69.2`); `cicd.yml`'s own pin needs the same fix in
Phase C.

## Build

```sh
docker compose --env-file versions.env -f .devcontainer/compose.tools.yml build
```

If your `docker` CLI has no `compose` subcommand (no buildx-backed
compose v2 plugin installed), fall back to the standalone `docker-compose`
binary instead — same syntax, e.g. `docker-compose --env-file versions.env
-f .devcontainer/compose.tools.yml build` (this is exactly the fallback the repo-root
`Makefile` already probes for). Note the standalone binary shells out to
the legacy (non-BuildKit) `docker build`, under which BuildKit's
automatic `TARGETARCH` build arg is NOT populated — this Dockerfile
deliberately does not rely on it for that reason (see its own comment;
architecture is instead resolved at build time via `dpkg
--print-architecture`).

(A plain `docker compose -f .devcontainer/compose.tools.yml build` — no
`--env-file` flag, e.g. what the VS Code Dev Containers extension runs
internally — resolves build args via the fallback defaults baked into
`compose.tools.yml`'s `build.args` (e.g. `GO_VERSION: ${GO_VERSION:-1.26}`).
These defaults mirror `.devcontainer/versions.env` and must be kept in sync
with it whenever a version is bumped there.)

## Design notes

- **Build context is this directory only** (`.devcontainer/toolchain`), never
  the repo root — this image bakes in *tools*, not repository *source*.
  Source is bind-mounted at `/workspace` at run time by
  `.devcontainer/compose.tools.yml`, so there is nothing here that needs a large build
  context or a `.dockerignore`.
- **Non-root**: a fixed `tools` user (uid/gid 1000) is the image's own
  default, but `.devcontainer/compose.tools.yml` overrides the effective `user:` at
  run time to the invoking host user's uid:gid, so files created under
  the `/workspace` bind-mount are owned by the calling developer/CI
  user, not by this image's baked-in uid. Every cache directory this
  image writes to is therefore world-writable (`chmod -R 0777`), so an
  arbitrary uid:gid can still write new cache entries at run time.
- **Cache paths double as compose volume mount points**:
  `GOMODCACHE=/cache/go/mod`, `GOCACHE=/cache/go/build`,
  `BUN_INSTALL_CACHE_DIR=/cache/bun`, `TF_PLUGIN_CACHE_DIR=/cache/tf-plugins`
  are exactly the paths `.devcontainer/compose.tools.yml` mounts named volumes over.
  Docker copies a directory's existing image content into a brand-new
  named volume the first time it is mounted (documented Docker volume
  behaviour), so the module/build cache this Dockerfile warms by
  `go install`-ing sqlc/goose/swag/goimports survives into the
  (initially empty) `gomodcache`/`gobuild` volumes on first use.
  `GOBIN=/usr/local/go-tools/bin` is deliberately **not** one of those
  volume-mounted paths, so the installed binaries are always present
  regardless of volume state.
- **`go run pkg@version` does not work offline — call the installed
  binaries by name instead (verified, corrects the SPEC-009 plan's own
  assumption).** `go run pkg@version` performs a "loading deprecation
  for `<module>`" module-proxy lookup on every invocation, unconditionally,
  and fails under `GOPROXY=off` even when the exact module+version is
  fully present in `GOMODCACHE`. `go install` (used above, a one-time
  build rather than `go run`'s per-invocation resolution path) does not
  hit this. **Consequence for Phase B**: each stack's Makefile must
  invoke `sqlc` / `goose` / `swag` / `goimports` by their installed
  binary name (already on `PATH` via `GOBIN` in every image, regardless
  of volume state) rather than `go run pkg@version`, for the exec phase
  to actually complete under `tools-offline`. This is the SPEC-009
  plan's own pre-approved fallback for exactly this scenario ("万一
  offline 実行できない場合の代替"), now confirmed necessary rather than
  hypothetical.
- **`go build`/`go vet`/`go test` need `git` to trust `/workspace`.**
  Go's VCS build-info stamping shells out to `git`, which refuses to run
  in a bind-mounted directory it considers to have "dubious ownership"
  whenever the mount point's reported uid does not match the container's
  effective uid (observed here under colima/virtiofs: the mount point
  itself reports uid 0 while files inside it correctly report the host
  uid — a real-world instance of exactly the mismatch Git's
  CVE-2022-24765 mitigation guards against). `.devcontainer/compose.tools.yml` works
  around this with Git's env-var config injection
  (`GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_0`/`GIT_CONFIG_VALUE_0` =
  `safe.directory=/workspace`) rather than a persistent `~/.gitconfig`
  write, since `$HOME` here is baked into the image, not a volume, and
  would not persist across `run` invocations anyway.
- **This only pre-warms the cache for sqlc/goose/swag/goimports' own
  module graphs.** Each Go app module's own `go.sum`-declared runtime
  dependencies (e.g. `app/migrator`'s `pgx` + `goose`-as-a-library) are
  a separate concern and still need one network-enabled `go mod
  download` (via the `tools` service) before *that* module can build/
  test fully offline — see the repo-root `.devcontainer/compose.tools.yml`'s header
  comment and this file's "検証" section below.
- **sqlc's `go.mod` requires `go >= 1.26.0`, matching `GO_VERSION` in
  `versions.env`.** No `GOTOOLCHAIN=auto` override is needed (ISSUE-027
  removed the workaround that existed while the base was go1.24).
- **Image size (~5.9GB) is dominated by sqlc's own dependency graph**
  (it vendors client libraries for essentially every SQL engine it
  supports — MySQL/Postgres/SQLite/ClickHouse/YDB/etc.). This is
  baked-in `GOMODCACHE`/`GOCACHE` content deliberately kept (not
  pruned) so it can seed the `gomodcache`/`gobuild` named volumes on
  first use (see above) — trimming it after the fact would defeat that
  purpose. A residual, known cost of the "single polyglot image"
  design (SPEC-009's own accepted trade-off vs. per-stack images).

## Git hooks

Pre-commit hooks (`.githooks/`, enabled via root `make setup-hooks`) run the same
stack checks as CI for staged files only, always inside this toolchain image:

- **Host**: the hook script re-execs via `docker compose -f .devcontainer/compose.tools.yml run tools`.
- **Devcontainer session**: `IN_TOOLBOX=1` is already set — checks run in-process with no nested Docker.

Manual run: `make hook-check`. Details: `.githooks/README.md`.

## 検証 (verification performed for this Phase A implementation)

```sh
# 1. Build
docker compose --env-file versions.env -f .devcontainer/compose.tools.yml build

# 2. compose config is valid
docker compose --env-file versions.env -f .devcontainer/compose.tools.yml config >/dev/null

# 3. Smoke test every tool version inside tools-offline (network_mode: none)
TOOLBOX_UID="$(id -u)" TOOLBOX_GID="$(id -g)" \
  docker compose --env-file versions.env -f .devcontainer/compose.tools.yml \
  run --rm tools-offline sh -c '
    go version
    bun --version
    golangci-lint version
    terraform version
    tflint --version
    trivy --version
    sqlc version
    goose -version
    swag --version
  '

# 4. Two-phase network: warm app/migrator's own go.sum deps via `tools`
#    (network), then build+lint+test it offline via `tools-offline`
#    (network none, GOPROXY=off) -- calling golangci-lint/gofmt/goimports
#    directly (not `go run`), per the "does not work offline" note above
TOOLBOX_UID="$(id -u)" TOOLBOX_GID="$(id -g)" \
  docker compose --env-file versions.env -f .devcontainer/compose.tools.yml \
  run --rm -w /workspace/app/migrator tools go mod download

TOOLBOX_UID="$(id -u)" TOOLBOX_GID="$(id -g)" \
  docker compose --env-file versions.env -f .devcontainer/compose.tools.yml \
  run --rm -w /workspace/app/migrator tools-offline sh -c '
    go build ./... && go vet ./... && go test ./... \
      && golangci-lint run ./... && gofmt -l . && goimports -l .
  '
```

Actually run (via the standalone `docker-compose` fallback, this
environment's `docker` CLI has no `compose` subcommand) for this Phase A
implementation: every tool in step 3 reported exactly its pinned
version; step 4's `tools-offline` run completed `go build` / `go vet` /
`go test` (all packages `ok`) / `golangci-lint run` (`0 issues`) /
`gofmt -l` / `goimports -l` (both empty, i.e. no diff) fully offline,
confirming the warm-then-run-offline flow before any Phase B Makefile
wrapper depends on it. `docker compose --env-file .devcontainer/versions.env -f
.devcontainer/compose.tools.yml config` also validated cleanly both with and without
the `--env-file` flag (the latter exercising the `build.args` fallback
defaults in `compose.tools.yml` that devcontainer relies on).
