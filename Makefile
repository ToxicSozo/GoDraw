.PHONY: build run test

build:
	go build -o bin/reviewer-service ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./...
