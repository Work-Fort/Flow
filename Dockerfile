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
