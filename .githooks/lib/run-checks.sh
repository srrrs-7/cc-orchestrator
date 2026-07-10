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
  make -C "$repo_root/app/api" openapi
  make -C "$repo_root/app/web" install INSTALL_FLAGS=--frozen-lockfile
  make -C "$repo_root/app/web" generate

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
  make -C "$repo_root/app/$stack" sqlc

  git -C "$repo_root" add -N -- "app/$stack/infra/postgres/sqlcgen"

  if ! git -C "$repo_root" diff --exit-code -- "app/$stack/infra/postgres/sqlcgen"; then
    githooks_log "sqlc drift detected in app/$stack — run 'make -C app/$stack sqlc', then commit." >&2
    git -C "$repo_root" status -- "app/$stack/infra/postgres/sqlcgen" >&2 || true
    return 1
  fi
}

githooks_run_stack_checks() {
  local repo_root
  repo_root="$(githooks_repo_root)"

  if [[ "$NEED_WEB" == 1 ]]; then
    githooks_log "web check..."
    make -C "$repo_root/app/web" install INSTALL_FLAGS=--frozen-lockfile
    make -C "$repo_root/app/web" check
  fi

  if [[ "$NEED_API" == 1 ]]; then
    githooks_log "api check..."
    githooks_warm_go_mod app/api
    make -C "$repo_root/app/api" check
  fi

  if [[ "$NEED_AUTH" == 1 ]]; then
    githooks_log "auth check..."
    githooks_warm_go_mod app/auth
    make -C "$repo_root/app/auth" check
  fi

  if [[ "$NEED_IAC" == 1 ]]; then
    githooks_log "iac check..."
    make -C "$repo_root/app/iac" check
  fi

  if [[ "$NEED_MIGRATOR" == 1 ]]; then
    githooks_log "migrator check..."
    githooks_warm_go_mod app/migrator
    make -C "$repo_root/app/migrator" check
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

  githooks_ensure_toolbox

  githooks_print_plan
  githooks_run_stack_checks
  githooks_log "all checks passed"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  githooks_main "$@"
fi
