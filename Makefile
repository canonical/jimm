# Copyright 2014 Canonical Ltd.
# Makefile for the JIMM service.

export GO111MODULE=on
export DOCKER_BUILDKIT=1

PROJECT := github.com/canonical/jimm

GIT_COMMIT := $(shell git rev-parse --verify HEAD)
GIT_VERSION := $(shell git describe --abbrev=0 --dirty)
GO_VERSION := $(shell go list -f {{.GoVersion}} -m)
ARCH := $(shell dpkg --print-architecture)

default: build

build: version/commit.txt version/version.txt
	go build -tags version $(PROJECT)/...

build/server: version/commit.txt version/version.txt
	go build -tags version ./cmd/jimmsrv

check: version/commit.txt version/version.txt
	go test -timeout 30m $(PROJECT)/... -cover

clean:
	go clean $(PROJECT)/...
	-$(RM) version/commit.txt version/version.txt
	-$(RM) jimmsrv
	-$(RM) -r jimm-release/
	-$(RM) jimm-*.tar.xz

certs:
	@cd local/traefik/certs; ./certs.sh; cd -

test-env: sysdeps certs
	@touch ./local/vault/approle.json && touch ./local/vault/roleid.txt
	@docker compose up --force-recreate -d --wait

test-env-cleanup:
	@docker compose down -v --remove-orphans

dev-env-setup: sysdeps certs
	@touch ./local/vault/approle.json && touch ./local/vault/roleid.txt
	@make version/commit.txt && make version/version.txt
	@go mod vendor

dev-env:
	@docker compose --profile dev up --force-recreate

dev-env-cleanup:
	@docker compose down -v --remove-orphans

# Reformat all source files.
format:
	gofmt -w -l .

# Reformat and simplify source files.
simplify:
	gofmt -w -l -s .

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

jimm-image:
	docker build --target deploy-env \
	--build-arg="GIT_COMMIT=$(GIT_COMMIT)" \
	--build-arg="VERSION=$(GIT_VERSION)" \
	--build-arg="GO_VERSION=$(GO_VERSION)" \
	--build-arg="ARCH=$(ARCH)" \
	--tag jimm:latest .

jimm-snap:
	mkdir -p ./snap
	cp ./snaps/jimm/snapcraft.yaml ./snap/
	snapcraft

jimmctl-snap:
	mkdir -p ./snap
	cp -R ./snaps/jimmctl/* ./snap/
	snapcraft

jaas-snap:
	mkdir -p ./snap
	cp -R ./snaps/jaas/* ./snap/
	snapcraft

push-microk8s: jimm-image
	docker tag jimm:latest localhost:32000/jimm:latest
	docker push localhost:32000/jimm:latest

get-local-auth:
	@go run ./local/authy

define check_dep
    if ! which $(1) > /dev/null; then\
		echo "$(2)";\
	else\
		echo "Detected $(1)";\
	fi
endef

# Install packages required to develop JIMM and run tests.
APT_BASED := $(shell command -v apt-get >/dev/null; echo $$?)
sysdeps:
ifeq ($(APT_BASED),0)
	@$(call check_dep,go,Missing Go - install from https://go.dev/doc/install or 'sudo snap install go')
	@$(call check_dep,git,Missing Git - install with 'sudo apt install git')
	@$(call check_dep,gcc,Missing gcc - install with 'sudo apt install build-essentials')
	@$(call check_dep,docker,Missing Docker - install from https://docs.docker.com/engine/install/')
	@$(call check_dep,docker-compose,Missing Docker Compose - install from https://docs.docker.com/engine/install/')
	@$(call check_dep,juju-db.mongo,Missing juju-db - install with 'sudo snap install juju-db --channel=4.4/stable')
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
	@echo 'make get-local-auth - Get local auth to the API WSS endpoint locally.'

.PHONY: build check install release clean format server simplify sysdeps help FORCE

FORCE:
