# Copyright 2014 Canonical Ltd.
# Makefile for the JIMM service.

ifndef GOPATH
$(warning You need to set up a GOPATH.)
endif

PROJECT := github.com/CanonicalLtd/jimm
PROJECT_DIR := $(shell go list -e -f '{{.Dir}}' $(PROJECT))

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

ifeq ($(VERSION),no)
	VERSIONDEPS :=
else
	VERSIONDEPS := version/init.go
endif

default: build

$(GOPATH)/bin/godeps:
	go get -v github.com/rogpeppe/godeps

# Start of GOPATH-dependent targets. Some targets only make sense -
# and will only work - when this tree is found on the GOPATH.
ifeq ($(CURDIR),$(PROJECT_DIR))

build: $(VERSIONDEPS)
	go build $(PROJECT)/...

check: $(VERSIONDEPS)
	go test $(PROJECT)/...

install: $(VERSIONDEPS)
	go install $(INSTALL_FLAGS) -v $(PROJECT)/...

release: jimm-$(GIT_VERSION).tar.xz

clean:
	go clean $(PROJECT)/...
	-$(RM) version/init.go
	-$(RM) jemd
	-$(RM) -r jimm-release/
	-$(RM) jimm-*.tar.xz

else

build:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

check:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

install:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

release:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

clean:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

endif
# End of GOPATH-dependent targets.

# Reformat source files.
format:
	gofmt -w -l .

# Reformat and simplify source files.
simplify:
	gofmt -w -l -s .

# Run the JIMM server.
server: install
	jemd cmd/jemd/config.yaml

# Update the project Go dependencies to the required revision.
deps: $(GOPATH)/bin/godeps
	$(GOPATH)/bin/godeps -u dependencies.tsv

# Generate the dependencies file.
create-deps: $(GOPATH)/bin/godeps
	godeps -t $(shell go list $(PROJECT)/...) > dependencies.tsv || true

# Generate version information
version/init.go: version/init.go.tmpl FORCE
	gofmt -r "unknownVersion -> Version{GitCommit: \"${GIT_COMMIT}\", Version: \"${GIT_VERSION}\",}" $< > $@

jemd: version/init.go
	go build -v $(PROJECT)/cmd/jemd

jimm-$(GIT_VERSION).tar.xz: jimm-release/bin/jemd
	tar c -C jimm-release . | xz > $@

jimm-release/bin/jemd: jemd
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
	@echo -e 'Identity service - list of make targets:\n'
	@echo 'make - Build the package.'
	@echo 'make check - Run tests.'
	@echo 'make install - Install the package.'
	@echo 'make server - Start the JIMM server.'
	@echo 'make clean - Remove object files from package source directories.'
	@echo 'make sysdeps - Install the development environment system packages.'
	@echo 'make deps - Set up the project Go dependencies.'
	@echo 'make create-deps - Generate the Go dependencies file.'
	@echo 'make format - Format the source files.'
	@echo 'make simplify - Format and simplify the source files.'

.PHONY: build check install release clean format server simplify sysdeps help FORCE

FORCE:
