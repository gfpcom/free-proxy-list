.PHONY: update build clone-wiki update-wiki

update:
	go build -o gfp cmd/main.go && ./gfp

build:
	go build -v -o gfp cmd/main.go

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
