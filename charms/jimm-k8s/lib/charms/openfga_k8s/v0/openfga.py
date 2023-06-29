"""# Interface Library for OpenFGA

This library wraps relation endpoints using the `openfga` interface
and provides a Python API for requesting OpenFGA authorization model 
stores to be created.

## Getting Started

To get started using the library, you just need to fetch the library using `charmcraft`.

```shell
cd some-charm
charmcraft fetch-lib charms.openfga_k8s.v0.openfga
```

In the `metadata.yaml` of the charm, add the following:

```yaml
requires:
  openfga:
    interface: openfga
```

Then, to initialise the library:
```python
from charms.openfga_k8s.v0.openfga import (
    OpenFGARequires,
    OpenFGAStoreCreateEvent,
)

class SomeCharm(CharmBase):
  def __init__(self, *args):
    # ...
    self.openfga = OpenFGARequires(self, "test-openfga-store")
    self.framework.observe(
        self.openfga.on.openfga_store_created,
        self._on_openfga_store_created,
    )

    def _on_openfga_store_created(self, event: OpenFGAStoreCreateEvent):
        if not self.unit.is_leader():
            return

        if not event.store_id:
            return

        logger.info("store id {}".format(event.store_id))
        logger.info("token {}".format(event.token))
        logger.info("address {}".format(event.address))
        logger.info("port {}".format(event.port))
        logger.info("scheme {}".format(event.scheme))
```

"""

import logging

from ops.charm import (
    CharmEvents,
    RelationChangedEvent,
    RelationEvent,
    RelationJoinedEvent,
)
from ops.framework import EventSource, Object

# The unique Charmhub library identifier, never change it
LIBID = "216f28cfeea4447b8a576f01bfbecdf5"

# Increment this major API version when introducing breaking changes
LIBAPI = 0

# Increment this PATCH version before using `charmcraft publish-lib` or reset
# to 0 if you are raising the major API version
LIBPATCH = 2

logger = logging.getLogger(__name__)

RELATION_NAME = "openfga"


class OpenFGAEvent(RelationEvent):
    """Base class for OpenFGA events."""

    @property
    def store_id(self):
        return self.relation.data[self.relation.app].get("store_id")

    @property
    def token(self):
        return self.relation.data[self.relation.app].get("token")

    @property
    def address(self):
        return self.relation.data[self.relation.app].get("address")

    @property
    def scheme(self):
        return self.relation.data[self.relation.app].get("scheme")

    @property
    def port(self):
        return self.relation.data[self.relation.app].get("port")


class OpenFGAStoreCreateEvent(OpenFGAEvent):
    """
    Event emitted when a new OpenFGA store is created
    for use on this relation.
    """


class OpenFGAEvents(CharmEvents):
    """Custom charm events."""

    openfga_store_created = EventSource(OpenFGAStoreCreateEvent)


class OpenFGARequires(Object):
    """This class defines the functionality for the 'requires' side of the 'openfga' relation.

    Hook events observed:
        - relation-joined
        - relation-changed
    """

    on = OpenFGAEvents()

    def __init__(self, charm, store_name: str):
        super().__init__(charm, RELATION_NAME)

        self.framework.observe(
            charm.on[RELATION_NAME].relation_joined, self._on_relation_joined
        )
        self.framework.observe(
            charm.on[RELATION_NAME].relation_changed,
            self._on_relation_changed,
        )

        self.data = {}
        self.store_name = store_name

    def _on_relation_joined(self, event: RelationJoinedEvent):
        """Handle the relation-joined event."""
        # `self.unit` isn't available here, so use `self.model.unit`.
        if self.model.unit.is_leader():
            event.relation.data[self.model.app]["store_name"] = self.store_name

    def _on_relation_changed(self, event: RelationChangedEvent):
        """Handle the relation-changed event."""
        if self.model.unit.is_leader():
            self.on.openfga_store_created.emit(
                event.relation, app=event.app, unit=event.unit
            )
