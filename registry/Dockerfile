# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
RUN apk add --no-cache build-base && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/registry ./cmd/registry

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/registry /registry
EXPOSE 8080
ENTRYPOINT ["/registry"]
