PREFIX ?= $(CURDIR)/bin

.PHONY: build clean

build:
	go build -ldflags="-s -w" -o $(PREFIX)/claude-state ./cmd/claude-state/

clean:
	rm -rf bin/
