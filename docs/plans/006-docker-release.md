---
type: plan
step: "6"
status: pending
codebase: flow
---

# Flow Phase 6 — Docker and Release Pipeline

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Dockerfile, mise build tasks, `.dockerignore`, and GitHub Actions workflows for CI, release, and container publishing. Follows the Sharkfin pattern exactly, adapted for Flow.

**Critical blocker — replace directives:** Flow's `go.mod` has three `replace` directives pointing to local filesystem paths:

```
replace github.com/Work-Fort/Hive => /home/kazw/Work/WorkFort/hive/lead
replace github.com/Work-Fort/sharkfin/client/go => /home/kazw/Work/WorkFort/sharkfin/lead/client/go
replace github.com/Work-Fort/Pylon/client/go => /home/kazw/Work/WorkFort/pylon/lead/client/go
```

These will fail in CI and Docker builds. The chosen strategy is **multi-repo checkout + `go mod edit`**: each GitHub Actions workflow checks out the dependent repos alongside Flow, rewrites the replace directives to relative paths with `go mod edit`, then runs `go mod tidy` to refresh `go.sum` before any build step. The Docker build copies dependencies into `vendor-local/` within the build context and applies the same `go mod edit` + `go mod tidy` sequence during the build stage.

**Sharkfin note:** Sharkfin's `go.mod` has no replace directives — it only depends on published modules. This strategy is Flow-specific.

**Assumption:** `sharkfin/client/go` and `pylon/client/go` each contain their own `go.mod` file, making them independent Go modules that can be used as replace targets. This is required for the replace strategy to work and is assumed to be true.

---

## Chunk 1: Mise Build Tasks

### Task 1: Create `.mise/tasks/build/dev`

**Files:** `.mise/tasks/build/dev` (new)

- [ ] **Step 1: Create `.mise/tasks/build/dev`**

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
#MISE description="Build flow (dev, dynamic)"
#MISE sources=["**/*.go", "**/*.sql", "go.mod", "go.sum"]
#MISE outputs=["build/flow"]
set -euo pipefail

BUILD_DIR=build
BINARY_NAME=flow
GIT_SHORT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION="${VERSION:-dev-$GIT_SHORT_SHA}"

mkdir -p "$BUILD_DIR"

go build -ldflags "-X github.com/Work-Fort/Flow/cmd.Version=$VERSION" -o "$BUILD_DIR/$BINARY_NAME"
echo "Built $BUILD_DIR/$BINARY_NAME"
```

- [ ] **Step 2: Make executable** — `chmod +x .mise/tasks/build/dev`

- [ ] **Step 3: Verify** — `mise run build:dev` exits 0 and produces `build/flow`.

- [ ] **Step 4: Commit** — `feat(build): add mise build:dev task`

---

### Task 2: Create `.mise/tasks/build/release`

**Files:** `.mise/tasks/build/release` (new)

- [ ] **Step 1: Create `.mise/tasks/build/release`**

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
#MISE description="Build optimized static release binary"
#MISE sources=["**/*.go", "**/*.sql", "go.mod", "go.sum"]
#MISE outputs=["build/flow"]
set -euo pipefail

BUILD_DIR=build
BINARY_NAME=flow
GIT_SHORT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION="${VERSION:-dev-$GIT_SHORT_SHA}"

mkdir -p "$BUILD_DIR"

CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/Work-Fort/Flow/cmd.Version=$VERSION" \
    -o "$BUILD_DIR/$BINARY_NAME"
echo "Built $BUILD_DIR/$BINARY_NAME (static, stripped)"
```

- [ ] **Step 2: Make executable** — `chmod +x .mise/tasks/build/release`

- [ ] **Step 3: Verify** — `mise run build:release` exits 0 and produces `build/flow`.

- [ ] **Step 4: Commit** — `feat(build): add mise build:release task`

---

### Task 3: Create `.mise/tasks/lint` and `.mise/tasks/test`

**Files:** `.mise/tasks/lint` (new), `.mise/tasks/test` (new)

- [ ] **Step 1: Create `.mise/tasks/lint`**

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
#MISE description="Run linters"
set -euo pipefail

UNFORMATTED=$(gofmt -l .)
if [ -n "$UNFORMATTED" ]; then
  echo "Unformatted files:"
  echo "$UNFORMATTED"
  exit 1
fi

go vet ./...
```

- [ ] **Step 2: Create `.mise/tasks/test`**

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
#MISE description="Run tests with coverage"
set -euo pipefail

BUILD_DIR=build
mkdir -p "$BUILD_DIR"

go test -v -race -coverprofile="$BUILD_DIR/coverage.out" ./...
```

- [ ] **Step 3: Make both executable** — `chmod +x .mise/tasks/lint .mise/tasks/test`

- [ ] **Step 4: Verify** — `mise run lint` and `mise run test` both exit 0.

- [ ] **Step 5: Commit** — `feat(build): add mise lint and test tasks`

---

### Task 4: Create `.mise/tasks/ci`

**Files:** `.mise/tasks/ci` (new)

- [ ] **Step 1: Create `.mise/tasks/ci`**

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
#MISE description="Run all CI checks"
#MISE depends=["lint", "test"]
set -euo pipefail
```

Note: No `e2e` dependency — Flow uses SQLite only, no separate e2e suite.

- [ ] **Step 2: Make executable** — `chmod +x .mise/tasks/ci`

- [ ] **Step 3: Verify** — `mise run ci` exits 0.

- [ ] **Step 4: Commit** — `feat(build): add mise ci task`

---

### Task 5: Create `.mise/tasks/docker/vendor-local`

**Files:** `.mise/tasks/docker/vendor-local` (new)

This task populates `vendor-local/` from the local sibling repos so developers can run `docker build .` locally without needing the CI environment. The paths match the local repo layout.

- [ ] **Step 1: Create `.mise/tasks/docker/vendor-local`**

```bash
#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
#MISE description="Populate vendor-local/ from sibling repos for local Docker builds"
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

rm -rf "$REPO_ROOT/vendor-local"
mkdir -p "$REPO_ROOT/vendor-local"

cp -r /home/kazw/Work/WorkFort/hive/lead "$REPO_ROOT/vendor-local/hive"
cp -r /home/kazw/Work/WorkFort/sharkfin/lead/client/go "$REPO_ROOT/vendor-local/sharkfin-client-go"
cp -r /home/kazw/Work/WorkFort/pylon/lead/client/go "$REPO_ROOT/vendor-local/pylon-client-go"

echo "vendor-local/ populated. Run: docker build ."
```

- [ ] **Step 2: Make executable** — `chmod +x .mise/tasks/docker/vendor-local`

- [ ] **Step 3: Verify** — `mise run docker:vendor-local` populates `vendor-local/` with the three dependency directories.

- [ ] **Step 4: Commit** — `feat(build): add mise docker:vendor-local task for local Docker builds`

---

### Task 6: Update `mise.toml`

**Files:** `mise.toml`

The current `mise.toml` has inline task definitions. Replace them with the `.mise/tasks/` directory pattern (Sharkfin style) — remove inline tasks and keep only the tools block.

- [ ] **Step 1: Update `mise.toml`**

```toml
# SPDX-License-Identifier: GPL-2.0-only
[tools]
go = "1.26.0"
```

- [ ] **Step 2: Verify** — `mise run build:dev` still works (tasks are discovered from `.mise/tasks/`).

- [ ] **Step 3: Commit** — `refactor(build): migrate mise tasks to .mise/tasks/ directory`

---

## Chunk 2: Dockerfile and .dockerignore

### Task 7: Create `.dockerignore`

**Files:** `.dockerignore` (new)

- [ ] **Step 1: Create `.dockerignore`**

```
build/
.git/
.worktrees/
docs/
```

- [ ] **Step 2: Verify** — file exists at repo root.

- [ ] **Step 3: Commit** — `feat(docker): add .dockerignore`

---

### Task 8: Create `Dockerfile`

**Files:** `Dockerfile` (new)

Flow uses `modernc.org/sqlite` (pure Go, no CGo), so `CGO_ENABLED=0` static builds work without a C toolchain.

The replace directives require the dependency source trees to be present during `go mod download` and build. The Dockerfile copies `vendor-local/` (pre-populated by CI or `mise run docker:vendor-local` locally) into the build stage, copies all source, then rewrites replace directives with `go mod edit`, runs `go mod tidy`, and downloads dependencies. `go mod tidy` requires the full source tree to determine imports, so the `go mod edit` + `go mod tidy` + `go mod download` block must come after `COPY . .` — dep-download layer caching is sacrificed for correctness.

- [ ] **Step 1: Create `Dockerfile`**

```dockerfile
# SPDX-License-Identifier: GPL-2.0-only

FROM alpine:3.21 AS build
RUN apk add --no-cache bash git \
    && apk add --no-cache --repository=https://dl-cdn.alpinelinux.org/alpine/edge/community mise
SHELL ["/bin/bash", "-c"]
WORKDIR /src

COPY mise.toml ./
COPY .mise/ .mise/
RUN mise trust && mise install

# Copy local dependency sources (populated by CI or `mise run docker:vendor-local`).
# These satisfy the replace directives in go.mod during the Docker build.
COPY vendor-local/ vendor-local/

COPY go.mod go.sum ./
COPY . .

# Rewrite replace directives to in-context paths. go mod tidy needs the full
# source tree to resolve imports, so this block must come after COPY . .
# Dep-download layer caching is lost, but the build is correct.
RUN eval "$(mise activate bash)" \
    && go mod edit \
        -replace github.com/Work-Fort/Hive=./vendor-local/hive \
        -replace "github.com/Work-Fort/sharkfin/client/go=./vendor-local/sharkfin-client-go" \
        -replace "github.com/Work-Fort/Pylon/client/go=./vendor-local/pylon-client-go" \
    && go mod tidy \
    && go mod download
ARG VERSION=dev
RUN eval "$(mise activate bash)" && VERSION=${VERSION} mise run build:release

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /src/build/flow /usr/local/bin/flow
ENTRYPOINT ["flow"]
CMD ["daemon"]
```

- [ ] **Step 2: Verify** — `mise run docker:vendor-local && docker build --no-cache .` completes successfully.

- [ ] **Step 3: Commit** — `feat(docker): add Dockerfile`

---

## Chunk 3: GitHub Actions Workflows

### Task 9: Create `.github/workflows/ci.yaml`

**Files:** `.github/workflows/ci.yaml` (new)

CI runs on PR and push to master. It checks out all three dependency repos alongside Flow, rewrites replace directives with `go mod edit`, then runs `go mod tidy` before `mise run ci`.

- [ ] **Step 1: Create `.github/workflows/ci.yaml`**

```yaml
# SPDX-License-Identifier: GPL-2.0-only
name: CI

on:
  push:
    branches: [master]
  pull_request:

jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
        with:
          path: flow

      - name: Check out Hive
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/Hive
          path: hive

      - name: Check out Sharkfin
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/sharkfin
          path: sharkfin

      - name: Check out Pylon
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/Pylon
          path: pylon

      - uses: jdx/mise-action@v3
        with:
          working_directory: flow

      - name: Rewrite replace directives for CI paths
        working-directory: flow
        run: |
          go mod edit \
            -replace github.com/Work-Fort/Hive=../hive \
            -replace "github.com/Work-Fort/sharkfin/client/go=../sharkfin/client/go" \
            -replace "github.com/Work-Fort/Pylon/client/go=../pylon/client/go"
          go mod tidy

      - run: mise run ci
        working-directory: flow
```

- [ ] **Step 2: Verify** — YAML is valid (`python3 -c "import yaml,sys; yaml.safe_load(sys.stdin)" < .github/workflows/ci.yaml` exits 0).

- [ ] **Step 3: Commit** — `feat(ci): add CI workflow`

---

### Task 10: Create `.github/workflows/release.yaml`

**Files:** `.github/workflows/release.yaml` (new)

The `test` job rewrites replace directives and runs `go mod tidy` before `mise run ci`. The `build` matrix job installs Go via `jdx/mise-action@v3`, rewrites replace directives, and runs `go mod tidy` before invoking `go build` directly (cross-compilation requires explicit `GOOS`/`GOARCH` env, which `mise run build:release` does not forward).

- [ ] **Step 1: Create `.github/workflows/release.yaml`**

```yaml
# SPDX-License-Identifier: GPL-2.0-only
name: Release

on:
  push:
    branches: [master]

permissions:
  contents: write

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0
          path: flow

      - name: Check out Hive
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/Hive
          path: hive

      - name: Check out Sharkfin
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/sharkfin
          path: sharkfin

      - name: Check out Pylon
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/Pylon
          path: pylon

      - uses: jdx/mise-action@v3
        with:
          working_directory: flow

      - name: Rewrite replace directives for CI paths
        working-directory: flow
        run: |
          go mod edit \
            -replace github.com/Work-Fort/Hive=../hive \
            -replace "github.com/Work-Fort/sharkfin/client/go=../sharkfin/client/go" \
            -replace "github.com/Work-Fort/Pylon/client/go=../pylon/client/go"
          go mod tidy

      - run: mise run ci
        working-directory: flow

  tag:
    name: Create Version Tag
    runs-on: ubuntu-latest
    needs: [test]
    outputs:
      new_tag: ${{ steps.tag_version.outputs.new_tag }}
      changelog: ${{ steps.tag_version.outputs.changelog }}
      should_release: ${{ steps.check_tag.outputs.should_release }}
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0

      - name: Bump version and create tag
        id: tag_version
        uses: Work-Fort/github-tag-action@v6.3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          default_bump: false
          release_branches: master
          tag_prefix: v

      - name: Check if new tag was created
        id: check_tag
        run: |
          if [ -n "${{ steps.tag_version.outputs.new_tag }}" ]; then
            echo "should_release=true" >> $GITHUB_OUTPUT
          else
            echo "should_release=false" >> $GITHUB_OUTPUT
          fi

  build:
    name: Build (${{ matrix.target }})
    runs-on: ubuntu-latest
    needs: tag
    if: needs.tag.outputs.should_release == 'true'
    strategy:
      matrix:
        include:
          - target: linux-amd64
            goos: linux
            goarch: amd64
          - target: linux-arm64
            goos: linux
            goarch: arm64
          - target: darwin-amd64
            goos: darwin
            goarch: amd64
          - target: darwin-arm64
            goos: darwin
            goarch: arm64
      fail-fast: false
    steps:
      - uses: actions/checkout@v6
        with:
          ref: ${{ needs.tag.outputs.new_tag }}
          path: flow

      - name: Check out Hive
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/Hive
          path: hive

      - name: Check out Sharkfin
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/sharkfin
          path: sharkfin

      - name: Check out Pylon
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/Pylon
          path: pylon

      - uses: jdx/mise-action@v3
        with:
          working_directory: flow

      - name: Rewrite replace directives for CI paths
        working-directory: flow
        run: |
          go mod edit \
            -replace github.com/Work-Fort/Hive=../hive \
            -replace "github.com/Work-Fort/sharkfin/client/go=../sharkfin/client/go" \
            -replace "github.com/Work-Fort/Pylon/client/go=../pylon/client/go"
          go mod tidy

      - name: Build binary
        working-directory: flow
        env:
          CGO_ENABLED: '0'
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: |
          VERSION="${{ needs.tag.outputs.new_tag }}"
          go build -trimpath \
            -ldflags "-s -w -X github.com/Work-Fort/Flow/cmd.Version=${VERSION}" \
            -o "flow-${{ matrix.target }}"

      - name: Compress and generate checksum
        working-directory: flow
        run: |
          TARGET="${{ matrix.target }}"
          xz -z -9 "flow-${TARGET}"
          sha256sum "flow-${TARGET}.xz" > "flow-${TARGET}.sha256"

      - uses: actions/upload-artifact@v4
        with:
          name: ${{ matrix.target }}-${{ needs.tag.outputs.new_tag }}
          path: flow/flow-${{ matrix.target }}.*
          retention-days: 7

  release:
    name: Create GitHub Release
    runs-on: ubuntu-latest
    needs: [tag, build]
    if: needs.tag.outputs.should_release == 'true'
    steps:
      - uses: actions/checkout@v6
        with:
          ref: ${{ needs.tag.outputs.new_tag }}

      - uses: actions/download-artifact@v4
        with:
          path: artifacts
          pattern: '*-${{ needs.tag.outputs.new_tag }}'
          merge-multiple: true

      - name: Create SHA256SUMS
        working-directory: artifacts
        run: |
          cat *.sha256 > SHA256SUMS
          rm *.sha256

      - name: Create release notes
        env:
          TAG: ${{ needs.tag.outputs.new_tag }}
          CHANGELOG: ${{ needs.tag.outputs.changelog }}
          REPO: ${{ github.repository }}
        run: |
          VERSION=$(echo "$TAG" | sed 's/^v//')
          cat > release-notes.md << EOF
          # Flow v${VERSION}

          ## What's Changed

          ${CHANGELOG}

          ## Installation

          ### Linux (x86_64)
          \`\`\`bash
          wget https://github.com/${REPO}/releases/download/${TAG}/flow-linux-amd64.xz
          xz -d flow-linux-amd64.xz
          chmod +x flow-linux-amd64
          sudo mv flow-linux-amd64 /usr/local/bin/flow
          \`\`\`

          ### Linux (ARM64)
          \`\`\`bash
          wget https://github.com/${REPO}/releases/download/${TAG}/flow-linux-arm64.xz
          xz -d flow-linux-arm64.xz
          chmod +x flow-linux-arm64
          sudo mv flow-linux-arm64 /usr/local/bin/flow
          \`\`\`

          ### macOS (Apple Silicon)
          \`\`\`bash
          wget https://github.com/${REPO}/releases/download/${TAG}/flow-darwin-arm64.xz
          xz -d flow-darwin-arm64.xz
          chmod +x flow-darwin-arm64
          sudo mv flow-darwin-arm64 /usr/local/bin/flow
          \`\`\`

          ### macOS (Intel)
          \`\`\`bash
          wget https://github.com/${REPO}/releases/download/${TAG}/flow-darwin-amd64.xz
          xz -d flow-darwin-amd64.xz
          chmod +x flow-darwin-amd64
          sudo mv flow-darwin-amd64 /usr/local/bin/flow
          \`\`\`

          ## Verification

          \`\`\`bash
          wget https://github.com/${REPO}/releases/download/${TAG}/SHA256SUMS
          sha256sum -c SHA256SUMS --ignore-missing
          \`\`\`

          ---

          Built automatically by [Flow CI](https://github.com/${REPO})
          EOF

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          tag_name: ${{ needs.tag.outputs.new_tag }}
          name: Release ${{ needs.tag.outputs.new_tag }}
          body_path: release-notes.md
          draft: false
          prerelease: false
          files: artifacts/*
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 2: Verify** — YAML is valid.

- [ ] **Step 3: Commit** — `feat(ci): add release workflow`

---

### Task 11: Create `.github/workflows/release-container.yaml`

**Files:** `.github/workflows/release-container.yaml` (new)

The container workflow populates `vendor-local/` inside the Docker build context before running `docker buildx build`. Sharkfin and Pylon client/go packages are subdirectories within their repos, so the workflow checks out the full repos and copies just the `client/go` subdirectory into the right `vendor-local/` path.

- [ ] **Step 1: Create `.github/workflows/release-container.yaml`**

```yaml
# SPDX-License-Identifier: GPL-2.0-only
name: Release Container Image

on:
  release:
    types: [published]

permissions:
  contents: read
  packages: write

jobs:
  container:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - name: Check out Hive
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/Hive
          path: vendor-local/hive

      - name: Check out Sharkfin (for client/go)
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/sharkfin
          path: vendor-local/sharkfin-client-go-repo

      - name: Check out Pylon (for client/go)
        uses: actions/checkout@v6
        with:
          repository: Work-Fort/Pylon
          path: vendor-local/pylon-client-go-repo

      - name: Extract client/go subdirectories
        run: |
          cp -r vendor-local/sharkfin-client-go-repo/client/go vendor-local/sharkfin-client-go
          cp -r vendor-local/pylon-client-go-repo/client/go vendor-local/pylon-client-go
          rm -rf vendor-local/sharkfin-client-go-repo vendor-local/pylon-client-go-repo

      - uses: docker/setup-buildx-action@v4

      - name: Log in to GHCR
        uses: docker/login-action@v4
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push
        uses: docker/build-push-action@v7
        with:
          context: .
          push: true
          platforms: linux/amd64,linux/arm64
          build-args: VERSION=${{ github.ref_name }}
          tags: |
            ghcr.io/${{ github.repository }}:${{ github.ref_name }}
            ghcr.io/${{ github.repository }}:latest
```

- [ ] **Step 2: Verify** — YAML is valid.

- [ ] **Step 3: Commit** — `feat(ci): add container release workflow`

---

## Summary

After all tasks complete, the repo will have:

```
.mise/tasks/
  build/dev
  build/release
  ci
  docker/vendor-local
  lint
  test
mise.toml               (tools only, no inline tasks)
Dockerfile
.dockerignore
.github/workflows/
  ci.yaml
  release.yaml
  release-container.yaml
```

**Replace directive strategy recap:**
- Local dev: unchanged `go.mod` with absolute local paths — works as before.
- Local Docker builds: `mise run docker:vendor-local` populates `vendor-local/` from sibling repos, then `docker build .` works.
- CI workflows: check out each dependency repo alongside Flow, rewrite replace directives with `go mod edit` to relative `../` paths, run `go mod tidy` to refresh `go.sum`, then proceed with build/test. Edits are ephemeral (not committed).
- Docker builds in CI: the container release workflow populates `vendor-local/` from checked-out repos before calling `docker buildx build`. The Dockerfile rewrites replace directives, runs `go mod tidy`, then proceeds with `go mod download` and build.
