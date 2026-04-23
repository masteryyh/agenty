.PHONY: all build clean test vet fmt install

all: build

build:
	@echo "Building agenty..."
	go build -o bin/agenty cmd/main.go

install: build
	@echo "Installing agenty to /usr/local/bin..."
	sudo install -m 755 bin/agenty /usr/local/bin/agenty
	sudo chmod +x /usr/local/bin/agenty

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
