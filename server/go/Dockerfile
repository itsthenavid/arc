# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS build
WORKDIR /src

# Install build essentials
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/arc ./cmd/arc

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/arc /arc
USER nonroot:nonroot
ENTRYPOINT ["/arc"]
