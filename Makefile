# Copyright 2014 Canonical Ltd.
# Makefile for the JEM service.

ifndef GOPATH
$(warning You need to set up a GOPATH.)
endif

PROJECT := github.com/CanonicalLtd/jem
PROJECT_DIR ?= $(shell go list -e -f '{{.Dir}}' $(PROJECT))

GIT_COMMIT := $(shell git rev-parse --verify HEAD)
GIT_VERSION := $(shell git describe --dirty)

ifeq ($(shell uname -p | sed -r 's/.*(x86|armel|armhf).*/golang/'), golang)
	GO_C := golang
	INSTALL_FLAGS :=
else
	GO_C := gccgo-4.9 gccgo-go
	INSTALL_FLAGS := -gccgoflags=-static-libgo
endif

define DEPENDENCIES
  build-essential
  bzr
  juju-mongodb
  mongodb-server
  $(GO_C)
endef

default: build

$(GOPATH)/bin/godeps:
	go get -v launchpad.net/godeps

# Start of GOPATH-dependent targets. Some targets only make sense -
# and will only work - when this tree is found on the GOPATH.
ifeq ($(CURDIR),$(PROJECT_DIR))

build: version/init.go
	go build $(PROJECT)/...

check: version/init.go
	go test $(PROJECT)/...

install: version/init.go
	go install $(INSTALL_FLAGS) -v $(PROJECT)/...

clean:
	go clean $(PROJECT)/...
	-$(RM) version/init.go

else

build:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

check:
	$(error Cannot $@; $(CURDIR) is not on GOPATH)

install:
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

# Run the JEM server.
server: install
	jemd -logging-config INFO cmd/jemd/config.yaml

# Update the project Go dependencies to the required revision.
deps: $(GOPATH)/bin/godeps
	$(GOPATH)/bin/godeps -u dependencies.tsv

# Generate the dependencies file.
create-deps: $(GOPATH)/bin/godeps
	godeps -t $(shell go list $(PROJECT)/...) > dependencies.tsv || true

# Generate version information
version/init.go: version/init.go.tmpl FORCE
	gofmt -r "unknownVersion -> Version{GitCommit: \"${GIT_COMMIT}\", Version: \"${GIT_VERSION}\",}" $< > $@

# Install packages required to develop JEM and run tests.
APT_BASED := $(shell command -v apt-get >/dev/null; echo $$?)
sysdeps:
ifeq ($(APT_BASED),0)
ifeq ($(shell lsb_release -cs|sed -r 's/precise|quantal|raring/old/'),old)
	@echo Adding PPAs for golang and mongodb
	@sudo apt-add-repository --yes ppa:juju/golang
	@sudo apt-add-repository --yes ppa:juju/stable
endif
	@echo Installing dependencies
	sudo apt-get update
	@sudo apt-get --force-yes install $(strip $(DEPENDENCIES)) \
	$(shell apt-cache madison juju-mongodb mongodb-server | head -1 | cut -d '|' -f1)
else
	@echo sysdeps runs only on systems with apt-get
	@echo on OS X with homebrew try: brew install bazaar mongodb
endif

deb:
	make deps GOPATH=${CURDIR}
	mkdir -p src/${PROJECT}
	-for f in `find . -maxdepth 1 | grep -Ev '^\.$$|\.git|bin|debian|pkg|src'`  ; do cp -a $$f src/${PROJECT}/ ; done
	GOPATH=${CURDIR} fakeroot debian/rules clean build binary

# Install binaries to system location.
system-install: install
	mkdir -p $(DESTDIR)/usr/bin $(DESTDIR)/etc/jemd
	# When we get a jemd and config.yaml uncomment these lines
	# and be sure to add debian/upstart and debian/preinst
	#install ${GOPATH}/bin/jemd $(DESTDIR)/usr/bin
	#install cmd/jemd/config.yaml $(DESTDIR)/etc/jemd/config.yaml.sample


deb-clean:
	-$(RM) -rf src bin pkg

help:
	@echo -e 'Identity service - list of make targets:\n'
	@echo 'make - Build the package.'
	@echo 'make check - Run tests.'
	@echo 'make install - Install the package.'
	@echo 'make server - Start the JEM server.'
	@echo 'make clean - Remove object files from package source directories.'
	@echo 'make deb - Create a debian package.'
	@echo 'make system-install - Install to system paths instead of GOPATH.'
	@echo 'make deb-clean - Remove debian package GOPATH.'
	@echo 'make sysdeps - Install the development environment system packages.'
	@echo 'make deps - Set up the project Go dependencies.'
	@echo 'make create-deps - Generate the Go dependencies file.'
	@echo 'make format - Format the source files.'
	@echo 'make simplify - Format and simplify the source files.'

.PHONY: build check install clean format server simplify sysdeps help FORCE

FORCE:
