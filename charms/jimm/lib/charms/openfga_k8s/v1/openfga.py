# Copyright 2023 Canonical Ltd.
# See LICENSE file for licensing details.

"""# Interface Library for OpenFGA.

This library wraps relation endpoints using the `openfga` interface
and provides a Python API for requesting OpenFGA authorization model
stores to be created.

## Getting Started

To get started using the library, you just need to fetch the library using `charmcraft`.

```shell
cd some-charm
charmcraft fetch-lib charms.openfga_k8s.v1.openfga
```

In the `metadata.yaml` of the charm, add the following:

```yaml
requires:
  openfga:
    interface: openfga
```

Then, to initialise the library:
```python
from charms.openfga_k8s.v1.openfga import (
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
        if not event.store_id:
            return

        info = self.openfga.get_store_info()
        if not info:
            return

        logger.info("store id {}".format(info.store_id))
        logger.info("token {}".format(info.token))
        logger.info("grpc_api_url {}".format(info.grpc_api_url))
        logger.info("http_api_url {}".format(info.http_api_url))

```

The OpenFGA charm will attempt to use Juju secrets to pass the token
to the requiring charm. However if the Juju version does not support secrets it will
fall back to passing plaintext token via relation data.
"""

import json
import logging
from typing import Dict, MutableMapping, Optional, Union

import pydantic
from ops import (
    CharmBase,
    Handle,
    HookEvent,
    Relation,
    RelationCreatedEvent,
    RelationDepartedEvent,
    TooManyRelatedAppsError,
)
from ops.charm import CharmEvents, RelationChangedEvent, RelationEvent
from ops.framework import EventSource, Object
from pydantic import BaseModel, Field, validator
from typing_extensions import Self

# The unique Charmhub library identifier, never change it
LIBID = "216f28cfeea4447b8a576f01bfbecdf5"

# Increment this major API version when introducing breaking changes
LIBAPI = 1

# Increment this PATCH version before using `charmcraft publish-lib` or reset
# to 0 if you are raising the major API version
LIBPATCH = 1
PYDEPS = ["pydantic<2.0"]

logger = logging.getLogger(__name__)
BUILTIN_JUJU_KEYS = {"ingress-address", "private-address", "egress-subnets"}
RELATION_NAME = "openfga"
OPENFGA_TOKEN_FIELD = "token"


class OpenfgaError(RuntimeError):
    """Base class for custom errors raised by this library."""


class DataValidationError(OpenfgaError):
    """Raised when data validation fails on relation data."""


class DatabagModel(BaseModel):
    """Base databag model."""

    class Config:
        """Pydantic config."""

        allow_population_by_field_name = True
        """Allow instantiating this class by field name (instead of forcing alias)."""

    @classmethod
    def _load_value(cls, v: str) -> Union[Dict, str]:
        try:
            return json.loads(v)
        except json.JSONDecodeError:
            return v

    @classmethod
    def load(cls, databag: MutableMapping) -> Self:
        """Load this model from a Juju databag."""
        try:
            data = {
                k: cls._load_value(v) for k, v in databag.items() if k not in BUILTIN_JUJU_KEYS
            }
        except json.JSONDecodeError:
            logger.error(f"invalid databag contents: expecting json. {databag}")
            raise

        return cls.parse_raw(json.dumps(data))  # type: ignore

    def dump(self, databag: Optional[MutableMapping] = None) -> MutableMapping:
        """Write the contents of this model to Juju databag."""
        if databag is None:
            databag = {}

        dct = self.dict()
        for key, field in self.__fields__.items():  # type: ignore
            value = dct[key]
            if value is None:
                continue
            databag[field.alias or key] = (
                json.dumps(value) if not isinstance(value, (str)) else value
            )

        return databag


class OpenfgaRequirerAppData(DatabagModel):
    """Openfga requirer application databag model."""

    store_name: str = Field(description="The store name the application requires")


class OpenfgaProviderAppData(DatabagModel):
    """Openfga requirer application databag model."""

    store_id: Optional[str] = Field(description="The store_id", default=None)
    token: Optional[str] = Field(description="The token", default=None)
    token_secret_id: Optional[str] = Field(
        description="The juju secret_id which can be used to retrieve the token",
        default=None,
    )
    grpc_api_url: str = Field(description="The openfga server GRPC address")
    http_api_url: str = Field(description="The openfga server HTTP address")

    @validator("token_secret_id", pre=True)
    def validate_token(cls, v: str, values: Dict) -> str:  # noqa: N805
        """Validate token_secret_id arg."""
        if not v and not values["token"]:
            raise ValueError("invalid scheme: neither of token and token_secret_id were defined")
        return v


class OpenFGAStoreCreateEvent(HookEvent):
    """Event emitted when a new OpenFGA store is created."""

    def __init__(self, handle: Handle, store_id: str):
        super().__init__(handle)
        self.store_id = store_id

    def snapshot(self) -> Dict:
        """Save event."""
        return {
            "store_id": self.store_id,
        }

    def restore(self, snapshot: Dict) -> None:
        """Restore event."""
        self.store_id = snapshot["store_id"]


class OpenFGAStoreRemovedEvent(HookEvent):
    """Event emitted when a new OpenFGA store is removed."""


class OpenFGARequirerEvents(CharmEvents):
    """Custom charm events."""

    openfga_store_created = EventSource(OpenFGAStoreCreateEvent)
    openfga_store_removed = EventSource(OpenFGAStoreRemovedEvent)


class OpenFGARequires(Object):
    """This class defines the functionality for the 'requires' side of the 'openfga' relation.

    Hook events observed:
        - relation-created
        - relation-changed
        - relation-departed
    """

    on = OpenFGARequirerEvents()

    def __init__(
        self, charm: CharmBase, store_name: str, relation_name: str = RELATION_NAME
    ) -> None:
        super().__init__(charm, relation_name)
        self.charm = charm
        self.relation_name = relation_name
        self.store_name = store_name

        self.framework.observe(charm.on[relation_name].relation_created, self._on_relation_created)
        self.framework.observe(
            charm.on[relation_name].relation_changed,
            self._on_relation_changed,
        )
        self.framework.observe(
            charm.on[relation_name].relation_departed,
            self._on_relation_departed,
        )

    def _on_relation_created(self, event: RelationCreatedEvent) -> None:
        """Handle the relation-created event."""
        if not self.model.unit.is_leader():
            return

        databag = event.relation.data[self.model.app]
        OpenfgaRequirerAppData(store_name=self.store_name).dump(databag)

    def _on_relation_changed(self, event: RelationChangedEvent) -> None:
        """Handle the relation-changed event."""
        if not (app := event.relation.app):
            return
        databag = event.relation.data[app]
        try:
            data = OpenfgaProviderAppData.load(databag)
        except pydantic.ValidationError:
            return

        self.on.openfga_store_created.emit(store_id=data.store_id)

    def _on_relation_departed(self, event: RelationDepartedEvent) -> None:
        """Handle the relation-departed event."""
        self.on.openfga_store_removed.emit()

    def _get_relation(self, relation_id: Optional[int] = None) -> Optional[Relation]:
        try:
            relation = self.model.get_relation(self.relation_name, relation_id=relation_id)
        except TooManyRelatedAppsError:
            raise RuntimeError("More than one relations are defined. Please provide a relation_id")
        if not relation or not relation.app:
            return None
        return relation

    def get_store_info(self) -> Optional[OpenfgaProviderAppData]:
        """Get the OpenFGA store and server info."""
        if not (relation := self._get_relation()):
            return None
        if not relation.app:
            return None

        databag = relation.data[relation.app]
        try:
            data = OpenfgaProviderAppData.load(databag)
        except pydantic.ValidationError:
            return None

        if data.token_secret_id:
            token_secret = self.model.get_secret(id=data.token_secret_id)
            token = token_secret.get_content()["token"]
            data.token = token

        return data


class OpenFGAStoreRequestEvent(RelationEvent):
    """Event emitted when a new OpenFGA store is requested."""

    def __init__(self, handle: Handle, relation: Relation, store_name: str) -> None:
        super().__init__(handle, relation)
        self.store_name = store_name

    def snapshot(self) -> Dict:
        """Save event."""
        dct = super().snapshot()
        dct["store_name"] = self.store_name
        return dct

    def restore(self, snapshot: Dict) -> None:
        """Restore event."""
        super().restore(snapshot)
        self.store_name = snapshot["store_name"]


class OpenFGAProviderEvents(CharmEvents):
    """Custom charm events."""

    openfga_store_requested = EventSource(OpenFGAStoreRequestEvent)


class OpenFGAProvider(Object):
    """Requirer side of the openfga relation."""

    on = OpenFGAProviderEvents()

    def __init__(
        self,
        charm: CharmBase,
        relation_name: str = RELATION_NAME,
        http_port: Optional[str] = "8080",
        grpc_port: Optional[str] = "8081",
        scheme: Optional[str] = "http",
    ):
        super().__init__(charm, relation_name)
        self.charm = charm
        self.relation_name = relation_name
        self.http_port = http_port
        self.grpc_port = grpc_port
        self.scheme = scheme

        self.framework.observe(
            charm.on[relation_name].relation_changed,
            self._on_relation_changed,
        )

    def _on_relation_changed(self, event: RelationChangedEvent) -> None:
        if not (app := event.app):
            return
        data = event.relation.data[app]
        if not data:
            logger.info("No relation data available.")
            return

        try:
            data = OpenfgaRequirerAppData.load(data)
        except pydantic.ValidationError:
            return

        self.on.openfga_store_requested.emit(event.relation, store_name=data.store_name)

    def _get_http_url(self, relation: Relation) -> str:
        address = self.model.get_binding(relation).network.ingress_address.exploded
        return f"{self.scheme}://{address}:{self.http_port}"

    def _get_grpc_url(self, relation: Relation) -> str:
        address = self.model.get_binding(relation).network.ingress_address.exploded
        return f"{self.scheme}://{address}:{self.grpc_port}"

    def update_relation_info(
        self,
        store_id: str,
        grpc_api_url: Optional[str] = None,
        http_api_url: Optional[str] = None,
        token: Optional[str] = None,
        token_secret_id: Optional[str] = None,
        relation_id: Optional[int] = None,
    ) -> None:
        """Update a relation databag."""
        if not self.model.unit.is_leader():
            return

        relation = self.model.get_relation(self.relation_name, relation_id)
        if not relation or not relation.app:
            return

        if not grpc_api_url:
            grpc_api_url = self._get_grpc_url(relation=relation)
        if not http_api_url:
            http_api_url = self._get_http_url(relation=relation)

        data = OpenfgaProviderAppData(
            store_id=store_id,
            grpc_api_url=grpc_api_url,
            http_api_url=http_api_url,
            token_secret_id=token_secret_id,
            token=token,
        )
        databag = relation.data[self.charm.app]

        try:
            data.dump(databag)
        except pydantic.ValidationError as e:
            msg = "failed to validate app data"
            logger.info(msg, exc_info=True)
            raise DataValidationError(msg) from e

    def update_server_info(
        self, grpc_api_url: Optional[str] = None, http_api_url: Optional[str] = None
    ) -> None:
        """Update all the relations databags with the server info."""
        if not self.model.unit.is_leader():
            return

        for relation in self.model.relations[self.relation_name]:
            grpc_url = grpc_api_url
            http_url = http_api_url
            if not grpc_api_url:
                grpc_url = self._get_grpc_url(relation=relation)
            if not http_api_url:
                http_url = self._get_http_url(relation=relation)
            data = OpenfgaProviderAppData(grpc_api_url=grpc_url, http_api_url=http_url)

            try:
                data.dump(relation.data[self.model.app])
            except pydantic.ValidationError as e:
                msg = "failed to validate app data"
                logger.info(msg, exc_info=True)
                raise DataValidationError(msg) from e
