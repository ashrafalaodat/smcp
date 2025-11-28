# Multi-stage build for ksyncer operator
FROM golang:1.24 AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /workspace/bin/ksyncer ./cmd/ksyncer

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /workspace/bin/ksyncer /ksyncer
USER nonroot:nonroot
ENTRYPOINT ["/ksyncer"]
