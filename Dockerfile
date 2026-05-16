FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o vm-energy-agent ./cmd/agent

FROM gcr.io/distroless/static-debian12

WORKDIR /
COPY --from=builder /app/vm-energy-agent /vm-energy-agent

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/vm-energy-agent"]