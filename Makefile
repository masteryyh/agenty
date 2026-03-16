.PHONY: all build clean test vet fmt

all: build

build:
	@echo "Building agenty..."
	go build -o bin/agenty cmd/main.go

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
