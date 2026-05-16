# ============================================
# Stage 1: Builder
# ============================================
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# 1. Copy go.mod and go.sum first
COPY go.mod go.sum ./

# 2. Copy the full source code
COPY . .

# 3. Now run go mod tidy (after source is present) + download
RUN go mod tidy && go mod download

# 4. Build the binary
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