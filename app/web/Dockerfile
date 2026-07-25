# syntax=docker/dockerfile:1

# ---- Build stage -----------------------------------------------------------
# SPEC-016: pinned to an exact bun version *and* its immutable digest, closing
# two gaps at once -- `oven/bun:1` floated the whole major (so the version that
# built an image was whatever the tag pointed at that day, and it drifted from
# versions.env's BUN_VERSION=1.3.14 used by the toolchain), and a tag alone is
# re-pushable. Multi-arch OCI index digest, so arm64/amd64 both resolve.
# Refresh version + digest together, keeping the version aligned with
# versions.env's BUN_VERSION:
#   docker pull oven/bun:<version>
#   docker inspect --format '{{index .RepoDigests 0}}' oven/bun:<version>
FROM oven/bun:1.3.14@sha256:e10577f0db68676a7024391c6e5cb4b879ebd17188ab750cf10024a6d700e5c4 AS build
WORKDIR /app

# Install dependencies first (cache layer): only files that affect resolution.
# bunfig.toml belongs to that set, not to the source COPY below: it carries
# this repo's supply-chain gates (.claude/rules/web.md「依存インストール(サプ
# ライチェーン対策)」, ISSUE-020) -- the pinned internal registry and the
# `minimumReleaseAge` = 21 days quarantine against Shai-Hulud-class npm worms,
# which publish a poisoned version and rely on installs picking it up
# immediately. Without bunfig.toml present *before* `bun install` runs, this
# image build is the one install path in the repo that resolves against the
# default public registry with no release-age gate.
COPY package.json bun.lock bunfig.toml ./

# --ignore-scripts: no dependency lifecycle script (postinstall & co.) executes
# during the image build. Those scripts are the actual infection/exfiltration
# vector of that worm class, and this build needs none of them -- every native
# dependency here (Tailwind's oxide binary, esbuild/Vite) ships prebuilt
# platform binaries as optionalDependencies. Bun already limits lifecycle
# scripts to trusted packages, so this closes the remaining default-trusted set
# as well. --frozen-lockfile keeps resolution pinned to bun.lock (integrity
# hashes enforced, no version drift at build time).
RUN bun install --frozen-lockfile --ignore-scripts

# Now copy the rest of the source and build. .dockerignore keeps the host's
# node_modules/ and dist/ out of the build context, so this COPY cannot
# overwrite the dependency tree the gated install above just produced with
# whatever happens to sit in the working copy.
COPY . .
RUN bun run build

# ---- Runtime stage ----------------------------------------------------------
# nginx-unprivileged runs the master process as a non-root user (uid 101) and
# ships a main nginx.conf already hardened for that (listen 8080, pid in
# /tmp, writable temp paths) — see ISSUE-056. Only the server block
# (conf.d/default.conf) is supplied by this repo.
#
# Digest-pinned as well (SPEC-016): `:alpine` is a rolling tag on this image,
# so the digest is what actually identifies what is serving the public
# surface. Multi-arch index digest. Refresh with:
#   docker pull nginxinc/nginx-unprivileged:alpine
#   docker inspect --format '{{index .RepoDigests 0}}' nginxinc/nginx-unprivileged:alpine
FROM nginxinc/nginx-unprivileged:alpine@sha256:18d67281256ded39ff65e010ae4f831be18f19356f83c60bc546492c7eb6dd23 AS runtime

COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD ["wget", "-q", "-O", "-", "http://127.0.0.1:8080/"]

CMD ["nginx", "-g", "daemon off;"]
