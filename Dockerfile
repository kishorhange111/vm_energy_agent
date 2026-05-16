# ============================================
# Stage 1: Builder
# ============================================
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go.mod + go.sum
COPY go.mod go.sum ./

# === FIX: Use go mod tidy first to clean bad entries in go.sum ===
# This prevents "unknown revision v0.0.1" errors for indirect dependencies
RUN go mod tidy && go mod download

# Copy source code
COPY . .

# Build optimized static binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /vm-energy-agent ./cmd/agent

# ============================================
# Stage 2: Minimal final image
# ============================================
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /vm-energy-agent /app/vm-energy-agent

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app/vm-energy-agent"]