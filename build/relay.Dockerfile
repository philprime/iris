# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE} -X main.sentryRelease=iris@${VERSION}:${COMMIT}" \
    -o /out/relay ./cmd/relay

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /out/relay /relay
USER 65532:65532
EXPOSE 25 8080
ENTRYPOINT ["/relay"]
