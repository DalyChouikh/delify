# ============================================
# Stage 1: Build the Go binary
# ============================================
FROM golang:1.25.4-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags '-static'" \
    -o /build/delify \
    ./cmd/bot

# ============================================
# Stage 2: Minimal runtime image
# ============================================
FROM alpine:3.19

# Install CA certificates for HTTPS requests
RUN apk add --no-cache ca-certificates

# Create non-root user for security
RUN addgroup -g 1001 -S delify && \
    adduser -u 1001 -S delify -G delify

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/delify /app/delify

# Use non-root user
USER delify

# Run the bot
ENTRYPOINT ["/app/delify"]
