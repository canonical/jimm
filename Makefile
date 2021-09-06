# Copyright 2014 Canonical Ltd.
# Makefile for the JIMM service.

export GO111MODULE=on

PROJECT := github.com/CanonicalLtd/jimm

GIT_COMMIT := $(shell git rev-parse --verify HEAD)
GIT_VERSION := $(shell git describe --dirty)

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

build: version/v/git-commit version/v/version
	go build $(PROJECT)/...

check: version/v/git-commit version/v/version
	go test -p 1 -timeout 30m $(PROJECT)/...

install: version/v/git-commit version/v/version
	go install $(INSTALL_FLAGS) -v $(PROJECT)/...

release: jimm-$(GIT_VERSION).tar.xz

clean:
	go clean $(PROJECT)/...
	-$(RM) version/v/git-commit version/v/version
	-$(RM) jimmsrv
	-$(RM) -r jimm-release/
	-$(RM) jimm-*.tar.xz

# Reformat source files.
format:
	gofmt -w -l .

# Reformat and simplify source files.
simplify:
	gofmt -w -l -s .

# Run the JIMM server.
server: install
	jemd cmd/jemd/config.yaml

# Generate version information
version/v/git-commit: FORCE
	git rev-parse --verify HEAD > version/v/git-commit

version/v/version: FORCE
	git describe --dirty > version/v/version

jimmsrv: version/v/git-commit version/v/version
	go build -tags release -v $(PROJECT)/cmd/jemd

jimm-$(GIT_VERSION).tar.xz: jimm-release/bin/jimmsrv
	tar c -C jimm-release . | xz > $@

jimm-release/bin/jimmsrv: jimmsrv
	mkdir -p jimm-release/bin
	cp jemd jimm-release/bin

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

.PHONY: build check install release clean format server simplify sysdeps help FORCE

FORCE:
