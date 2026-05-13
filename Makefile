.PHONY: all build clean test vet fmt install

VERSION ?= $(shell sh -c 'rev=$$(git rev-parse --short=12 HEAD); if ! git diff --quiet --ignore-submodules -- || ! git diff --cached --quiet --ignore-submodules --; then printf "%s-dirty" "$$rev"; else printf "%s" "$$rev"; fi')
LDFLAGS := -X github.com/masteryyh/agenty/pkg/version.Version=$(VERSION)

all: build

build:
	@echo "Building agenty..."
	go build -tags=fts5 -ldflags "$(LDFLAGS)" -o bin/agenty cmd/main.go

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
