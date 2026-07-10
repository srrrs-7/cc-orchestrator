# Detect which CI checks to run from staged paths.
# Path filters mirror .github/workflows/cicd.yml, contract-drift.yml, sqlc-drift.yml.
set -euo pipefail

# Outputs (0/1 globals): NEED_WEB NEED_API NEED_AUTH NEED_IAC NEED_MIGRATOR
# NEED_CONTRACT_DRIFT NEED_SQLC_API NEED_SQLC_AUTH
NEED_WEB=0
NEED_API=0
NEED_AUTH=0
NEED_IAC=0
NEED_MIGRATOR=0
NEED_CONTRACT_DRIFT=0
NEED_SQLC_API=0
NEED_SQLC_AUTH=0

githooks_detect_stacks() {
  NEED_WEB=0
  NEED_API=0
  NEED_AUTH=0
  NEED_IAC=0
  NEED_MIGRATOR=0
  NEED_CONTRACT_DRIFT=0
  NEED_SQLC_API=0
  NEED_SQLC_AUTH=0

  local files
  files="$(git diff --cached --name-only --diff-filter=ACMR)"
  if [[ -z "$files" ]]; then
    return 0
  fi

  local all_stacks=0
  local file

  while IFS= read -r file; do
    [[ -z "$file" ]] && continue

    # cicd.yml `changes` job — toolchain-common paths re-run every stack.
    if githooks_matches_any "$file" \
      .github/workflows/cicd.yml \
      .devcontainer/versions.env \
      .devcontainer/toolchain/ \
      .devcontainer/compose.tools.yml \
      .github/actions/build-toolchain-image/ ; then
      all_stacks=1
    fi

    if githooks_matches_any "$file" app/web/ ; then
      NEED_WEB=1
    fi
    if githooks_matches_any "$file" \
      app/api/ \
      compose.yml \
      Makefile ; then
      NEED_API=1
    fi
    if githooks_matches_any "$file" \
      app/auth/ \
      compose.yml \
      Makefile ; then
      NEED_AUTH=1
    fi
    if githooks_matches_any "$file" app/iac/ ; then
      NEED_IAC=1
    fi
    if githooks_matches_any "$file" \
      app/migrator/ \
      compose.yml \
      Makefile ; then
      NEED_MIGRATOR=1
    fi

    # contract-drift.yml paths
    if githooks_matches_any "$file" \
      app/api/ \
      app/web/ \
      .devcontainer/versions.env \
      .devcontainer/toolchain/ \
      .devcontainer/compose.tools.yml \
      .github/actions/build-toolchain-image/ \
      .github/workflows/contract-drift.yml ; then
      NEED_CONTRACT_DRIFT=1
    fi

    # sqlc-drift.yml paths
    if githooks_matches_any "$file" \
      app/api/db/ \
      app/api/infra/postgres/ \
      app/api/sqlc.yaml \
      .devcontainer/versions.env \
      .devcontainer/toolchain/ \
      .devcontainer/compose.tools.yml \
      .github/actions/build-toolchain-image/ \
      .github/workflows/sqlc-drift.yml ; then
      NEED_SQLC_API=1
    fi
    if githooks_matches_any "$file" \
      app/auth/db/ \
      app/auth/infra/postgres/ \
      app/auth/sqlc.yaml \
      .devcontainer/versions.env \
      .devcontainer/toolchain/ \
      .devcontainer/compose.tools.yml \
      .github/actions/build-toolchain-image/ \
      .github/workflows/sqlc-drift.yml ; then
      NEED_SQLC_AUTH=1
    fi
  done <<< "$files"

  if [[ "$all_stacks" == 1 ]]; then
    NEED_WEB=1
    NEED_API=1
    NEED_AUTH=1
    NEED_IAC=1
    NEED_MIGRATOR=1
  fi
}

githooks_any_stack_needed() {
  [[ "$NEED_WEB$NEED_API$NEED_AUTH$NEED_IAC$NEED_MIGRATOR$NEED_CONTRACT_DRIFT$NEED_SQLC_API$NEED_SQLC_AUTH" == *1* ]]
}

githooks_print_plan() {
  githooks_log "checks to run:"
  [[ "$NEED_WEB" == 1 ]] && githooks_log "  - web: make install + make check"
  [[ "$NEED_API" == 1 ]] && githooks_log "  - api: make check"
  [[ "$NEED_AUTH" == 1 ]] && githooks_log "  - auth: make check"
  [[ "$NEED_IAC" == 1 ]] && githooks_log "  - iac: make check"
  [[ "$NEED_MIGRATOR" == 1 ]] && githooks_log "  - migrator: make check"
  [[ "$NEED_CONTRACT_DRIFT" == 1 ]] && githooks_log "  - contract drift (make openapi + make generate)"
  [[ "$NEED_SQLC_API" == 1 ]] && githooks_log "  - api sqlc drift (make sqlc)"
  [[ "$NEED_SQLC_AUTH" == 1 ]] && githooks_log "  - auth sqlc drift (make sqlc)"
}
