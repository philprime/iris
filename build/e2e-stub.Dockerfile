# syntax=docker/dockerfile:1
# Image for the end-to-end stub HTTP destination (test/e2e/stub). It is built
# and loaded into the kind cluster by `make test-e2e` and is not published.

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/stub ./test/e2e/stub

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /out/stub /stub
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/stub"]
