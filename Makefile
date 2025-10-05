GO ?= go
PKGS := ./...
GOCACHE ?= $(PWD)/.cache/go-build

export GOCACHE

.PHONY: all build test cover race bench fmt lint tidy

all: build

build:
	$(GO) build $(PKGS)

test:
	$(GO) test $(PKGS)

cover:
	$(GO) test -coverprofile=coverage.out $(PKGS)

race:
	$(GO) test -race $(PKGS)

bench:
	$(GO) test -bench=. $(PKGS)

fmt:
	$(GO) fmt $(PKGS)

lint:
	$(GO) vet $(PKGS)

tidy:
	$(GO) mod tidy
