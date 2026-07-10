# --- build stage -----------------------------------------------------------
# Go 1.24 on Alpine (musl). The module has no external requires and
# CGO_ENABLED=0 produces a fully static binary, so glibc/gcc from the
# full golang:1.24 image are not needed; the binary still runs unmodified
# on the distroless runtime stage below.
FROM golang:1.24-alpine AS build
WORKDIR /src

# Copy only the dependency manifest first so the `go mod download` layer is
# cached across builds that only change application source. go.sum may not
# exist (stdlib-only module); the glob keeps COPY from failing in that case.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -trimpath -o /out/ ./cmd/api ./cmd/healthcheck

# --- runtime stage -----------------------------------------------------------
# distroless/static: no shell, no package manager, non-root user by default.
FROM gcr.io/distroless/static:nonroot AS runtime

COPY --from=build /out/api /api
COPY --from=build /out/healthcheck /healthcheck

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/healthcheck", "http://localhost:8080/tasks"]

USER nonroot:nonroot

ENTRYPOINT ["/api"]
