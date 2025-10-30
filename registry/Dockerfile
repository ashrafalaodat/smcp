# syntax=docker/dockerfile:1.7

##
## Build stage
##
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Install build tooling
RUN apk add --no-cache ca-certificates git

# Copy go modules first to leverage Docker layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source.
COPY . .

# CGO disabled for static binary; adjust if pgx pure-go mode is sufficient.
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o /out/mcp-registry ./cmd/registry

##
## Runtime stage
##
FROM gcr.io/distroless/base-debian12:nonroot

ENV MCP_REGISTRY_SERVICE_NAME=mcp-registry \
    MCP_REGISTRY_HTTP_ADDRESS=:8080

WORKDIR /app
COPY --from=builder /out/mcp-registry /app/mcp-registry

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app/mcp-registry"]
