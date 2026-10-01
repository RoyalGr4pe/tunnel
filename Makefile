PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build install test

build:
	mkdir -p bin
	go build -o bin/karo .

install: build
	mkdir -p "$(BINDIR)"
	install -m 755 bin/karo "$(BINDIR)/karo"

test:
	go test ./...
