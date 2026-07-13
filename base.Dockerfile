# syntax=docker/dockerfile:1
# Shared base images for OLake driver builds.
#   build   — Go toolchain + JRE/maven/node(chalk)/jq + a warmed Go module/build cache;
#             used only by the disposable integration-test container (tests/testutils),
#             which builds this image locally on first use.
#   runtime — slim base the shipped driver images sit on.
#
# Local-only — not published to a registry. Rebuild when this file changes:
#   make docker.base.build   # host arch (set BASE_PLATFORMS to cross-build)
#
# The golang version comes from the go directive in go.mod: docker.base.build
# passes it as GO_VERSION and derives the image tag (build-go<version>) from the
# same line, so bumping go.mod is the only step. The default (golang:1-bookworm,
# the rolling latest-Go-1.x tag) ONLY APPLIES TO DIRECT DOCKER BUILDS.

# ---------------------------------------------------------------------------
ARG GO_VERSION=1
FROM golang:${GO_VERSION}-bookworm AS build

RUN apt-get update && apt-get install -y \
        openjdk-17-jre-headless maven nodejs npm jq \
    && npm install -g chalk-cli \
    && rm -rf /var/lib/apt/lists/*

# Warm the Go module and build caches into this image so the disposable driver-build containers
# start hot. testutils mounts the go-mod / go-build named volumes at /go/pkg/mod and
# /root/.cache/go-build; Docker seeds those (empty) volumes from the image on first mount, so
# whatever is cached here is what every in-container build.sh compile reuses. `go work sync`
# downloads the whole workspace's module graph (every driver), and `go build ./...` compiles the
# root module — it stops at the drivers/* module boundaries, so no per-driver cgo toolchain
# (notably db2's clidriver) is needed here. The source is bind-mounted for this step only and is
# not baked into the image; only the caches under /go and /root/.cache persist.
RUN --mount=type=bind,target=/src,rw \
    cd /src && go work sync && go build ./...
