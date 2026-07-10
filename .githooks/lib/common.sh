# Shared helpers for githooks (SPEC-009: same toolchain container as CI).
set -euo pipefail

githooks_repo_root() {
  git rev-parse --show-toplevel
}

githooks_log() {
  printf 'githooks: %s\n' "$*"
}

githooks_die() {
  githooks_log "error: $*" >&2
  exit 1
}

githooks_in_toolbox() {
  [[ -n "${IN_TOOLBOX:-}" ]]
}

# Prefix match when pattern ends with '/'; exact match otherwise.
githooks_match_path() {
  local file="$1"
  local pattern="$2"
  if [[ "$pattern" == */ ]]; then
    [[ "$file" == "${pattern}"* ]]
  else
    [[ "$file" == "$pattern" ]]
  fi
}

githooks_matches_any() {
  local file="$1"
  shift
  local pattern
  for pattern in "$@"; do
    if githooks_match_path "$file" "$pattern"; then
      return 0
    fi
  done
  return 1
}

githooks_compose_bin() {
  if docker compose version >/dev/null 2>&1; then
    echo "docker compose"
  else
    echo "docker-compose"
  fi
}

githooks_export_versions_env() {
  local repo_root="$1"
  local versions_file="$repo_root/.devcontainer/versions.env"
  if [[ -f "$versions_file" ]]; then
    set -a
    # shellcheck source=/dev/null
    source "$versions_file"
    set +a
  fi
}

# Inside toolbox (IN_TOOLBOX=1), stack Makefiles only expose *-native targets.
githooks_make_target() {
  local target="$1"
  if githooks_in_toolbox; then
    printf '%s-native' "$target"
  else
    printf '%s' "$target"
  fi
}

githooks_run_iac_check() {
  local repo_root="$1"
  if githooks_in_toolbox; then
    make -C "$repo_root/app/iac" fmt-check-native
    make -C "$repo_root/app/iac" validate-native
    make -C "$repo_root/app/iac" lint-native
    make -C "$repo_root/app/iac" security-native
  else
    make -C "$repo_root/app/iac" check
  fi
}

# Re-exec hook logic inside the online toolbox on the host. No-op when
# IN_TOOLBOX is already set (devcontainer session or nested make).
githooks_ensure_toolbox() {
  if githooks_in_toolbox; then
    return 0
  fi

  if ! command -v docker >/dev/null 2>&1; then
    githooks_die "Docker is required on the host. Install Docker or skip with SKIP_PRE_COMMIT=1."
  fi

  local repo_root
  repo_root="$(githooks_repo_root)"
  local compose
  compose="$(githooks_compose_bin)"

  githooks_export_versions_env "$repo_root"
  githooks_log "running inside toolchain container..."

  TOOLBOX_UID="$(id -u)" TOOLBOX_GID="$(id -g)" \
    COMPOSE_PROJECT_NAME=cc-orchestrator \
    TOOLBOX_WORKSPACE="$repo_root" \
    TOOLBOX_CONTEXT="$repo_root/.devcontainer/toolchain" \
    "$compose" -f "$repo_root/.devcontainer/compose.tools.yml" \
      run --rm -w /workspace \
      tools bash .githooks/lib/run-checks.sh

  exit $?
}

githooks_warm_go_mod() {
  local stack_dir="$1"
  githooks_log "warming Go module cache ($stack_dir)..."
  local repo_root
  repo_root="$(githooks_repo_root)"
  (
    cd "$repo_root/$stack_dir"
    go mod download
  )
}
