# syntax=docker/dockerfile:1.7

# --- Build stage -------------------------------------------------------------
# Pinned to 1.22-alpine to match go.mod (go 1.22.2). Alpine keeps the build
# image small; we statically link below so the runtime base can be distroless.
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Build context is the repo root — source lives under backend/.
COPY backend/go.mod ./

# Two-step copy: module layer (reused when only source changes) then source.
COPY backend/cmd ./cmd
COPY backend/internal ./internal

# CGO_ENABLED=0 produces a fully static binary so it runs on distroless/static.
# -trimpath and -ldflags="-s -w" strip local paths and debug info for a smaller
# image without affecting runtime behaviour.
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/server \
        ./cmd/server

# Seed data directory — reviews/ must exist so the server can write temp files
# without needing to create the dir itself inside a read-only layer.
RUN mkdir -p /data/reviews

# --- Runtime stage -----------------------------------------------------------
# distroless/static gives us a non-root user (nonroot:nonroot, uid 65532),
# ca-certificates, tzdata, and nothing else. No shell, no package manager.
FROM gcr.io/distroless/static-debian12:nonroot

ENV PORT=8080 \
    DATA_DIR=/data \
    POLL_INTERVAL_MINUTES=15 \
    MAX_PAGES=10

# Copy the pre-built binary and the seed data directory (owned by nonroot so
# the server can write reviews and update apps.json without a volume mount).
COPY --from=builder --chown=nonroot:nonroot /out/server /server
COPY --from=builder --chown=nonroot:nonroot /data /data
# Seed tracked apps and any previously-extracted review snapshots. Run
# `docker cp runway-backend:/data/reviews ./data/` then rebuild to persist
# reviews across image rebuilds.
COPY --chown=nonroot:nonroot data/apps.json /data/apps.json
COPY --chown=nonroot:nonroot data/reviews/ /data/reviews/

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/server"]
