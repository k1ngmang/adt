.PHONY: build generate test

build: generate
	go build ./...

generate:
	go generate ./...

test: generate
	go test ./...
