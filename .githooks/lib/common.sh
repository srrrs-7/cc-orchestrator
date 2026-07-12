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

# Re-exec run-checks.sh inside ONE toolbox *phase* container.
#
# History / rationale:
#
# SPEC-013 R6 (review-security Major, 2026-07-11): the hook used to
# re-exec its entire run into a single network-enabled `tools` container,
# letting `go test` (which executes arbitrary dependency code against a
# real Postgres) run with the internet reachable. Fixed by splitting into
# two separate `docker compose run` invocations so that `go test` only
# runs in a no-internet container.
#
# ISSUE-029 (2026-07-12): the two-phase split still left api/auth's own
# fmt-check/lint/vet/build running in the network-enabled `tools` phase
# alongside go mod download, creating an asymmetry vs. CI and direct
# `make check` (which run those checks in tools-offline). Fixed by adding
# a third phase so that static analysis also runs in tools-offline,
# matching CI exactly.
#
# Three phases, each its OWN `docker compose run` (a running container
# cannot change its own network profile mid-lifetime, so true separation
# requires separate invocations, not a single shared one):
#
#   warm          -- service `tools` (network enabled, compose.tools.yml
#                   alone). Only genuinely network-needing work: `bun
#                   install`/web check, iac validate's provider fetch,
#                   `go mod download` warming for Go stacks, contract/
#                   sqlc drift generation. Static analysis absent here
#                   (ISSUE-029).
#   offline-check -- service `tools-offline` (--network none / GOPROXY=
#                   off). api/auth fmt-check+lint+vet+build; migrator
#                   full check-native (fmt-check+lint+vet+build+test,
#                   DB-free). Matches what CI and direct `make check`
#                   already run in tools-offline (ISSUE-029 fix).
#   db-test       -- service `tools-db` (dbnet: postgres-reachable, no
#                   internet route at all -- SPEC-013 R6). ONLY api/auth
#                   `test-native` (`go test` against a real Postgres).
#                   Layers the sibling compose.yml on top so `postgres`
#                   resolves by name.
#
# Named volumes (gomodcache/gobuild/buncache) persist across all three
# invocations, so offline-check and db-test both see whatever warm's
# `go mod download` already cached -- the same cross-invocation caching
# CI already relies on (its "Warm Go module cache" step vs. "Check" step
# run in different containers).
#
# `$1` = phase name (exported into the re-exec'd process as
# GITHOOKS_PHASE, read by run-checks.sh's githooks_main to pick which of
# githooks_run_warm_phase / githooks_run_offline_check_phase /
# githooks_run_db_test_phase to run).
# `$2` = service (`tools` | `tools-offline` | `tools-db`).
githooks_reexec_phase() {
  local phase="$1"
  local service="$2"
  local repo_root
  repo_root="$(githooks_repo_root)"

  # An array, not a plain quoted scalar: githooks_compose_bin can return
  # the two-word "docker compose" (the modern plugin form), and invoking a
  # *quoted* `"$compose"` with an embedded space later below tries to run
  # a single executable literally named "docker compose" (with the space
  # in the filename), which does not exist and fails "command not found"
  # -- verified empirically. `read -a` word-splits it into separate argv
  # elements up front instead (safe here: neither "docker compose" nor
  # "docker-compose" contains anything `read`'s default IFS would mis-split
  # on).
  local -a compose_cmd
  read -r -a compose_cmd <<< "$(githooks_compose_bin)"

  # Only `db-test` needs `postgres` reachable at all -- `offline` never
  # touches it (see this function's own header comment).
  local -a compose_files=(-f "$repo_root/.devcontainer/compose.tools.yml")
  if [[ "$service" == "tools-db" ]]; then
    compose_files=(-f "$repo_root/compose.yml" -f "$repo_root/.devcontainer/compose.tools.yml")
  fi

  githooks_log "running phase '$phase' inside toolbox ($service)..."

  # `-e GITHOOKS_PHASE` (bare, no `=value`) forwards this invocation's own
  # `GITHOOKS_PHASE="$phase"` prefix -- set on this exact command line, so
  # docker compose's own process environment has it -- into the container,
  # the same bare-passthrough idiom app/auth/Makefile's own `-e
  # REQUIRE_DB` already uses.
  GITHOOKS_PHASE="$phase" \
    TOOLBOX_UID="$(id -u)" TOOLBOX_GID="$(id -g)" \
    COMPOSE_PROJECT_NAME=cc-orchestrator \
    TOOLBOX_WORKSPACE="$repo_root" \
    TOOLBOX_CONTEXT="$repo_root/.devcontainer/toolchain" \
    "${compose_cmd[@]}" "${compose_files[@]}" \
      run --rm -e GITHOOKS_PHASE -w /workspace \
      "$service" bash .githooks/lib/run-checks.sh
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
