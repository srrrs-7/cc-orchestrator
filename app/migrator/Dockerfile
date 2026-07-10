# app/migrator: SPEC-005's shared migration runner for app/api and
# app/auth (2026-07-09 refactor, plan §RF.1.2/§RF.6.1 RF-b). One image
# bundles both stacks' db/migrations; the caller selects which to apply
# at run time via -target api|auth (see main.go). This replaces the
# prior per-stack app/{api,auth}/Dockerfile.migrate + db/migrate-
# entrypoint.sh (removed by this refactor), which needed a shell + psql
# to CREATE SCHEMA before invoking goose -- this binary does its own
# database bootstrap (ensureDatabase, database.go) via database/sql, so
# no psql/shell is required at all.
#
# Build context is the *repository root*, not app/migrator, because this
# Dockerfile also COPYs app/api/db/migrations and app/auth/db/migrations,
# which live outside app/migrator's own directory tree:
#
#   docker build -f app/migrator/Dockerfile .
#
# --- build stage -------------------------------------------------------
# Go 1.26 on Alpine (musl). CGO_ENABLED=0 produces a fully static
# binary, so glibc/gcc from the full golang:1.26 image are not needed;
# the binary still runs unmodified on the distroless runtime stage
# below (same pattern as app/api/Dockerfile and app/auth/Dockerfile).
FROM golang:1.26-alpine AS build
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
# shell.
FROM gcr.io/distroless/static:nonroot AS runtime

COPY --from=build /out/migrator /migrator

# Both stacks' migration SQL is baked into this one shared image
# (RF-b: a single ECR repository/image serves api and auth alike,
# selected at run time by -target's default migrations-dir,
# "/migrations/<target>" -- see main.go's parseFlags).
COPY app/api/db/migrations /migrations/api
COPY app/auth/db/migrations /migrations/auth

USER nonroot:nonroot

# No default CMD: -target is required (main.go's parseFlags rejects an
# empty/unrecognized value), so a `docker run` with no command fails
# fast instead of silently defaulting to one stack. The runner (ECS
# task definition's `command`, a Makefile target, or a CI step) always
# supplies at least ["-target", "api"] or ["-target", "auth"].
ENTRYPOINT ["/migrator"]
