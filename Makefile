.PHONY: build test tol lua54-subset-test

build:
	go fmt ./... && go build ./...

tol:
	go fmt ./... && go build -o tol ./cmd/tolang

test:
	go test -p $(or $(shell echo "$(MAKEFLAGS)" | grep -oP '(?<=-j)\d+' | head -1),$(shell expr $$(nproc 2>/dev/null || echo 8) / 2)) ./...

lua54-subset-test:
	./_tools/run-lua54-subset-tests.sh
