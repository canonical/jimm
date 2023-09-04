# Copyright 2014 Canonical Ltd.
# Makefile for the JIMM service.

export GO111MODULE=on
export DOCKER_BUILDKIT=1

PROJECT := github.com/canonical/jimm

GIT_COMMIT := $(shell git rev-parse --verify HEAD)
GIT_VERSION := $(shell git describe --abbrev=0 --dirty)
GO_VERSION := $(shell go list -f {{.GoVersion}} -m)

ifeq ($(shell uname -p | sed -r 's/.*(x86|armel|armhf).*/golang/'), golang)
	GO_C := golang
	INSTALL_FLAGS :=
else
	GO_C := gccgo-4.9 gccgo-go
	INSTALL_FLAGS := -gccgoflags=-static-libgo
endif

# bzr and git are installed so that 'go get' will work with those VCS.
define DEPENDENCIES
  bzr
  git
  $(GO_C)
endef

default: build

build: version/commit.txt version/version.txt
	go build -tags version $(PROJECT)/...

build/server: version/commit.txt version/version.txt
	go build -tags version ./cmd/jimmsrv

check: version/commit.txt version/version.txt
	go test -timeout 30m $(PROJECT)/... -coverprofile cover.out

install: version/commit.txt version/version.txt
	go install -tags version $(INSTALL_FLAGS) -v $(PROJECT)/...

release: jimm-$(GIT_VERSION).tar.xz

clean:
	go clean $(PROJECT)/...
	-$(RM) version/commit.txt version/version.txt
	-$(RM) jimmsrv
	-$(RM) -r jimm-release/
	-$(RM) jimm-*.tar.xz

# Reformat all source files.
format:
	gofmt -w -l .

# Reformat and simplify source files.
simplify:
	gofmt -w -l -s .

# Run the JIMM server.
server: install
	jemd cmd/jemd/config.yaml

# Generate version information
version/commit.txt: FORCE
	git rev-parse --verify HEAD > version/commit.txt

version/version.txt: FORCE
	if [ -z "$(GIT_VERSION)" ]; then \
        echo "dev" > version/version.txt; \
    else \
        echo $(GIT_VERSION) > version/version.txt; \
    fi

jimmsrv: version/commit.txt version/version.txt
	go build -tags release -v $(PROJECT)/cmd/jemd

jimm-$(GIT_VERSION).tar.xz: jimm-release/bin/jimmsrv
	tar c -C jimm-release . | xz > $@

jimm-release/bin/jimmsrv: jimmsrv
	mkdir -p jimm-release/bin
	cp jemd jimm-release/bin

jimm-image:
	docker build --target deploy-env \
	--build-arg="GIT_COMMIT=$(GIT_COMMIT)" \
	--build-arg="VERSION=$(GIT_VERSION)" \
	--build-arg="GO_VERSION=$(GO_VERSION)" \
	--tag jimm:latest .

jimm-snap:
	mkdir -p ./snap
	cp ./snaps/jimm/snapcraft.yaml ./snap/
	snapcraft 

push-microk8s: jimm-image
	docker tag jimm:latest localhost:32000/jimm:latest
	docker push localhost:32000/jimm:latest

pull/candid:
	-git clone https://github.com/canonical/candid.git ./tmp/candid
	(cd ./tmp/candid && make image)
	docker image ls candid

get-local-auth:
	@go run ./local/authy

# Install packages required to develop JIMM and run tests.
APT_BASED := $(shell command -v apt-get >/dev/null; echo $$?)
sysdeps:
ifeq ($(APT_BASED),0)
ifeq ($(shell lsb_release -cs|sed -r 's/precise|quantal|raring/old/'),old)
	@echo Adding PPAs for golang and mongodb
	@sudo apt-add-repository --yes ppa:juju/golang
	@sudo apt-add-repository --yes ppa:juju/stable
endif
	@echo Installing dependencies
	@sudo apt-get update
	@sudo snap install juju-db --channel 4.4/stable 
	@sudo apt-get --yes install $(strip $(DEPENDENCIES)) \
	  $(shell apt-cache madison juju-mongodb mongodb-server | head -1 | cut -d '|' -f1)
else
	@echo sysdeps runs only on systems with apt-get
	@echo on OS X with homebrew try: brew install bazaar mongodb
endif

help:
	@echo -e 'JIMM - list of make targets:\n'
	@echo 'make - Build the package.'
	@echo 'make check - Run tests.'
	@echo 'make install - Install the package.'
	@echo 'make server - Start the JIMM server.'
	@echo 'make clean - Remove object files from package source directories.'
	@echo 'make sysdeps - Install the development environment system packages.'
	@echo 'make format - Format the source files.'
	@echo 'make simplify - Format and simplify the source files.'
	@echo 'make pull/candid - Pull candid for local development environment.'
	@echo 'make get-local-auth - Get local auth to the API WSS endpoint locally.'

.PHONY: build check install release clean format server simplify sysdeps help FORCE

FORCE:
