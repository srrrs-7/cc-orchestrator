-- +goose Up
-- SPEC-005 R2/R3: clients / users / authorization_codes back
-- domain/{client,user,authcode}.Repository. All three tables are
-- deliberately unqualified (no schema prefix) -- the "auth" schema is
-- selected purely via the connection's search_path (plan §0 "スキーマ
-- 分離機構"), so this file is byte-identical regardless of which
-- schema it ends up applied to. Schema creation itself happens outside
-- goose (local: docker/postgres/initdb, prod: iac bootstrap).

-- clients backs domain/client.Repository (FindByID only; read-only
-- port -- rows are populated by the startup idempotent seed, see
-- infra/postgres/seed.go's SeedClient / db/queries/clients.sql's
-- UpsertClient). Client's four multi-valued attributes
-- (redirect_uris / allowed_scopes / response_types / grant_types) are
-- stored as jsonb arrays of strings and round-tripped via
-- encoding/json in infra/postgres/client_repository.go (plan §1.2:
-- jsonb chosen over text[] for stable database/sql + pgx stdlib
-- scanning).
CREATE TABLE clients (
    id             text  PRIMARY KEY,
    redirect_uris  jsonb NOT NULL,
    allowed_scopes jsonb NOT NULL,
    response_types jsonb NOT NULL,
    grant_types    jsonb NOT NULL
);

-- users backs domain/user.Repository (FindByID / FindByUsername;
-- read-only port -- rows are populated by the startup idempotent
-- seed, see infra/postgres/seed.go's SeedUser). username is UNIQUE so
-- FindByUsername can rely on at most one match, matching
-- infra/memory.UserRepository's map-keyed-by-id-with-linear-username-
-- scan behavior (which also assumes uniqueness by construction of
-- Seed). password is stored as-is (plaintext): this mirrors the
-- existing domain/user.User design (see user.go's VerifyPassword doc
-- comment) and is a deliberate, documented scope limit of this Spec
-- (plan §6.1 R-b) -- hashing is left to a future Issue.
CREATE TABLE users (
    id            text PRIMARY KEY,
    username      text NOT NULL UNIQUE,
    password      text NOT NULL,
    profile_name  text NOT NULL,
    profile_email text NOT NULL
);

-- authorization_codes backs domain/authcode.Repository. Unlike
-- clients/users, this port is read-write (Save / FindByCode /
-- Consume) and is the one aggregate whose persistence directly closes
-- SPEC-005's motivating gap (an in-memory, single-instance authcode
-- store cannot support more than one authz server instance).
--
-- consumed defaults to false and is set only by callers that read a
-- row back before it is deleted (see db/queries/authcodes.sql's
-- GetActiveAuthCode); it is not the single-use enforcement mechanism.
-- Consume (db/queries/authcodes.sql's ConsumeAuthCode) is
-- delete-based: a successful redemption removes the row outright, the
-- same "delete, don't flag" contract infra/memory.AuthCodeRepository
-- implements (plan §0 "authcode 単回使用/TTL"), so a repeated /token
-- call for an already-redeemed code finds no row at all rather than a
-- row with consumed=true.
--
-- nonce is nullable: domain/authcode.Nonce's zero value ("") is a
-- valid "no nonce was requested" state (see nonce.go's IsEmpty),
-- stored here as SQL NULL rather than an empty string so the sqlc
-- nullable-column scan type (sql.NullString) round-trips it
-- unambiguously.
--
-- expires_at is timestamptz (not timestamp) so TTL comparisons against
-- now() are unambiguous with respect to UTC offset, matching Go's
-- time.Time semantics (same rationale as tasks.created_at/updated_at
-- in app/api's migration).
CREATE TABLE authorization_codes (
    code             text        PRIMARY KEY,
    client_id        text        NOT NULL,
    user_id          text        NOT NULL,
    redirect_uri     text        NOT NULL,
    scope            text        NOT NULL,
    nonce            text,
    challenge        text        NOT NULL,
    challenge_method text        NOT NULL,
    expires_at       timestamptz NOT NULL,
    consumed         boolean     NOT NULL DEFAULT false,
    created_at       timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE authorization_codes;
DROP TABLE users;
DROP TABLE clients;
