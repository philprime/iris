# syntax=docker/dockerfile:1
# Image for the end-to-end stub HTTP destination (test/e2e/stub). It is built
# and loaded into the kind cluster by `make test-e2e` and is not published.

ARG GO_VERSION=1.26

# Pin the builder to the native build platform so the Go toolchain always runs
# on the runner's own architecture. The target arch is selected via GOOS/GOARCH
# below, so any cross-arch build compiles at native speed instead of compiling
# under QEMU emulation.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# TARGETOS/TARGETARCH are provided by BuildKit for each requested platform.
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build,id=go-build-${TARGETOS}-${TARGETARCH} \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/stub ./test/e2e/stub

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /out/stub /stub
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/stub"]
