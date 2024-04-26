#!/usr/bin/env python3
# Copyright 2023 Canonical Ltd.
# See LICENSE file for licensing details.

"""Library for the vault-kv relation.

This library contains the Requires and Provides classes for handling the vault-kv
interface.

## Getting Started
From a charm directory, fetch the library using `charmcraft`:

```shell
charmcraft fetch-lib charms.vault_k8s.v0.vault_kv
```

### Requirer charm
The requirer charm is the charm requiring a secret value store. In this example, the requirer charm
is requiring a secret value store.

```python
import secrets

from charms.vault_k8s.v0 import vault_kv
from ops.charm import CharmBase, InstallEvent
from ops.main import main
from ops.model import ActiveStatus, BlockedStatus

NONCE_SECRET_LABEL = "nonce"


class ExampleRequirerCharm(CharmBase):
    def __init__(self, *args):
        super().__init__(*args)
        self.interface = vault_kv.VaultKvRequires(
            self,
            "vault-kv",
            "my-suffix",
        )

        self.framework.observe(self.on.install, self._on_install)
        self.framework.observe(self.interface.on.connected, self._on_connected)
        self.framework.observe(self.interface.on.ready, self._on_ready)
        self.framework.observe(self.interface.on.gone_away, self._on_gone_away)
        self.framework.observe(self.on.update_status, self._on_update_status)

    def _on_install(self, event: InstallEvent):
        self.unit.add_secret(
            {"nonce": secrets.token_hex(16)},
            label=NONCE_SECRET_LABEL,
            description="Nonce for vault-kv relation",
        )
        self.unit.status = BlockedStatus("Waiting for vault-kv relation")

    def _on_connected(self, event: vault_kv.VaultKvConnectedEvent):
        relation = self.model.get_relation(event.relation_name, event.relation_id)
        egress_subnet = str(self.model.get_binding(relation).network.interfaces[0].subnet)
        self.interface.request_credentials(relation, egress_subnet, self.get_nonce())

    def _on_ready(self, event: vault_kv.VaultKvReadyEvent):
        relation = self.model.get_relation(event.relation_name, event.relation_id)
        if relation is None:
            return
        vault_url = self.interface.get_vault_url(relation)
        ca_certificate = self.interface.get_ca_certificate(relation)
        mount = self.interface.get_mount(relation)

        unit_credentials = self.interface.get_unit_credentials(relation)
        # unit_credentials is a juju secret id
        secret = self.model.get_secret(id=unit_credentials)
        secret_content = secret.get_content()
        role_id = secret_content["role-id"]
        role_secret_id = secret_content["role-secret-id"]

        self._configure(vault_url, ca_certificate, mount, role_id, role_secret_id)

        self.unit.status = ActiveStatus()

    def _on_gone_away(self, event: vault_kv.VaultKvGoneAwayEvent):
        self.unit.status = BlockedStatus("Waiting for vault-kv relation")

    def _configure(
        self,
        vault_url: str,
        ca_certificate: str,
        mount: str,
        role_id: str,
        role_secret_id: str,
    ):
        pass

    def _on_update_status(self, event):
        # Check somewhere that egress subnet has not changed i.e. pod has not been rescheduled
        # Update status might not be the best place
        binding = self.model.get_binding("vault-kv")
        if binding is not None:
            egress_subnet = str(binding.network.interfaces[0].subnet)
            self.interface.request_credentials(event.relation, egress_subnet, self.get_nonce())

    def get_nonce(self):
        secret = self.model.get_secret(label=NONCE_SECRET_LABEL)
        nonce = secret.get_content()["nonce"]
        return nonce


if __name__ == "__main__":
    main(ExampleRequirerCharm)
```

You can integrate both charms by running:

```bash
juju integrate <vault provider charm> <vault requirer charm>
```
"""

import json
import logging
from dataclasses import dataclass
from typing import Any, Dict, Iterable, List, Mapping, Optional, Union

import ops
from interface_tester.schema_base import DataBagSchema  # type: ignore[import-untyped]
from pydantic import BaseModel, Field, Json, ValidationError

logger = logging.getLogger(__name__)


# The unique Charmhub library identifier, never change it
LIBID = "591d6d2fb6a54853b4bb53ef16ef603a"

# Increment this major API version when introducing breaking changes
LIBAPI = 0

# Increment this PATCH version before using `charmcraft publish-lib` or reset
# to 0 if you are raising the major API version
LIBPATCH = 4

PYDEPS = ["pydantic", "pytest-interface-tester"]


class VaultKvProviderSchema(BaseModel):
    """Provider side of the vault-kv interface."""

    vault_url: str = Field(description="The URL of the Vault server to connect to.")
    mount: str = Field(
        description=(
            "The KV mount available for the requirer application, "
            "respecting the pattern 'charm-<requirer app>-<user provided suffix>'."
        )
    )
    ca_certificate: str = Field(description="The CA certificate to use when validating the Vault server's certificate.")
    credentials: Json[Mapping[str, str]] = Field(
        description=(
            "Mapping of unit name and credentials for that unit."
            " Credentials are a juju secret containing a 'role-id' and a 'role-secret-id'."
        )
    )


class AppVaultKvRequirerSchema(BaseModel):
    """App schema of the requirer side of the vault-kv interface."""

    mount_suffix: str = Field(description="Suffix to append to the mount name to get the KV mount.")


class UnitVaultKvRequirerSchema(BaseModel):
    """Unit schema of the requirer side of the vault-kv interface."""

    egress_subnet: str = Field(description="Egress subnet to use, in CIDR notation.")
    nonce: str = Field(description="Uniquely identifying value for this unit. `secrets.token_hex(16)` is recommended.")


class ProviderSchema(DataBagSchema):
    """The schema for the provider side of this interface."""

    app: VaultKvProviderSchema  # type: ignore


class RequirerSchema(DataBagSchema):
    """The schema for the requirer side of this interface."""

    app: AppVaultKvRequirerSchema  # type: ignore
    unit: UnitVaultKvRequirerSchema  # type: ignore


@dataclass
class KVRequest:
    """This class represents a kv request from an interface Requirer."""

    relation_id: int
    app_name: str
    unit_name: str
    mount_suffix: str
    egress_subnet: str
    nonce: str


def is_requirer_data_valid(app_data: Mapping[str, str], unit_data: Mapping[str, str]) -> bool:
    """Return whether the requirer data is valid."""
    try:
        RequirerSchema(
            app=AppVaultKvRequirerSchema(**app_data),
            unit=UnitVaultKvRequirerSchema(**unit_data),
        )
        return True
    except ValidationError as e:
        logger.debug("Invalid data: %s", e)
        return False


def is_provider_data_valid(data: Mapping[str, str]) -> bool:
    """Return whether the provider data is valid."""
    try:
        ProviderSchema(app=VaultKvProviderSchema(**data))  # type: ignore https://github.com/pydantic/pydantic/issues/8616
        return True
    except ValidationError as e:
        logger.debug("Invalid data: %s", e)
        return False


class NewVaultKvClientAttachedEvent(ops.EventBase):
    """New vault kv client attached event."""

    def __init__(
        self,
        handle: ops.Handle,
        relation_id: int,
        app_name: str,
        unit_name: str,
        mount_suffix: str,
        egress_subnet: str,
        nonce: str,
    ):
        super().__init__(handle)
        self.relation_id = relation_id
        self.app_name = app_name
        self.unit_name = unit_name
        self.mount_suffix = mount_suffix
        self.egress_subnet = egress_subnet
        self.nonce = nonce

    def snapshot(self) -> dict:
        """Return snapshot data that should be persisted."""
        return {
            "relation_id": self.relation_id,
            "app_name": self.app_name,
            "unit_name": self.unit_name,
            "mount_suffix": self.mount_suffix,
            "egress_subnet": self.egress_subnet,
            "nonce": self.nonce,
        }

    def restore(self, snapshot: Dict[str, Any]):
        """Restore the value state from a given snapshot."""
        super().restore(snapshot)
        self.relation_id = snapshot["relation_id"]
        self.app_name = snapshot["app_name"]
        self.unit_name = snapshot["unit_name"]
        self.mount_suffix = snapshot["mount_suffix"]
        self.egress_subnet = snapshot["egress_subnet"]
        self.nonce = snapshot["nonce"]


class VaultKvProviderEvents(ops.ObjectEvents):
    """List of events that the Vault Kv provider charm can leverage."""

    new_vault_kv_client_attached = ops.EventSource(NewVaultKvClientAttachedEvent)


class VaultKvProvides(ops.Object):
    """Class to be instanciated by the providing side of the relation."""

    on = VaultKvProviderEvents()  # type: ignore

    def __init__(
        self,
        charm: ops.CharmBase,
        relation_name: str,
    ) -> None:
        super().__init__(charm, relation_name)
        self.charm = charm
        self.relation_name = relation_name
        self.framework.observe(
            self.charm.on[relation_name].relation_changed,
            self._on_relation_changed,
        )

    def _on_relation_changed(self, event: ops.RelationChangedEvent):
        """Handle client changed relation.

        This handler will emit a new_vault_kv_client_attached event for each requiring unit
        with valid relation data.
        """
        if event.app is None:
            logger.debug("No remote application yet")
            return
        app_data = event.relation.data[event.app]
        for unit in event.relation.units:
            if not is_requirer_data_valid(app_data, event.relation.data[unit]):
                logger.debug("Invalid data from unit %r", unit.name)
                continue
            self.on.new_vault_kv_client_attached.emit(
                relation_id=event.relation.id,
                app_name=event.app.name,
                unit_name=unit.name,
                mount_suffix=event.relation.data[event.app]["mount_suffix"],
                egress_subnet=event.relation.data[unit]["egress_subnet"],
                nonce=event.relation.data[unit]["nonce"],
            )

    def set_vault_url(self, relation: ops.Relation, vault_url: str):
        """Set the vault_url on the relation."""
        if not self.charm.unit.is_leader():
            return

        relation.data[self.charm.app]["vault_url"] = vault_url

    def set_ca_certificate(self, relation: ops.Relation, ca_certificate: str):
        """Set the ca_certificate on the relation."""
        if not self.charm.unit.is_leader():
            return
        if not relation:
            logger.warning("Relation is None")
            return
        if not relation.active:
            logger.warning("Relation is not active")
            return
        relation.data[self.charm.app]["ca_certificate"] = ca_certificate

    def set_mount(self, relation: ops.Relation, mount: str):
        """Set the mount on the relation."""
        if not self.charm.unit.is_leader():
            return

        relation.data[self.charm.app]["mount"] = mount

    def set_unit_credentials(self, relation: ops.Relation, nonce: str, secret: ops.Secret):
        """Set the unit credentials on the relation."""
        if not self.charm.unit.is_leader():
            return

        credentials = self.get_credentials(relation)
        if secret.id is None:
            logger.debug(
                "Secret id is None, not updating the relation '%s:%d' for nonce %r",
                relation.name,
                relation.id,
                nonce,
            )
            return
        credentials[nonce] = secret.id
        relation.data[self.charm.app]["credentials"] = json.dumps(credentials, sort_keys=True)

    def remove_unit_credentials(self, relation: ops.Relation, nonce: Union[str, Iterable[str]]):
        """Remove nonce(s) from the relation."""
        if not self.charm.unit.is_leader():
            return

        if isinstance(nonce, str):
            nonce = [nonce]

        credentials = self.get_credentials(relation)

        for n in nonce:
            credentials.pop(n, None)

        relation.data[self.charm.app]["credentials"] = json.dumps(credentials, sort_keys=True)

    def get_credentials(self, relation: ops.Relation) -> dict:
        """Get the unit credentials from the relation."""
        return json.loads(relation.data[self.charm.app].get("credentials", "{}"))

    def get_outstanding_kv_requests(self, relation_id: Optional[int] = None) -> List[KVRequest]:
        """Get the outstanding requests for the relation."""
        outstanding_requests: List[KVRequest] = []
        kv_requests = self.get_kv_requests(relation_id=relation_id)
        for request in kv_requests:
            if not self._credentials_issued_for_request(nonce=request.nonce, relation_id=relation_id):
                outstanding_requests.append(request)
        return outstanding_requests

    def get_kv_requests(self, relation_id: Optional[int] = None) -> List[KVRequest]:
        """Get all KV requests for the relation."""
        kv_requests: List[KVRequest] = []
        relations = (
            [relation for relation in self.model.relations[self.relation_name] if relation.id == relation_id]
            if relation_id is not None
            else self.model.relations.get(self.relation_name, [])
        )
        for relation in relations:
            assert isinstance(relation.app, ops.Application)
            if not relation.active:
                continue
            app_data = relation.data[relation.app]
            for unit in relation.units:
                unit_data = relation.data[unit]
                if not is_requirer_data_valid(app_data=app_data, unit_data=unit_data):
                    continue
                kv_requests.append(
                    KVRequest(
                        relation_id=relation.id,
                        app_name=relation.app.name,
                        unit_name=unit.name,
                        mount_suffix=app_data["mount_suffix"],
                        egress_subnet=unit_data["egress_subnet"],
                        nonce=unit_data["nonce"],
                    )
                )
        return kv_requests

    def _credentials_issued_for_request(self, nonce: str, relation_id: Optional[int]) -> bool:
        """Return whether credentials have been issued for the request."""
        relation = self.model.get_relation(self.relation_name, relation_id)
        if not relation:
            return False
        credentials = self.get_credentials(relation)
        return credentials.get(nonce) is not None


class VaultKvConnectedEvent(ops.EventBase):
    """VaultKvConnectedEvent Event."""

    def __init__(
        self,
        handle: ops.Handle,
        relation_id: int,
        relation_name: str,
    ):
        super().__init__(handle)
        self.relation_id = relation_id
        self.relation_name = relation_name

    def snapshot(self) -> dict:
        """Return snapshot data that should be persisted."""
        return {
            "relation_id": self.relation_id,
            "relation_name": self.relation_name,
        }

    def restore(self, snapshot: Dict[str, Any]):
        """Restore the value state from a given snapshot."""
        super().restore(snapshot)
        self.relation_id = snapshot["relation_id"]
        self.relation_name = snapshot["relation_name"]


class VaultKvReadyEvent(ops.EventBase):
    """VaultKvReadyEvent Event."""

    def __init__(
        self,
        handle: ops.Handle,
        relation_id: int,
        relation_name: str,
    ):
        super().__init__(handle)
        self.relation_id = relation_id
        self.relation_name = relation_name

    def snapshot(self) -> dict:
        """Return snapshot data that should be persisted."""
        return {
            "relation_id": self.relation_id,
            "relation_name": self.relation_name,
        }

    def restore(self, snapshot: Dict[str, Any]):
        """Restore the value state from a given snapshot."""
        super().restore(snapshot)
        self.relation_id = snapshot["relation_id"]
        self.relation_name = snapshot["relation_name"]


class VaultKvGoneAwayEvent(ops.EventBase):
    """VaultKvGoneAwayEvent Event."""

    pass


class VaultKvRequireEvents(ops.ObjectEvents):
    """List of events that the Vault Kv requirer charm can leverage."""

    connected = ops.EventSource(VaultKvConnectedEvent)
    ready = ops.EventSource(VaultKvReadyEvent)
    gone_away = ops.EventSource(VaultKvGoneAwayEvent)


class VaultKvRequires(ops.Object):
    """Class to be instanciated by the requiring side of the relation."""

    on = VaultKvRequireEvents()  # type: ignore

    def __init__(
        self,
        charm: ops.CharmBase,
        relation_name: str,
        mount_suffix: str,
    ) -> None:
        super().__init__(charm, relation_name)
        self.charm = charm
        self.relation_name = relation_name
        self.mount_suffix = mount_suffix
        self.framework.observe(
            self.charm.on[relation_name].relation_joined,
            self._on_vault_kv_relation_joined,
        )
        self.framework.observe(
            self.charm.on[relation_name].relation_changed,
            self._on_vault_kv_relation_changed,
        )
        self.framework.observe(
            self.charm.on[relation_name].relation_broken,
            self._on_vault_kv_relation_broken,
        )

    def _set_unit_nonce(self, relation: ops.Relation, nonce: str):
        """Set the nonce on the relation."""
        relation.data[self.charm.unit]["nonce"] = nonce

    def _set_unit_egress_subnet(self, relation: ops.Relation, egress_subnet: str):
        """Set the egress_subnet on the relation."""
        relation.data[self.charm.unit]["egress_subnet"] = egress_subnet

    def _on_vault_kv_relation_joined(self, event: ops.RelationJoinedEvent):
        """Handle relation joined.

        Set the secret backend in the application databag if we are the leader.
        Always update the egress_subnet in the unit databag.
        """
        if self.charm.unit.is_leader():
            event.relation.data[self.charm.app]["mount_suffix"] = self.mount_suffix
        self.on.connected.emit(
            event.relation.id,
            event.relation.name,
        )

    def _on_vault_kv_relation_changed(self, event: ops.RelationChangedEvent):
        """Handle relation changed."""
        if event.app is None:
            logger.debug("No remote application yet")
            return

        if (
            is_provider_data_valid(event.relation.data[event.app])
            and self.get_unit_credentials(event.relation) is not None
        ):
            self.on.ready.emit(
                event.relation.id,
                event.relation.name,
            )

    def _on_vault_kv_relation_broken(self, event: ops.RelationBrokenEvent):
        """Handle relation broken."""
        self.on.gone_away.emit()

    def request_credentials(self, relation: ops.Relation, egress_subnet: str, nonce: str) -> None:
        """Request credentials from the vault-kv relation.

        Generated secret ids are tied to the unit egress_subnet, so if the egress_subnet
        changes a new secret id must be generated.

        A change in egress_subnet can happen when the pod is rescheduled to a different
        node by the underlying substrate without a change from Juju.
        """
        self._set_unit_egress_subnet(relation, egress_subnet)
        self._set_unit_nonce(relation, nonce)

    def get_vault_url(self, relation: ops.Relation) -> Optional[str]:
        """Return the vault_url from the relation."""
        if relation.app is None:
            return None
        return relation.data[relation.app].get("vault_url")

    def get_ca_certificate(self, relation: ops.Relation) -> Optional[str]:
        """Return the ca_certificate from the relation."""
        if relation.app is None:
            return None
        return relation.data[relation.app].get("ca_certificate")

    def get_mount(self, relation: ops.Relation) -> Optional[str]:
        """Return the mount from the relation."""
        if relation.app is None:
            return None
        return relation.data[relation.app].get("mount")

    def get_unit_credentials(self, relation: ops.Relation) -> Optional[str]:
        """Return the unit credentials from the relation.

        Unit credentials are stored in the relation data as a Juju secret id.
        """
        nonce = relation.data[self.charm.unit].get("nonce")
        if nonce is None or relation.app is None:
            return None
        return json.loads(relation.data[relation.app].get("credentials", "{}")).get(nonce)
