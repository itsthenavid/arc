# syntax=docker/dockerfile:1

FROM golang:1.25.6-alpine AS build
WORKDIR /src

# Install build essentials + CA bundle for module downloads.
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/arc ./cmd/arc

# Debian-based distroless includes a CA trust store suitable for outbound TLS.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/arc /arc
USER nonroot:nonroot
ENTRYPOINT ["/arc"]
