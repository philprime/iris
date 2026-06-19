# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26
# POSTFIX_IMAGE is kept current by renovate. It is declared before the first
# FROM so it is usable in the final stage's FROM.
ARG POSTFIX_IMAGE=boky/postfix:latest

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
    -o /out/reloader ./cmd/reloader

FROM ${POSTFIX_IMAGE}
COPY --from=builder /out/reloader /usr/local/bin/iris-reloader
COPY build/postfix-entrypoint.sh /usr/local/bin/iris-entrypoint.sh
ENTRYPOINT ["/usr/local/bin/iris-entrypoint.sh"]
