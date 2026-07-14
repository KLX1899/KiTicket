# syntax=docker/dockerfile:1.7
FROM golang:1.26.0-alpine3.23 AS build
ARG SERVICE
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/${SERVICE}

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata && addgroup -S kiticket && adduser -S -G kiticket kiticket
COPY --from=build /out/service /service
USER kiticket
ENTRYPOINT ["/service"]
