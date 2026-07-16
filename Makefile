BIN := $(HOME)/.local/bin/pier

.PHONY: build install test clean

build:
	go build -o bin/pier .

install:
	mkdir -p $(HOME)/.local/bin
	go build -o $(BIN) .

# one-shot: build, install, and wire tmux + Claude Code hooks
setup: install
	$(BIN) setup

test:
	go vet ./...
	go test ./...

clean:
	rm -rf bin
