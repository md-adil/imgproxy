# ---------------------------
# 1. Build Stage
# ---------------------------
FROM golang:1.24-bookworm AS builder

# Install build tools and libvips dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips-dev libheif-dev libaom-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy go mod files and download deps (layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the Go binary with CGO enabled (required for bimg/libvips)
# -ldflags="-s -w" strips debug info for smaller binary
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o imgproxy ./cmd

# ---------------------------
# 2. Runtime Stage
# ---------------------------
FROM debian:bookworm-slim

# Install only the runtime libvips dependency
RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips42 \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root user for security
RUN useradd -u 1001 -m appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/imgproxy .

# Set ownership to non-root user
RUN chown appuser:appuser /app/imgproxy

USER appuser

# Expose port
EXPOSE 8080

# Run the app
CMD ["./imgproxy"]