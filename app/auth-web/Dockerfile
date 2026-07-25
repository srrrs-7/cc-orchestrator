# syntax=docker/dockerfile:1

# SPEC-016: exact bun version + immutable digest instead of the floating
# `oven/bun:1` major tag (kept aligned with versions.env's BUN_VERSION and with
# app/web/Dockerfile's own pin -- see that file for the refresh command).
FROM oven/bun:1.3.14@sha256:e10577f0db68676a7024391c6e5cb4b879ebd17188ab750cf10024a6d700e5c4 AS build
WORKDIR /app

# bunfig.toml is copied *with* the manifest, not with the source below: it
# carries this repo's supply-chain gates (.claude/rules/web.md「依存インストール
# (サプライチェーン対策)」, ISSUE-020) -- pinned internal registry plus the
# `minimumReleaseAge` = 21 days quarantine against Shai-Hulud-class npm worms.
# Without it in place before `bun install`, this image build would resolve
# against the default public registry with no release-age gate. Same shape as
# app/web/Dockerfile's build stage (see its fuller comments).
COPY package.json bun.lock bunfig.toml ./

# --ignore-scripts: no dependency lifecycle script (postinstall & co.) runs
# during the image build -- that is the infection vector of that worm class,
# and nothing here needs one (native deps ship prebuilt binaries via
# optionalDependencies). --frozen-lockfile pins resolution to bun.lock.
RUN bun install --frozen-lockfile --ignore-scripts

# .dockerignore (this directory) keeps the host's node_modules/ and dist/ out
# of the build context, so this COPY cannot overwrite the gated install above.
COPY . .
RUN bun run build

# nginx-unprivileged runs the master process as a non-root user (uid 101) and
# ships a main nginx.conf already hardened for that (listen 8080, pid in
# /tmp, writable temp paths) — see ISSUE-056. Only the server block
# (conf.d/default.conf) is supplied by this repo. Digest-pinned (SPEC-016),
# identical to app/web/Dockerfile's pin -- `:alpine` is a rolling tag.
FROM nginxinc/nginx-unprivileged:alpine@sha256:18d67281256ded39ff65e010ae4f831be18f19356f83c60bc546492c7eb6dd23 AS runtime

COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD ["wget", "-q", "-O", "-", "http://127.0.0.1:8080/"]

CMD ["nginx", "-g", "daemon off;"]
