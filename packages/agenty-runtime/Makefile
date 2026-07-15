.PHONY: all build cli clean test vet fmt install

VERSION ?= $(shell sh -c 'rev=$$(git rev-parse --short=12 HEAD); if ! git diff --quiet --ignore-submodules -- || ! git diff --cached --quiet --ignore-submodules --; then printf "%s-dirty" "$$rev"; else printf "%s" "$$rev"; fi')
LDFLAGS := -X github.com/masteryyh/agenty/pkg/version.Version=$(VERSION)

all: build

build:
	@echo "Building agenty..."
	go build -tags=fts5 -ldflags "$(LDFLAGS)" -o bin/agenty cmd/main.go

clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -rf agenty-cli/dist

test:
	@echo "Running tests..."
	go test ./... -v
