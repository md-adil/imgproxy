# ---------------------------
# 1. Build Stage
# ---------------------------
FROM golang:1.24-bookworm AS builder

# Install build tools and libvips dependencies (apt cache kept across builds)
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean \
    && apt-get update && apt-get install -y --no-install-recommends \
    libvips-dev libheif-dev libaom-dev

WORKDIR /app

# Copy go mod files and download deps (layer caching)
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy only the source needed to build
COPY cmd ./cmd
COPY internal ./internal

# Build the Go binary with CGO enabled (required for bimg/libvips)
# -ldflags="-s -w" strips debug info, -trimpath drops local paths for smaller, reproducible builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o imgproxy ./cmd

# ---------------------------
# 2. Runtime Stage
# ---------------------------
FROM debian:bookworm-slim

# Install only the runtime libvips dependency, plus CA certs for https source fetches
RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips42 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root user for security (no home or login shell needed)
RUN useradd -u 1000 --no-create-home --shell /usr/sbin/nologin imgproxy

WORKDIR /app

# Copy binary from builder (owned by root so the app cannot modify it)
COPY --from=builder /app/imgproxy .

# Disk cache location, used only when CACHE_DIR=file:///app/cached is set
RUN mkdir -p /app/cached && chown imgproxy:imgproxy /app/cached
VOLUME /app/cached

USER imgproxy

# Expose port
EXPOSE 8080

# Run the app
CMD ["./imgproxy"]
