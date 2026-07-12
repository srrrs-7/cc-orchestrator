# Run CI-equivalent checks for affected stacks (pre-commit / make hook-check).
set -euo pipefail

GITHOOKS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=lib/common.sh
source "$GITHOOKS_DIR/lib/common.sh"
# shellcheck source=lib/detect-stacks.sh
source "$GITHOOKS_DIR/lib/detect-stacks.sh"

githooks_check_contract_drift() {
  local repo_root
  repo_root="$(githooks_repo_root)"
  githooks_log "contract drift check..."
  githooks_warm_go_mod app/api
  make -C "$repo_root/app/api" "$(githooks_make_target openapi)"
  make -C "$repo_root/app/web" "$(githooks_make_target install)" INSTALL_FLAGS=--frozen-lockfile
  make -C "$repo_root/app/web" "$(githooks_make_target generate)"

  git -C "$repo_root" add -N -- \
    app/api/docs/openapi.yaml \
    app/web/src/features/tasks/api/generated

  if ! git -C "$repo_root" diff --exit-code -- \
    app/api/docs/openapi.yaml \
    app/web/src/features/tasks/api/generated; then
    githooks_log "contract drift detected — run 'make -C app/api openapi' and 'make -C app/web generate', then commit." >&2
    git -C "$repo_root" status -- \
      app/api/docs/openapi.yaml \
      app/web/src/features/tasks/api/generated >&2 || true
    return 1
  fi
}

githooks_check_sqlc_drift() {
  local stack="$1"
  local repo_root
  repo_root="$(githooks_repo_root)"
  githooks_log "$stack sqlc drift check..."
  githooks_warm_go_mod "app/$stack"
  make -C "$repo_root/app/$stack" "$(githooks_make_target sqlc)"

  git -C "$repo_root" add -N -- "app/$stack/infra/postgres/sqlcgen"

  if ! git -C "$repo_root" diff --exit-code -- "app/$stack/infra/postgres/sqlcgen"; then
    githooks_log "sqlc drift detected in app/$stack — run 'make -C app/$stack sqlc', then commit." >&2
    git -C "$repo_root" status -- "app/$stack/infra/postgres/sqlcgen" >&2 || true
    return 1
  fi
}

# ISSUE-029 / SPEC-009 R3 fix: three-phase split.
#
# Phase 1 (warm, service `tools`, network enabled): only work that
# genuinely needs the network -- bun install, `go mod download`, iac
# validate's provider fetch, swag/openapi/sqlc code generation that
# follows a go-mod warm, bun-backed contract generation. Static analysis
# (fmt/lint/vet/build) and migrator's own test are deliberately absent
# here and instead run in the `offline-check` phase below, matching the
# same tools-offline service that CI and direct `make check` already use.
#
# api/auth's own DB-backed `test-native` is absent from both this phase
# and offline-check; it runs only in githooks_run_db_test_phase
# (tools-db, no internet -- SPEC-013 R6).
githooks_run_warm_phase() {
  local repo_root
  repo_root="$(githooks_repo_root)"

  if [[ "$NEED_WEB" == 1 ]]; then
    githooks_log "web check (warm + check in tools, network-enabled)..."
    make -C "$repo_root/app/web" "$(githooks_make_target install)" INSTALL_FLAGS=--frozen-lockfile
    make -C "$repo_root/app/web" "$(githooks_make_target check)"
  fi

  if [[ "$NEED_API" == 1 ]]; then
    githooks_log "api: warming Go module cache (tools, network-enabled)..."
    githooks_warm_go_mod app/api
    # fmt-check/lint/vet/build run in offline-check phase (tools-offline).
  fi

  if [[ "$NEED_AUTH" == 1 ]]; then
    githooks_log "auth: warming Go module cache (tools, network-enabled)..."
    githooks_warm_go_mod app/auth
    # fmt-check/lint/vet/build run in offline-check phase (tools-offline).
  fi

  if [[ "$NEED_IAC" == 1 ]]; then
    githooks_log "iac check (tools, network-enabled -- validate needs provider fetch)..."
    githooks_run_iac_check "$repo_root"
  fi

  if [[ "$NEED_MIGRATOR" == 1 ]]; then
    githooks_log "migrator: warming Go module cache (tools, network-enabled)..."
    githooks_warm_go_mod app/migrator
    # check-native (fmt-check+lint+vet+build+test) runs in offline-check
    # phase (tools-offline). migrator tests are DB-free, so they are safe
    # to run under GOPROXY=off once the module cache is warmed here.
  fi

  if [[ "$NEED_CONTRACT_DRIFT" == 1 ]]; then
    githooks_check_contract_drift
  fi

  if [[ "$NEED_SQLC_API" == 1 ]]; then
    githooks_check_sqlc_drift api
  fi

  if [[ "$NEED_SQLC_AUTH" == 1 ]]; then
    githooks_check_sqlc_drift auth
  fi
}

# ISSUE-029 / SPEC-009 R3 fix: Phase 2 (offline-check, service
# `tools-offline`, --network none / GOPROXY=off): static analysis and
# compilation for Go stacks, exactly matching what CI and direct `make
# check` already run in tools-offline.
#
# For api/auth: only fmt-check/lint/vet/build -- NOT test-native; that
# runs in githooks_run_db_test_phase (tools-db) because it needs a real
# Postgres. For migrator: full check-native (fmt-check+lint+vet+build+
# test) -- migrator tests are DB-free, so they run here under GOPROXY=off
# once the module cache was warmed in the preceding warm phase.
githooks_run_offline_check_phase() {
  local repo_root
  repo_root="$(githooks_repo_root)"

  if [[ "$NEED_API" == 1 ]]; then
    githooks_log "api offline check: fmt-check + lint + vet + build (tools-offline, --network none)..."
    # Call the four offline-only native targets individually rather than
    # check-offline-native or check-native: api/auth test-native is
    # DB-backed and must only run in the db-test phase (tools-db).
    make -C "$repo_root/app/api" fmt-check-native lint-native vet-native build-native
  fi

  if [[ "$NEED_AUTH" == 1 ]]; then
    githooks_log "auth offline check: fmt-check + lint + vet + build (tools-offline, --network none)..."
    make -C "$repo_root/app/auth" fmt-check-native lint-native vet-native build-native
  fi

  if [[ "$NEED_MIGRATOR" == 1 ]]; then
    githooks_log "migrator check: fmt-check + lint + vet + build + test (tools-offline, --network none; ISSUE-029 Info)..."
    # check-native = fmt-check+lint+vet+build+test. migrator tests are
    # DB-free, so running them in tools-offline (GOPROXY=off, module cache
    # pre-warmed in the preceding warm phase) satisfies both ISSUE-029's
    # Info item (go test should also be internet-non-reachable) and
    # SPEC-009 R6's spirit.
    make -C "$repo_root/app/migrator" "$(githooks_make_target check)"
  fi
}

# SPEC-013 R6 fix: DB-backed test execution ONLY, no internet reachable
# (runs inside the `tools-db` phase -- see common.sh's
# githooks_reexec_phase). Sets each stack's own local-only, non-secret
# DB_* defaults explicitly as real process env vars, not Make variables:
# `test-native` is a bare `go test` recipe with no `-e`/Makefile-level
# DB_* plumbing of its own (that plumbing lives only in each stack's
# HOST-side `test`/`check` targets, which this phase deliberately bypasses
# to avoid re-entering a second, nested `docker compose run` from inside
# an already-toolboxed container -- toolbox containers have no Docker
# socket, SPEC-009). REQUIRE_DB=1 (SPEC-013 plan §1.5 admin ruling): the
# regular hook path must fail loudly (t.Fatal), never silently t.Skip, if
# the test DB connection is ever unavailable here. Postgres + api_test/
# auth_test themselves were already provisioned by githooks_main's host
# branch (db-up + migrate-test) before this phase's container ever starts.
githooks_run_db_test_phase() {
  local repo_root
  repo_root="$(githooks_repo_root)"

  if [[ "$NEED_API" == 1 ]]; then
    githooks_log "api DB test (api_test, no-internet phase)..."
    DB_HOST=postgres DB_PORT=5432 DB_NAME=api_test DB_USER=app DB_PASSWORD=app DB_SSLMODE=disable REQUIRE_DB=1 \
      make -C "$repo_root/app/api" test-native
  fi

  if [[ "$NEED_AUTH" == 1 ]]; then
    githooks_log "auth DB test (auth_test, no-internet phase)..."
    DB_HOST=postgres DB_PORT=5432 DB_NAME=auth_test DB_USER=app DB_PASSWORD=app DB_SSLMODE=disable REQUIRE_DB=1 \
      make -C "$repo_root/app/auth" test-native
  fi
}

githooks_main() {
  if [[ "${SKIP_PRE_COMMIT:-}" == 1 ]]; then
    githooks_log "SKIP_PRE_COMMIT=1 — skipping checks"
    exit 0
  fi

  githooks_detect_stacks

  if ! githooks_any_stack_needed; then
    githooks_log "no CI-relevant staged changes — skipping checks"
    exit 0
  fi

  if githooks_in_toolbox; then
    # Either one of the two phase containers common.sh's
    # githooks_reexec_phase started below (GITHOOKS_PHASE tells us
    # which), or a direct devcontainer session (GITHOOKS_PHASE unset --
    # no host-level re-exec ever ran to set it, because a devcontainer
    # session already has IN_TOOLBOX=1 from the start). In the
    # devcontainer case there is no host-level container boundary to
    # split the two phases across at all (that session has no Docker
    # socket of its own, SPEC-009, so it cannot start either phase
    # container itself) -- this is a pre-existing gap (the same one
    # already applied to `make test-integration` before SPEC-013) that
    # this fix does not newly introduce or attempt to close; running
    # both phases natively in that one session is the best available
    # fallback, on the understanding that a devcontainer's own network
    # reachability is a separate, already-accepted caveat (see
    # .githooks/README.md's own "Devcontainer" note).
    case "${GITHOOKS_PHASE:-}" in
      warm)
        githooks_print_plan
        githooks_run_warm_phase
        ;;
      offline-check)
        githooks_run_offline_check_phase
        ;;
      db-test)
        githooks_run_db_test_phase
        ;;
      *)
        # Devcontainer / direct IN_TOOLBOX=1 session: no container
        # boundary to enforce network splits across, so run all three
        # phases sequentially in this one session (same pre-existing
        # caveat as before ISSUE-029 -- see README.md "Devcontainer").
        githooks_print_plan
        githooks_run_warm_phase
        githooks_run_offline_check_phase
        githooks_run_db_test_phase
        ;;
    esac
    githooks_log "all checks passed"
    return 0
  fi

  # Host: not in toolbox yet.
  if ! command -v docker >/dev/null 2>&1; then
    githooks_die "Docker is required on the host. Install Docker or skip with SKIP_PRE_COMMIT=1."
  fi

  local repo_root
  repo_root="$(githooks_repo_root)"
  githooks_export_versions_env "$repo_root"

  # Phase 1: warm (tools, network-enabled) -- go mod download / bun
  # install / iac validate / web check / contract+sqlc drift. Always run
  # when any stack is needed (even WEB-only or IAC-only stages reach here).
  githooks_reexec_phase warm tools

  # Phase 2: offline-check (tools-offline, --network none / GOPROXY=off)
  # -- api/auth fmt-check+lint+vet+build, migrator full check-native.
  # Skipped when only non-Go stacks (web / iac) are staged.
  # ISSUE-029: this is the fix -- previously these ran in the network-
  # enabled `tools` container alongside go mod download; now they run in
  # the same tools-offline service CI and direct `make check` already use.
  if [[ "$NEED_API" == 1 || "$NEED_AUTH" == 1 || "$NEED_MIGRATOR" == 1 ]]; then
    githooks_reexec_phase offline-check tools-offline
  fi

  # ISSUE-028 partial fix: provision Postgres AFTER offline checks pass.
  # Previously db-up + migrate-test ran unconditionally before the warm
  # phase, so a fmt-check/lint/vet/build failure still paid the full DB
  # startup + migration cost.  Moving this block here means that if the
  # offline-check phase above fails (set -euo pipefail + githooks_reexec_phase
  # exits non-zero), we never start Postgres at all -- fail-fast restored.
  # root `make db-up`/`make migrate-test` shell out to `docker compose`
  # themselves (no IN_TOOLBOX-guarded `-native` counterpart), so they must
  # run on the host; the ordering change is all that is needed here.
  if [[ "$NEED_API" == 1 || "$NEED_AUTH" == 1 ]]; then
    githooks_log "provisioning Postgres test databases (api_test/auth_test)..."
    make -C "$repo_root" db-up
    make -C "$repo_root" migrate-test
  fi

  # Phase 3: db-test (tools-db, Postgres-reachable, no internet) -- api/
  # auth `go test` only (SPEC-013 R6, unchanged).
  if [[ "$NEED_API" == 1 || "$NEED_AUTH" == 1 ]]; then
    githooks_reexec_phase db-test tools-db
  fi

  githooks_log "all checks passed"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  githooks_main "$@"
fi
