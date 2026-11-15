# Build: the Go toolchain compiles a static binary (and runs the tests, so a red image never builds).
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /chat-api ./cmd/chat-api

# Run: a small image without a shell beyond busybox (wget serves the healthcheck); not root.
FROM alpine:3.20
RUN adduser -D -u 10001 app
COPY --from=build /chat-api /usr/local/bin/chat-api
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/chat-api"]
