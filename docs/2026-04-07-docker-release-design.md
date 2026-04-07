# Docker and Release Flow Design

## Overview

Flow needs a Dockerfile and GitHub Actions release pipeline, following the
same pattern Sharkfin uses: multi-stage Docker builds, GitHub Container
Registry (ghcr.io), and automated versioning from conventional commits.

## Dockerfile

Multi-stage build, same as Sharkfin:

**Build stage** (alpine:3.21):
- Install mise for reproducible Go toolchain
- Copy `mise.toml` and `.mise/` first (layer cache)
- Download Go dependencies in isolated layer
- Build with `mise run build:release` (static binary, stripped symbols)
- Accept `VERSION` build arg

**Runtime stage** (alpine:3.21):
- CA certificates only
- Copy binary from build stage to `/usr/local/bin/flow`
- Entrypoint: `flow daemon`

Note: Flow uses `modernc.org/sqlite` (pure Go, no CGo), so `CGO_ENABLED=0`
static builds work without any C toolchain in the container.

## Mise Build Tasks

Add `.mise/tasks/` directory (Sharkfin pattern) with:

- `build/dev` — dynamic binary with dev version tag
- `build/release` — static, stripped binary (`CGO_ENABLED=0`,
  `-ldflags "-s -w"`, `-trimpath`)
- `ci` — aggregated lint + test

## GitHub Actions Workflows

### CI (`ci.yaml`)

Trigger: PR and pushes to master.

Jobs:
- `test` — `mise run ci` (vet, test)

### Release (`release.yaml`)

Trigger: pushes to master.

Jobs:
1. **test** — lint + unit tests
2. **tag** — auto-bump semver from conventional commits using
   `github-tag-action`. Creates `vX.Y.Z` tags. Only proceeds if new
   tag created.
3. **build** — matrix builds for:
   - linux-amd64, linux-arm64
   - darwin-amd64, darwin-arm64
   - Static binaries, compressed with xz
   - SHA256 checksums
4. **release** — create GitHub Release with artifacts and release notes

### Container Release (`release-container.yaml`)

Trigger: GitHub release published (manual, after auto-tag).

Jobs:
1. Login to ghcr.io with `GITHUB_TOKEN`
2. Build with `docker/build-push-action@v7`
3. Multi-platform: `linux/amd64,linux/arm64`
4. Tags: `ghcr.io/work-fort/flow:vX.Y.Z` + `ghcr.io/work-fort/flow:latest`
5. VERSION build arg from `github.ref_name`

## .dockerignore

```
build/
.git/
.worktrees/
docs/
```

## Versioning

- Conventional commits drive semver bumps via `github-tag-action`
- `feat:` → minor bump, `fix:` → patch bump
- Tags use `v` prefix: `v1.0.0`
- Container images and binary releases share the same version

## What This Does NOT Include

- docker-compose (no need — Flow is a single binary with SQLite)
- Windows builds (not needed for a server daemon)
- AUR package (can add later if needed)
- PostgreSQL e2e tests in CI (Flow only has SQLite currently)
