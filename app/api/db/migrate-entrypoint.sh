#!/bin/sh
# SPEC-005 R5/prod: creates this stack's target schema before running
# goose (app/iac/modules/service/README.md "スキーマブートストラップをどこで
# やるか" -- goose's own version table, goose_db_version, must live
# inside a schema that already exists, and that schema can only be
# created here, inside the migrate init container, not by goose itself
# or by Terraform against a private-subnet-only RDS instance).
#
# Connection parameters (including DB_PASSWORD) are passed to both
# psql and goose exclusively via the standard libpq PG* environment
# variables below, never interpolated into a hand-built DSN/URL
# string. This avoids the percent-encoding asymmetry a
# postgres://user:password@... string built by shell interpolation
# would have against infra/postgres/db.go's net/url-based DSN (SPEC-005
# review E4, security m-1): a master password containing a URL-reserved
# character (@, :, /, ?, #) would otherwise misparse the host/path
# here without erroring, silently pointing goose at the wrong
# host/database instead of failing the deploy. goose's postgres driver
# is jackc/pgx/v5/stdlib (same as infra/postgres/db.go), which -- like
# libpq -- fills in any connection setting not present in the dbstring
# argument from these PG* env vars, so passing only "search_path=..."
# as goose's dbstring is sufficient (verified locally: a schema-less
# database + a master password containing every URL-reserved character
# -- built and ran this Dockerfile.migrate image end to end via
# `docker build`/`docker run` before committing this script).
set -eu

: "${DB_HOST:?DB_HOST is required}"
: "${DB_PORT:?DB_PORT is required}"
: "${DB_NAME:?DB_NAME is required}"
: "${DB_USER:?DB_USER is required}"
: "${DB_PASSWORD:?DB_PASSWORD is required}"
: "${DB_SSLMODE:?DB_SSLMODE is required}"
: "${DB_SCHEMA:?DB_SCHEMA is required}"

export PGHOST="$DB_HOST"
export PGPORT="$DB_PORT"
export PGDATABASE="$DB_NAME"
export PGUSER="$DB_USER"
export PGPASSWORD="$DB_PASSWORD"
export PGSSLMODE="$DB_SSLMODE"

# -X: ignore any stray ~/.psqlrc. ON_ERROR_STOP: a failed CREATE SCHEMA
# (e.g. insufficient privilege) must fail this script (set -eu alone
# does not stop psql -c on a SQL-level error).
psql -X -v ON_ERROR_STOP=1 -c "CREATE SCHEMA IF NOT EXISTS \"$DB_SCHEMA\""

# "$@" defaults to the image's CMD (["up"]) but can be overridden by the
# runner (e.g. an ECS task definition's `command` override) with any
# other goose subcommand (status/down/...).
exec goose -dir /migrations postgres "search_path=$DB_SCHEMA" "$@"
