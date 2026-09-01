.PHONY: help build run test fmt vet install

help:
	@printf "%s\n" "make build" "make run" "make test" "make fmt" "make vet" "make install"

build:
	nix develop -c go build -o rv .

run:
	nix develop -c go run .

test:
	nix develop -c go test ./...

fmt:
	nix develop -c gofmt -w *.go

vet:
	nix develop -c go vet ./...

install: build
	mkdir -p "$$HOME/bin"
	mv ./rv "$$HOME/bin/rv"
