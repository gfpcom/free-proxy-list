.PHONY: update build clone-wiki update-wiki test lint install-curl-impersonate

CURL_IMPERSONATE_VERSION ?= v0.6.1
CURL_IMPERSONATE_INSTALL_DIR ?= $(HOME)/.local/bin

update:
	go build -o gfp cmd/main.go && ./gfp

build:
	go build -v -o gfp cmd/main.go

test:
	go test -coverprofile=coverage.txt ./...

lint:
	golangci-lint run

install-curl-impersonate:
	@set -eu; \
	case "$$(uname -m)" in \
		x86_64) platform=x86_64-linux-gnu ;; \
		aarch64|arm64) platform=aarch64-linux-gnu ;; \
		armv7l) platform=arm-linux-gnueabihf ;; \
		*) echo "unsupported curl-impersonate architecture: $$(uname -m)" >&2; exit 1 ;; \
	esac; \
	sudo apt-get update; \
	sudo apt-get install -y libnss3 nss-plugin-pem ca-certificates; \
	archive="curl-impersonate-$(CURL_IMPERSONATE_VERSION).$$platform.tar.gz"; \
	mkdir -p "$(CURL_IMPERSONATE_INSTALL_DIR)"; \
	curl -fL "https://github.com/lwthiker/curl-impersonate/releases/download/$(CURL_IMPERSONATE_VERSION)/$$archive" | tar -xzf - -C "$(CURL_IMPERSONATE_INSTALL_DIR)" --no-same-owner --no-same-permissions

# clone-wiki: clone the wiki repo and copy generated list files into wiki/lists
clone-wiki:
	# Clone wiki repository
	rm -rf ../wiki || true
	git clone https://github.com/gfpcom/free-proxy-list.wiki.git ../wiki || true
	mkdir -p ../wiki/lists
	cp -r list/* ../wiki/lists/ || true


# update-wiki: build Home.md and push to wiki (assumes wiki/ already contains lists/)
update-wiki:
	./update_wiki.sh
