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

# SPEC-013 R6 fix: everything that either needs the network (web install,
# iac validate's provider fetch, go module warming) or does not execute
# dependency/test code with the internet reachable in the sense R6 is
# scoped to (fmt/lint/vet/build -- static analysis and compilation, not
# `go test` execution). Runs inside the `tools` (network-enabled) phase --
# see common.sh's githooks_reexec_phase for why this is split from
# githooks_run_db_test_phase below into two separate container
# invocations. api/auth's own DB-backed `test-native` is deliberately NOT
# called here; it runs only in githooks_run_db_test_phase, no-internet.
githooks_run_offline_phase() {
  local repo_root
  repo_root="$(githooks_repo_root)"

  if [[ "$NEED_WEB" == 1 ]]; then
    githooks_log "web check..."
    make -C "$repo_root/app/web" "$(githooks_make_target install)" INSTALL_FLAGS=--frozen-lockfile
    make -C "$repo_root/app/web" "$(githooks_make_target check)"
  fi

  if [[ "$NEED_API" == 1 ]]; then
    githooks_log "api offline check (fmt-check + lint + vet + build; DB test runs in a separate no-internet phase)..."
    githooks_warm_go_mod app/api
    # fmt-check-native/lint-native/vet-native/build-native individually
    # (not the bundled check-native, which for app/api also includes
    # test-native -- see app/api/Makefile's own check-offline-native vs.
    # check-native split): this phase must never run test-native, so it
    # calls the four offline-only native targets by name instead of
    # relying on either stack's own (inconsistently named -- api has
    # check-offline-native, auth's check-native IS already offline-only)
    # bundle target.
    make -C "$repo_root/app/api" fmt-check-native lint-native vet-native build-native
  fi

  if [[ "$NEED_AUTH" == 1 ]]; then
    githooks_log "auth offline check (fmt-check + lint + vet + build; DB test runs in a separate no-internet phase)..."
    githooks_warm_go_mod app/auth
    make -C "$repo_root/app/auth" fmt-check-native lint-native vet-native build-native
  fi

  if [[ "$NEED_IAC" == 1 ]]; then
    githooks_log "iac check..."
    githooks_run_iac_check "$repo_root"
  fi

  if [[ "$NEED_MIGRATOR" == 1 ]]; then
    githooks_log "migrator check..."
    githooks_warm_go_mod app/migrator
    make -C "$repo_root/app/migrator" "$(githooks_make_target check)"
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
      offline)
        githooks_print_plan
        githooks_run_offline_phase
        ;;
      db-test)
        githooks_run_db_test_phase
        ;;
      *)
        githooks_print_plan
        githooks_run_offline_phase
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

  # SPEC-013: app/api's/app/auth's own `test-native` is now test-DB-backed
  # by default (api_test/auth_test, not the dev api/auth databases) -- see
  # this repo's root Makefile `migrate-test` target and app/{api,auth}/
  # Makefile's own `check`/`test`. Provision that Postgres + those two test
  # databases on the HOST'S OWN Docker before either phase container
  # starts: root `make db-up`/`make migrate-test` always shell out to
  # `docker compose` themselves (unlike stack Makefiles' targets, they
  # have no IN_TOOLBOX-guarded `-native` counterpart), so they cannot run
  # nested inside a toolbox container -- they must run here, still on the
  # host, exactly like CI's own `api`/`auth` jobs provision Postgres
  # before their own `make check` step.
  if [[ "$NEED_API" == 1 || "$NEED_AUTH" == 1 ]]; then
    githooks_log "provisioning Postgres test databases (api_test/auth_test)..."
    make -C "$repo_root" db-up
    make -C "$repo_root" migrate-test
  fi

  githooks_reexec_phase offline tools

  if [[ "$NEED_API" == 1 || "$NEED_AUTH" == 1 ]]; then
    githooks_reexec_phase db-test tools-db
  fi

  githooks_log "all checks passed"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  githooks_main "$@"
fi
