.PHONY: build run test setup lint docker-build docker-run

setup:
	go mod tidy

build:
	go build -o bin/vm-energy-agent ./cmd/agent

run:
	go run ./cmd/agent

test:
	go test ./...

lint:
	go vet ./...

docker-build:
	docker build -t vm-energy-agent .

docker-run:
	docker compose up --build
