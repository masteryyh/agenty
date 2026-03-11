.PHONY: all build-server build-cli clean test vet fmt

all: build-server build-cli

build-server:
	@echo "Building server..."
	go build -o bin/agenty-server cmd/server.go

build-cli:
	@echo "Building CLI..."
	go build -o bin/agenty-cli cmd/cli/main.go

clean:
	@echo "Cleaning..."
	rm -rf bin/

test:
	@echo "Running tests..."
	go test ./... -v

vet:
	@echo "Running go vet..."
	go vet ./...

fmt:
	@echo "Running go fmt..."
	go fmt ./...
