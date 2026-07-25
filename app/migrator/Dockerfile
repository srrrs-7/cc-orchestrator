# app/migrator: SPEC-005's shared migration runner for app/api and
# app/auth (2026-07-09 refactor, plan §RF.1.2/§RF.6.1 RF-b). One image
# bundles both stacks' schema migrations; the caller selects which to apply
# at run time via -target api|auth (see main.go). This replaces the
# prior per-stack app/{api,auth}/Dockerfile.migrate + db/migrate-
# entrypoint.sh (removed by this refactor), which needed a shell + psql
# to CREATE SCHEMA before invoking goose -- this binary does its own
# database bootstrap (ensureDatabase, database.go) via database/sql, so
# no psql/shell is required at all.
#
# Build context is the *repository root*, not app/migrator, because this
# Dockerfile also COPYs app/api/infra/postgres/schema/migrations and
# app/auth/infra/postgres/schema/migrations,
# which live outside app/migrator's own directory tree:
#
#   docker build -f app/migrator/Dockerfile .
#
# --- build stage -------------------------------------------------------
# Go 1.26 on Alpine (musl). CGO_ENABLED=0 produces a fully static
# binary, so glibc/gcc from the full golang:1.26 image are not needed;
# the binary still runs unmodified on the distroless runtime stage
# below (same pattern as app/api/Dockerfile and app/auth/Dockerfile).
#
# SPEC-016: digest-pinned (multi-arch OCI index, so linux/arm64 for ECS still
# resolves); the tag is a label, the digest is the identity. Refresh both with
# `docker pull golang:1.26-alpine && docker inspect --format
# '{{index .RepoDigests 0}}' golang:1.26-alpine` -- keep it identical to
# app/{api,auth}/Dockerfile's own pin.
FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS build
WORKDIR /src

# Copy only the dependency manifest first so `go mod download` is
# cached across builds that only change application source.
COPY app/migrator/go.mod app/migrator/go.sum ./
RUN go mod download

COPY app/migrator/ .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -trimpath -o /out/migrator ./cmd/migrator

# --- runtime stage -------------------------------------------------------
# distroless/static: no shell, no package manager, non-root user by
# default. This image runs as a short-lived task (an ECS init
# container or a one-off local/CI invocation), not a public-facing
# daemon, but still drops to a non-root user and carries no debugging
# shell. Digest-pinned (SPEC-016), identical to app/{api,auth}/Dockerfile's.
#
# This is also the image root `make migrate` / `make migrate-test` now run
# locally (compose.yml's profile-gated `migrator` service) -- so local, CI and
# the ECS migration init container all apply migrations with one artifact
# instead of a `go run` inside the toolchain container.
FROM gcr.io/distroless/static:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6 AS runtime

COPY --from=build /out/migrator /migrator

# Both stacks' migration SQL is baked into this one shared image
# (RF-b: a single ECR repository/image serves api and auth alike,
# selected at run time by -target's default migrations-dir,
# "/migrations/<target>" -- see main.go's parseFlags).
COPY app/api/infra/postgres/schema/migrations /migrations/api
COPY app/auth/infra/postgres/schema/migrations /migrations/auth

USER nonroot:nonroot

# No default CMD: -target is required (main.go's parseFlags rejects an
# empty/unrecognized value), so a `docker run` with no command fails
# fast instead of silently defaulting to one stack. The runner (ECS
# task definition's `command`, a Makefile target, or a CI step) always
# supplies at least ["-target", "api"] or ["-target", "auth"].
ENTRYPOINT ["/migrator"]
