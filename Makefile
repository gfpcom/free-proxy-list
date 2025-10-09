.PHONY: update build lint install-lint check-sources clean-lfs

update:
	go build -o gfp cmd/main.go && ./gfp

clean-lfs:
	git fetch --all
	git lfs migrate export --include="list/*" --everything
	git lfs migrate import --include="list/*" --everything --skip-fetch --fixup
	git lfs prune --force
	git reset --hard $$(git rev-parse HEAD)
	git lfs track "list/*"
	git add .gitattributes
	@echo "LFS objects cleaned up successfully"

build:
	go build -v -o gfp cmd/main.go

install-lint:
	go install golang.org/x/lint/golint@latest

lint:
	golint -set_exit_status ./...

check-sources:
	@bash ./scripts/check-sources.sh

