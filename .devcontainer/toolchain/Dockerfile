# .devcontainer/toolchain/Dockerfile
#
# Single polyglot "toolchain" image (SPEC-009). Bakes every pinned
# developer/CI tool this monorepo needs -- go, bun, golangci-lint,
# terraform, tflint, trivy, plus the `go run <pkg>@<version>` CLIs each
# Go stack's Makefile shells out to (sqlc / goose / swag / goimports) --
# into one image, so local dev, CI, and the devcontainer all run the
# exact same pinned toolchain instead of each installing its own copies.
# Threat model / rationale: docs/specs/20260710-009-containerized-toolchain-no-host-runtime.md.
#
# Every version below is a build ARG, populated from ../versions.env
# (the single source of truth) via .devcontainer/compose.tools.yml's `build.args` --
# nothing in this file hardcodes a version.
#
# Build context is intentionally this directory ONLY
# (`.devcontainer/toolchain`), never the repo root: this image bakes in
# *tools*, not repository *source* (source is bind-mounted at
# /workspace at run time -- see .devcontainer/compose.tools.yml), so there is nothing
# here that needs a large build context or a .dockerignore.
#
# Non-root by design (a fixed `tools` user, uid/gid 1000). At run time
# .devcontainer/compose.tools.yml overrides the effective user to the *invoking host
# user's* uid:gid (not this image's fixed 1000:1000), so that files
# created under the /workspace bind-mount are owned by the calling
# developer/CI user rather than by this image's baked-in uid. Every
# cache directory this image populates (GOMODCACHE / GOCACHE / bun's
# install+cache dir / the go-installed CLI binaries' own directory) is
# therefore made world-writable below, so an arbitrary uid:gid at run
# time can still write new cache entries into it.
#
# GOMODCACHE/GOCACHE below are deliberately the SAME paths
# .devcontainer/compose.tools.yml mounts named volumes over (/cache/go/mod,
# /cache/go/build). Docker copies a directory's existing image content
# into a brand-new named volume the first time that volume is mounted
# (documented Docker volume behaviour, not specific to this Dockerfile) --
# so the module/build cache warmed here by `go install`-ing
# sqlc/goose/swag/goimports at their exact pinned versions survives into
# the (initially empty) `gomodcache`/`gobuild` named volumes on their
# very first use. This only pre-warms the cache for THESE four tools'
# own module graphs; each Go app stack's own go.sum-declared runtime
# dependencies (pgx, goose-as-a-library in app/migrator, etc.) are a
# separate concern and still need one network-enabled `go mod download`
# (via the `tools` service) before *that* module can build/test offline
# too -- see .devcontainer/compose.tools.yml's header comment and this directory's
# README.md "検証" section for how that two-phase flow is exercised.
#
# IMPORTANT (verified during Phase A implementation, corrects the
# original SPEC-009 plan's assumption): `go run pkg@version` itself does
# NOT run offline even with the exact module+version fully warmed in
# GOMODCACHE -- it performs a "loading deprecation for <module>" lookup
# that unconditionally needs the module proxy, and fails ("module lookup
# disabled by GOPROXY=off") regardless of cache state. `go install`
# below does not hit this (it is a one-time build, not `go run`'s
# per-invocation resolution path), so this Dockerfile still uses it to
# pre-warm the cache, but each stack's Makefile (Phase B) MUST invoke
# these tools by their installed binary name (`sqlc`, `goose`, `swag`,
# `goimports` -- all already on PATH via GOBIN=/usr/local/go-tools/bin
# below, present in every image regardless of volume state), never via
# `go run pkg@version`, for the exec phase to actually work under
# `tools-offline`. This is the SPEC-009 plan's own pre-approved fallback
# for exactly this scenario ("万一 offline 実行できない場合の代替": switch
# from `go run` to the installed binary) -- confirmed necessary here,
# not merely a hypothetical risk.

ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-bookworm

# ARGs declared before FROM only survive for the FROM instruction itself;
# re-declare everything else consumed below.
ARG BUN_VERSION
ARG GOLANGCI_LINT_VERSION
ARG TERRAFORM_VERSION
ARG TFLINT_VERSION
ARG TRIVY_VERSION
ARG SQLC_VERSION
ARG GOOSE_VERSION
ARG SWAG_VERSION
ARG GOIMPORTS_VERSION
# NOTE: deliberately NOT relying on BuildKit's automatic `ARG
# TARGETARCH` here -- it is only populated by the BuildKit-based
# builder (`docker buildx build`, or a compose v2 `docker compose build`
# that itself shells out to buildx); the legacy `docker-compose`
# (v1-compatible) binary invokes the classic non-BuildKit `docker
# build`, under which TARGETARCH is silently empty. Computed instead
# below via `dpkg --print-architecture`, which reports the exact same
# "amd64"/"arm64" strings Docker's TARGETARCH would (Debian's dpkg arch
# names happen to match), regardless of which builder is in use.

# Fail fast (before any network calls below) if .devcontainer/compose.tools.yml /
# versions.env did not actually supply a version -- an unset ARG would
# otherwise silently expand to an empty string in a URL and produce a
# confusing 404 several layers down.
RUN set -eu; \
    for v in "$BUN_VERSION" "$GOLANGCI_LINT_VERSION" "$TERRAFORM_VERSION" \
             "$TFLINT_VERSION" "$TRIVY_VERSION" "$SQLC_VERSION" \
             "$GOOSE_VERSION" "$SWAG_VERSION" "$GOIMPORTS_VERSION"; do \
        if [ -z "$v" ]; then echo ".devcontainer/toolchain/Dockerfile: a required ARG is empty" >&2; exit 1; fi; \
    done

# --- OS packages -------------------------------------------------------
# golang:*-bookworm already ships gcc/git (needed for cgo and for `go
# install`'s VCS stamping); add only what it is missing: make (every
# stack's Makefile), unzip (terraform/tflint release .zip assets), jq
# (kept for interactive debugging of installer JSON responses, not used
# by any RUN step below).
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        make \
        unzip \
        jq \
    && rm -rf /var/lib/apt/lists/*

# --- non-root user -------------------------------------------------------
# Fixed uid/gid 1000 as the image's own sane default (e.g. a plain
# `docker run` with no `user:` override, or the devcontainer's
# remoteUser). .devcontainer/compose.tools.yml overrides this at run time to the
# invoking host user's uid:gid instead -- see this file's header comment.
RUN groupadd --gid 1000 tools \
    && useradd --uid 1000 --gid tools --create-home --shell /bin/bash tools
ENV HOME=/home/tools

# --- cache / tool directories, shared with .devcontainer/compose.tools.yml ------------
# These paths are exactly what .devcontainer/compose.tools.yml mounts named volumes
# over (gomodcache/gobuild/buncache/tfplugincache) -- see this file's
# header comment for why baking content in here still matters despite
# the volume mount. GOBIN is deliberately NOT one of those paths, so the
# sqlc/goose/swag/goimports binaries installed below are always present
# regardless of volume state.
ENV GOPATH=/cache/go \
    GOMODCACHE=/cache/go/mod \
    GOCACHE=/cache/go/build \
    GOBIN=/usr/local/go-tools/bin \
    BUN_INSTALL=/usr/local/share/bun \
    BUN_INSTALL_CACHE_DIR=/cache/bun \
    TF_PLUGIN_CACHE_DIR=/cache/tf-plugins \
    PATH=/usr/local/go-tools/bin:/usr/local/bin:${PATH}

RUN mkdir -p /cache/go/mod /cache/go/build /cache/bun /cache/tf-plugins \
        /usr/local/go-tools/bin "$BUN_INSTALL" "$HOME" /workspace \
    # World-writable: an arbitrary uid:gid (.devcontainer/compose.tools.yml's `user:`
    # override, see above) must be able to write new cache entries at
    # run time, not just read what was baked in here at build time.
    && chmod -R 0777 /cache /usr/local/go-tools "$BUN_INSTALL" "$HOME" /workspace

# --- bun -------------------------------------------------------------------
# Official install script (the same one oven-sh/setup-bun@v2 uses
# internally), pinned to an exact release tag ("bun-v<version>").
# Installs under BUN_INSTALL above; symlinked onto PATH so both root
# (this build) and the run-time uid:gid override can invoke `bun`
# directly without sourcing a shell profile.
RUN curl -fsSL https://bun.sh/install | bash -s "bun-v${BUN_VERSION}" \
    && ln -sf "${BUN_INSTALL}/bin/bun" /usr/local/bin/bun \
    && chmod -R 0777 "$BUN_INSTALL" \
    && bun --version

# --- golangci-lint ---------------------------------------------------------
# Same install method as the CI jobs this image replaces
# (.github/workflows/cicd.yml's pre-SPEC-009 "Install golangci-lint" step).
RUN curl -sSfL "https://raw.githubusercontent.com/golangci/golangci-lint/v${GOLANGCI_LINT_VERSION}/install.sh" \
        | sh -s -- -b /usr/local/bin "v${GOLANGCI_LINT_VERSION}" \
    && golangci-lint version

# --- terraform ---------------------------------------------------------
# Release .zip + published SHA256SUMS, verified before unzipping (the
# GitHub Actions equivalent, hashicorp/setup-terraform, does the same
# checksum verification internally -- reimplemented by hand here since
# GitHub Actions themselves cannot run inside a Dockerfile RUN step).
RUN set -eu; \
    arch="$(dpkg --print-architecture)"; \
    tf_zip="terraform_${TERRAFORM_VERSION}_linux_${arch}.zip"; \
    curl -fsSLo "/tmp/${tf_zip}" \
        "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/${tf_zip}"; \
    curl -fsSLo /tmp/terraform_SHA256SUMS \
        "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_SHA256SUMS"; \
    (cd /tmp && grep " ${tf_zip}\$" terraform_SHA256SUMS | sha256sum -c -); \
    unzip -q -o "/tmp/${tf_zip}" -d /usr/local/bin; \
    rm -f "/tmp/${tf_zip}" /tmp/terraform_SHA256SUMS; \
    chmod +x /usr/local/bin/terraform; \
    terraform version

# --- tflint ------------------------------------------------------------
# Release .zip + published checksums.txt, verified before unzipping (the
# GitHub Actions equivalent, terraform-linters/setup-tflint, does the
# same internally).
RUN set -eu; \
    arch="$(dpkg --print-architecture)"; \
    tflint_zip="tflint_linux_${arch}.zip"; \
    curl -fsSLo "/tmp/${tflint_zip}" \
        "https://github.com/terraform-linters/tflint/releases/download/${TFLINT_VERSION}/${tflint_zip}"; \
    curl -fsSLo /tmp/tflint_checksums.txt \
        "https://github.com/terraform-linters/tflint/releases/download/${TFLINT_VERSION}/checksums.txt"; \
    (cd /tmp && grep " ${tflint_zip}\$" tflint_checksums.txt | sha256sum -c -); \
    unzip -q -o "/tmp/${tflint_zip}" -d /usr/local/bin; \
    rm -f "/tmp/${tflint_zip}" /tmp/tflint_checksums.txt; \
    chmod +x /usr/local/bin/tflint; \
    tflint --version

# --- trivy ---------------------------------------------------------------
# Official install script (the same one aquasecurity/setup-trivy uses
# internally); it downloads its own checksums.txt and verifies the
# tarball's sha256 before extracting (see the script itself --
# hash_sha256_verify -- for the mechanism), so no separate verification
# step is needed here.
RUN curl -fsSL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh \
        | sh -s -- -b /usr/local/bin "${TRIVY_VERSION}" \
    && trivy --version

# --- go run <pkg>@<version> CLIs (sqlc / goose / swag / goimports) -------
# `go install` (not `go run`) here so this build (a) has an actual
# binary to smoke-test below, and (b) warms GOMODCACHE/GOCACHE with
# these exact module graphs at these exact pinned versions. GOBIN above
# is NOT one of the paths .devcontainer/compose.tools.yml mounts a volume over, so
# these installed binaries are always present regardless of volume
# state -- and, per this file's header comment, each stack's Makefile
# (Phase B) needs to invoke these binaries BY NAME (`sqlc` / `goose` /
# `swag` / `goimports`, already on PATH here), not via `go run
# pkg@version`, since the latter fails offline even when fully cached.
#
# sqlc's own go.mod requires `go >= 1.26.0`, matching this image's
# pinned GO_VERSION (versions.env). No GOTOOLCHAIN override is needed.
#
# goimports has no --version/-version flag (and running it with no
# arguments blocks reading stdin), so it is verified below by checking
# the installed binary exists/is executable rather than invoking it.
RUN go install "github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION}" \
    && go install "github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION}" \
    && go install "github.com/swaggo/swag/v2/cmd/swag@${SWAG_VERSION}" \
    && go install "golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION}" \
    && chmod -R 0777 /cache \
    && sqlc version \
    && goose -version \
    && swag --version \
    && test -x "${GOBIN}/goimports"

WORKDIR /workspace
USER tools

# Smoke-testable expectation (.devcontainer/toolchain/README.md "検証"):
# `go version` / `bun --version` / `golangci-lint version` /
# `terraform version` / `tflint --version` / `trivy --version` /
# `sqlc version` / `goose -version` / `swag --version` should all report
# exactly the versions ARG'd in above.
