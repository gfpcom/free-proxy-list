.PHONY: update build lint install-lint check-sources

update:
	go build -o gfp cmd/main.go && ./gfp

build:
	go build -v -o gfp cmd/main.go

install-lint:
	go install golang.org/x/lint/golint@latest

lint:
	golint -set_exit_status ./...

check-sources:
	@bash ./scripts/check-sources.sh

