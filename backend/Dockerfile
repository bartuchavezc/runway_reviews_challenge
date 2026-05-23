# syntax=docker/dockerfile:1.7

# --- Build stage -------------------------------------------------------------
# Pinned to 1.22-alpine to match go.mod (go 1.22.2). Alpine keeps the build
# image small; we statically link below so the runtime base can be distroless.
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Copy go.mod first. The project is stdlib-only today so go.sum is intentionally
# absent; we only copy go.mod and skip the download step (nothing to fetch).
COPY go.mod ./

# Now copy the rest of the source. Doing this in two steps lets Docker reuse
# the module-download layer when only source files change.
COPY cmd ./cmd
COPY internal ./internal

# CGO_ENABLED=0 produces a fully static binary so it runs on distroless/static.
# -trimpath and -ldflags="-s -w" strip local paths and debug info for a smaller
# image without affecting runtime behaviour.
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/server \
        ./cmd/server

# --- Runtime stage -----------------------------------------------------------
# distroless/static gives us a non-root user (nonroot:nonroot, uid 65532),
# ca-certificates, tzdata, and nothing else. No shell, no package manager.
FROM gcr.io/distroless/static-debian12:nonroot

# Defaults that match cmd/server/main.go's env.GetString/GetInt calls.
# DATA_DIR is /data so the compose bind mount works out of the box.
ENV PORT=8080 \
    DATA_DIR=/data \
    POLL_INTERVAL_MINUTES=15 \
    MAX_PAGES=10

# Declare the data volume so anonymous-volume users still get persistence and
# `docker inspect` surfaces it. Compose overrides this with a bind mount.
VOLUME ["/data"]

COPY --from=builder --chown=nonroot:nonroot /out/server /server

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/server"]
