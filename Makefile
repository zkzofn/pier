BIN := $(HOME)/.local/bin/pier

.PHONY: build install test clean

build:
	go build -o bin/pier .

install:
	mkdir -p $(HOME)/.local/bin
	go build -o $(BIN) .

test:
	go vet ./...
	go test ./...

clean:
	rm -rf bin
