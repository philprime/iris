# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src

# Cache dependencies separately from the source.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE} -X main.sentryRelease=iris@${VERSION}:${COMMIT}" \
    -o /out/controller ./cmd/controller

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /out/controller /controller
USER 65532:65532
ENTRYPOINT ["/controller"]
