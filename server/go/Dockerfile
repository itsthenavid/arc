FROM golang:1.25.6-alpine AS build
WORKDIR /src

# Install build essentials + CA bundle for module downloads.
RUN apk add --no-cache git ca-certificates

# Monorepo build:
# - context is repository root
# - go.work wires server/go + shared together
COPY go.work go.work.sum ./
COPY server/go/go.mod server/go/go.sum ./server/go/
COPY shared/go.mod ./shared/go.mod
RUN cd /src/server/go && go mod download

COPY server/go ./server/go
COPY shared ./shared
RUN cd /src/server/go && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/arc ./cmd/arc

# Debian-based distroless includes a CA trust store suitable for outbound TLS.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/arc /arc
USER nonroot:nonroot
ENTRYPOINT ["/arc"]
