CHARM?=/snap/bin/charm
CHARM_HELPERS_SYNC?=$(HOME)/src/github.com/juju/charm-helpers/tools/charm_helpers_sync/charm_helpers_sync.py

.PHONY: build
build: hooks/charmhelpers

hooks/charmhelpers: charm-helpers.yaml
	$(CHARM_HELPERS_SYNC) -c charm-helpers.yaml

.PHONY: lint
lint:
	flake8 hooks/config-changed hooks/db-relation-changed hooks/install.py hooks/jaascharm.py hooks/nrpe-relation-changed hooks/stop hooks/update-status hooks/upgrade-charm hooks/website-relation-joined

.PHONY: clean
clean:
	-rm -rf hooks/charmhelpers
