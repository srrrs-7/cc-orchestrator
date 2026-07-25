# --- build stage -----------------------------------------------------------
# Go 1.26 on Alpine (musl). The module has no external requires and
# CGO_ENABLED=0 produces a fully static binary, so glibc/gcc from the
# full golang:1.26 image are not needed; the binary still runs unmodified
# on the distroless runtime stage below.
#
# SPEC-016: pinned by immutable digest, not by tag alone -- `1.26-alpine` is
# re-pushed on every patch/base rebuild, so the tag is a human-readable label
# and the digest is the identity Docker actually resolves. It is a multi-arch
# OCI *index* digest, so linux/arm64 (ECS, `make push-images`) and amd64 both
# still resolve from this one pin. Refresh tag + digest together:
#   docker pull golang:1.26-alpine
#   docker inspect --format '{{index .RepoDigests 0}}' golang:1.26-alpine
FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS build
WORKDIR /src

# Copy only the dependency manifest first so the `go mod download` layer is
# cached across builds that only change application source. go.sum may not
# exist (stdlib-only module); the glob keeps COPY from failing in that case.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -trimpath -o /out/ ./cmd/authz ./cmd/healthcheck

# --- runtime stage -----------------------------------------------------------
# distroless/static: no shell, no package manager, non-root user by default.
# Digest-pinned for the same reason as the build stage above (SPEC-016);
# refresh with `docker pull` + `docker inspect --format '{{index .RepoDigests 0}}'`.
FROM gcr.io/distroless/static:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6 AS runtime

COPY --from=build /out/authz /authz
COPY --from=build /out/healthcheck /healthcheck

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/healthcheck", "http://localhost:8080/.well-known/openid-configuration"]

USER nonroot:nonroot

ENTRYPOINT ["/authz"]
