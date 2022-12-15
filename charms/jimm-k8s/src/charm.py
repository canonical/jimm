#!/usr/bin/env python3
# This file is part of the JIMM k8s Charm for Juju.
# Copyright 2022 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but
# WITHOUT ANY WARRANTY; without even the implied warranties of
# MERCHANTABILITY, SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR
# PURPOSE.  See the GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program. If not, see <http://www.gnu.org/licenses/>.


import functools
import hashlib
import json
import logging
import os

import hvac
from charms.data_platform_libs.v0.database_requires import (
    DatabaseEvent,
    DatabaseRequires,
)
from charms.openfga_k8s.v0.openfga import (
    OpenFGARequires,
    OpenFGAStoreCreateEvent,
)
from charms.tls_certificates_interface.v1.tls_certificates import (
    CertificateAvailableEvent,
    CertificateExpiringEvent,
    CertificateRevokedEvent,
    TLSCertificatesRequiresV1,
    generate_csr,
    generate_private_key,
)
from charms.traefik_k8s.v1.ingress import (
    IngressPerAppReadyEvent,
    IngressPerAppRequirer,
    IngressPerAppRevokedEvent,
)
from ops import pebble
from ops.charm import CharmBase, RelationChangedEvent, RelationJoinedEvent
from ops.framework import StoredState
from ops.main import main
from ops.model import (
    ActiveStatus,
    BlockedStatus,
    MaintenanceStatus,
    ModelError,
    WaitingStatus,
)
from state import PeerRelationState, RelationNotReadyError

logger = logging.getLogger(__name__)

WORKLOAD_CONTAINER = "jimm"

REQUIRED_SETTINGS = [
    "JIMM_UUID",
    "JIMM_DNS_NAME",
    "JIMM_DSN",
    "CANDID_URL",
]

STATE_KEY_CA = "ca"
STATE_KEY_CERTIFICATE = "certificate"
STATE_KEY_CHAIN = "chain"
STATE_KEY_CSR = "csr"
STATE_KEY_DSN = "dsn"
STATE_KEY_DNS_NAME = "dns"
STATE_KEY_PRIVATE_KEY = "private-key"
OPENFGA_STORE_ID = "openfga-store-id"
OPENFGA_TOKEN = "openfga-token"
OPENFGA_ADDRESS = "openfga-address"
OPENFGA_PORT = "openfga-port"
OPENFGA_SCHEME = "openfga-scheme"


class JimmOperatorCharm(CharmBase):
    """JIMM Operator Charm."""

    def __init__(self, *args):
        super().__init__(*args)
        self.framework.observe(
            self.on.jimm_pebble_ready, self._on_jimm_pebble_ready
        )
        self.framework.observe(self.on.config_changed, self._on_config_changed)
        self.framework.observe(self.on.update_status, self._on_update_status)
        self.framework.observe(self.on.leader_elected, self._on_leader_elected)
        self.framework.observe(self.on.start, self._on_start)
        self.framework.observe(self.on.stop, self._on_stop)

        self.framework.observe(
            self.on.dashboard_relation_joined,
            self._on_dashboard_relation_joined,
        )

        # Certificates relation
        self.certificates = TLSCertificatesRequiresV1(self, "certificates")
        self.framework.observe(
            self.on.certificates_relation_joined,
            self._on_certificates_relation_joined,
        )
        self.framework.observe(
            self.certificates.on.certificate_available,
            self._on_certificate_available,
        )
        self.framework.observe(
            self.certificates.on.certificate_expiring,
            self._on_certificate_expiring,
        )
        self.framework.observe(
            self.certificates.on.certificate_revoked,
            self._on_certificate_revoked,
        )

        # Ingress relation
        self.ingress = IngressPerAppRequirer(self, port=8080)
        self.framework.observe(self.ingress.on.ready, self._on_ingress_ready)
        self.framework.observe(
            self.ingress.on.revoked, self._on_ingress_revoked
        )

        # Database relation
        self.database = DatabaseRequires(
            self,
            relation_name="database",
            database_name="jimm",
        )
        self.framework.observe(
            self.database.on.database_created, self._on_database_event
        )
        self.framework.observe(
            self.database.on.endpoints_changed,
            self._on_database_event,
        )
        self.framework.observe(
            self.on.database_relation_broken, self._on_database_relation_broken
        )

        self.openfga = OpenFGARequires(self, "jimm")
        self.framework.observe(
            self.openfga.on.openfga_store_created,
            self._on_openfga_store_created,
        )

        self._local_agent_filename = "agent.json"
        self._local_vault_secret_filename = "vault_secret.js"
        self._agent_filename = "/root/config/agent.json"
        self._vault_secret_filename = "/root/config/vault_secret.json"
        self._dashboard_path = "/root/dashboard"
        self._dashboard_hash_path = "/root/dashboard/hash"

        self.state = PeerRelationState(self.model, self.app, "jimm")

    def _on_jimm_pebble_ready(self, event):
        self._update_workload(event)

    def _on_config_changed(self, event):
        self._update_workload(event)

    def _on_leader_elected(self, event):
        if self.unit.is_leader():
            try:
                # generate the private key if one is not present in the
                # application data bucket of the peer relation
                if not self.state.get(STATE_KEY_PRIVATE_KEY):
                    private_key: bytes = generate_private_key(key_size=4096)
                    self.state.set(STATE_KEY_PRIVATE_KEY, private_key.decode())

            except RelationNotReadyError:
                event.defer()
                return

        self._update_workload(event)

    def _ensure_bakery_agent_file(self, event):
        # we create the file containing agent keys if needed.
        if not self._path_exists_in_workload(self._agent_filename):
            url = self.config.get("candid-url", "")
            username = self.config.get("candid-agent-username", "")
            private_key = self.config.get("candid-agent-private-key", "")
            public_key = self.config.get("candid-agent-public-key", "")
            if not url or not username or not private_key or not public_key:
                return ""
            data = {
                "key": {"public": public_key, "private": private_key},
                "agents": [{"url": url, "username": username}],
            }
            agent_data = json.dumps(data)

            self._push_to_workload(self._agent_filename, agent_data, event)

    def _ensure_vault_config(self, event):
        addr = self.config.get("vault-url", "")
        if not addr:
            return

        # we create the file containing vault secretes if needed.
        if not self._path_exists_in_workload(self._vault_secret_filename):
            role_id = self.config.get("vault-role-id", "")
            if not role_id:
                return
            token = self.config.get("vault-token", "")
            if not token:
                return
            client = hvac.Client(url=addr, token=token)
            secret = client.sys.unwrap()
            secret["data"]["role_id"] = role_id

            secret_data = json.dumps(secret)
            self._push_to_workload(
                self._vault_secret_filename, secret_data, event
            )

    def _update_workload(self, event):
        """' Update workload with all available configuration
        data."""

        container = self.unit.get_container(WORKLOAD_CONTAINER)
        if not container.can_connect():
            logger.info(
                "cannot connect to the workload container - deferring the event"
            )
            event.defer()
            return

        self._ensure_bakery_agent_file(event)
        self._ensure_vault_config(event)
        self._install_dashboard(event)

        dnsname = "{}.{}-endpoints.{}.svc.cluster.local".format(
            self.unit.name.replace("/", "-"), self.app.name, self.model.name
        )
        dsn = ""
        try:
            if self.state.get(STATE_KEY_DNS_NAME):
                dnsname = self.state.get(STATE_KEY_DNS_NAME)
            dsn = self.state.get(STATE_KEY_DSN)
        except RelationNotReadyError:
            event.defer()
            return

        config_values = {
            "CANDID_PUBLIC_KEY": self.config.get("candid-public-key", ""),
            "CANDID_URL": self.config.get("candid-url", ""),
            "JIMM_ADMINS": self.config.get("controller-admins", ""),
            "JIMM_DNS_NAME": dnsname,
            "JIMM_LOG_LEVEL": self.config.get("log-level", ""),
            "JIMM_UUID": self.config.get("uuid", ""),
            "JIMM_DASHBOARD_LOCATION": self.config.get(
                "juju-dashboard-location", "https://jaas.ai/models"
            ),
            "JIMM_LISTEN_ADDR": ":8080",
        }
        if dsn:
            config_values["JIMM_DSN"] = dsn

        if container.exists(self._agent_filename):
            config_values["BAKERY_AGENT_FILE"] = self._agent_filename

        if container.exists(self._vault_secret_filename):
            config_values["VAULT_ADDR"] = self.config.get("vault-url", "")
            config_values["VAULT_PATH"] = "charm-jimm-creds"
            config_values["VAULT_SECRET_FILE"] = self._vault_secret_filename
            config_values["VAULT_AUTH_PATH"] = "/auth/approle/login"

        if self.model.unit.is_leader():
            config_values["JIMM_WATCH_CONTROLLERS"] = "1"

        if container.exists(self._dashboard_path):
            config_values["JIMM_DASHBOARD_LOCATION"] = self._dashboard_path

        # remove empty configuration values
        config_values = {
            key: value for key, value in config_values.items() if value
        }

        pebble_layer = {
            "summary": "jimm layer",
            "description": "pebble config layer for jimm",
            "services": {
                "jimm": {
                    "override": "merge",
                    "summary": "JAAS Intelligent Model Manager",
                    "command": "/root/jimmsrv",
                    "startup": "disabled",
                    "environment": config_values,
                }
            },
            "checks": {
                "jimm-check": {
                    "override": "replace",
                    "period": "1m",
                    "http": {"url": "http://localhost:8080/debug/status"},
                }
            },
        }
        container.add_layer("jimm", pebble_layer, combine=True)
        if self._ready():
            if container.get_service("jimm").is_running():
                container.replan()
            else:
                container.start("jimm")
            self.unit.status = ActiveStatus("running")
        else:
            logger.info("workload container not ready - defering")
            event.defer()

        dashboard_relation = self.model.get_relation("dashboard")
        if dashboard_relation:
            dashboard_relation.data[self.app].update(
                {
                    "controller-url": dnsname,
                    "identity-provider-url": self.config["candid-url"],
                    "is-juju": str(False),
                }
            )

    def _on_start(self, event):
        """Start JIMM."""
        self._update_workload(event)

    def _on_stop(self, _):
        """Stop JIMM."""
        container = self.unit.get_container(WORKLOAD_CONTAINER)
        if container.can_connect():
            container.stop()
        self._ready()

    def _on_update_status(self, _):
        """Update the status of the charm."""
        self._ready()

    def _on_dashboard_relation_joined(self, event: RelationJoinedEvent):
        dnsname = "{}.{}-endpoints.{}.svc.cluster.local".format(
            self.unit.name.replace("/", "-"), self.app.name, self.model.name
        )
        try:
            if self.state.get(STATE_KEY_DNS_NAME):
                dnsname = self.state.get(STATE_KEY_DNS_NAME)
        except RelationNotReadyError:
            event.defer()
            return

        event.relation.data[self.app].update(
            {
                "controller-url": dnsname,
                "identity-provider-url": self.config["candid-url"],
                "is-juju": str(False),
            }
        )

    def _on_database_event(self, event: DatabaseEvent) -> None:
        """Database event handler."""

        # get the first endpoint from a comma separate list
        ep = event.endpoints.split(",", 1)[0]
        # compose the db connection string
        uri = f"postgresql://{event.username}:{event.password}@{ep}/openfga"

        # record the connection string
        try:
            self.state.set(STATE_KEY_DSN, uri)
        except RelationNotReadyError:
            event.defer()
            return

        self._update_workload(event)

    def _on_database_relation_broken(self, event: DatabaseEvent) -> None:
        """Database relation broken handler."""

        # when the database relation is broken, we unset the
        # connection string and schema-created from the application
        # bucket of the peer relation
        try:
            self.state.unset(STATE_KEY_DSN)
        except RelationNotReadyError:
            event.defer()
            return
        self._update_workload(event)

    def _ready(self):
        container = self.unit.get_container(WORKLOAD_CONTAINER)

        if container.can_connect():
            plan = container.get_plan()
            if plan.services.get("jimm") is None:
                logger.error("waiting for service")
                self.unit.status = WaitingStatus("waiting for service")
                return False

            env_vars = plan.services.get("jimm").environment

            for setting in REQUIRED_SETTINGS:
                if not env_vars.get(setting, ""):
                    self.unit.status = BlockedStatus(
                        "{} configuration value not set".format(setting),
                    )
                    return False

            if container.get_service("jimm").is_running():
                self.unit.status = ActiveStatus("running")
            else:
                self.unit.status = WaitingStatus("stopped")
            return True
        else:
            logger.error("cannot connect to workload container")
            self.unit.status = WaitingStatus("waiting for jimm workload")
            return False

    def _install_dashboard(self, event):
        container = self.unit.get_container(WORKLOAD_CONTAINER)

        # if we can't connect to the container we should defer
        # this event.
        if not container.can_connect():
            event.defer()

        # fetch the resource filename
        try:
            dashboard_file = self.model.resources.fetch("dashboard")
        except ModelError:
            dashboard_file = None

        # if the resource is not specified, we can return
        # as there is nothing to install.
        if not dashboard_file:
            return

        # if the resource file is empty, we can return
        # as there is nothing to install.
        if os.path.getsize(dashboard_file) == 0:
            return

        dashboard_changed = False

        # compute the hash of the dashboard tarball.
        dashboard_hash = self._hash(dashboard_file)

        # check if we the file containing the dashboard
        # hash exists.
        if container.exists(self._dashboard_hash_path):
            # if it does, compare the stored hash with the
            # hash of the dashboard tarball.
            hash = container.pull(self._dashboard_hash_path)
            existing_hash = str(hash.read())
            # if the two hashes do not match
            # the resource must have changed.
            if not dashboard_hash == existing_hash:
                dashboard_changed = True
        else:
            dashboard_changed = True

        # if the resource file has not changed, we can
        # return as there is no need to push the same
        # dashboard content to the container.
        if not dashboard_changed:
            return

        self.unit.status = MaintenanceStatus("installing dashboard")

        # remove the existing dashboard from the workload/
        if container.exists(self._dashboard_path):
            container.remove_path(self._dashboard_path)

        container.make_dir(self._dashboard_path, make_parents=True)

        with open(dashboard_file, "rb") as f:
            container.push(
                os.path.join(self._dashboard_path, "dashboard.tar.bz2"), f
            )

        process = container.exec(
            [
                "tar",
                "xvf",
                os.path.join(self._dashboard_path, "dashboard.tar.bz2"),
                "-C",
                self._dashboard_path,
            ]
        )
        try:
            process.wait_output()
        except pebble.ExecError as e:
            logger.error(
                "error running untaring the dashboard. error code {}".format(
                    e.exit_code
                )
            )
            for line in e.stderr.splitlines():
                logger.error("    %s", line)

        self._push_to_workload(
            self._dashboard_hash_path, dashboard_hash, event
        )

    def _path_exists_in_workload(self, path: str):
        """Returns true if the specified path exists in the
        workload container."""
        container = self.unit.get_container(WORKLOAD_CONTAINER)
        if container.can_connect():
            return container.exists(path)
        return False

    def _push_to_workload(self, filename, content, event):
        """Create file on the workload container with
        the specified content."""

        container = self.unit.get_container(WORKLOAD_CONTAINER)
        if container.can_connect():
            logger.info(
                "pushing file {} to the workload containe".format(filename)
            )
            container.push(filename, content, make_dirs=True)
        else:
            logger.info("workload container not ready - defering")
            event.defer()

    def _hash(self, filename):
        BUF_SIZE = 65536
        md5 = hashlib.md5()

        with open(filename, "rb") as f:
            while True:
                data = f.read(BUF_SIZE)
                if not data:
                    break
                md5.update(data)
            return md5.hexdigest()

    def _on_openfga_store_created(self, event: OpenFGAStoreCreateEvent):
        if not event.store_id:
            return

        if self.unit.is_leader():
            try:
                self.state.set(OPENFGA_STORE_ID, event.store_id)
                self.state.set(OPENFGA_TOKEN, event.token)
                self.state.set(OPENFGA_ADDRESS, event.address)
                self.state.set(OPENFGA_PORT, event.port)
                self.state.set(OPENFGA_SCHEME, event.scheme)
            except RelationNotReadyError:
                event.defer()
                return

        self._update_workload()

    def _on_certificates_relation_joined(
        self, event: RelationJoinedEvent
    ) -> None:
        if not self.unit.is_leader():
            return

        dnsname = "{}.{}-endpoints.{}.svc.cluster.local".format(
            self.unit.name.replace("/", "-"),
            self.app.name,
            self.model.name,
        )
        try:
            if self.state.get(STATE_KEY_DNS_NAME):
                dnsname = self.state.get(STATE_KEY_DNS_NAME)

            private_key = self.state.get(STATE_KEY_PRIVATE_KEY)
            csr = generate_csr(
                private_key=private_key.encode(),
                subject=dnsname,
            )

            self.state.set(STATE_KEY_CSR, csr.decode())

            self.certificates.request_certificate_creation(
                certificate_signing_request=csr
            )
        except RelationNotReadyError:
            event.defer()
            return

    def _on_certificate_available(
        self, event: CertificateAvailableEvent
    ) -> None:
        if self.unit.is_leader():
            try:
                self.state.set(STATE_KEY_CERTIFICATE, event.certificate)
                self.state.set(STATE_KEY_CA, event.ca)
                self.state.set(STATE_KEY_CHAIN, event.chain)

            except RelationNotReadyError:
                event.defer()
                return

        self._update_workload(event)

    def _on_certificate_expiring(
        self, event: CertificateExpiringEvent
    ) -> None:
        if self.unit.is_leader():
            old_csr = ""
            private_key = ""
            dnsname = "{}.{}-endpoints.{}.svc.cluster.local".format(
                self.unit.name.replace("/", "-"),
                self.app.name,
                self.model.name,
            )
            try:
                old_csr = self.state.get(STATE_KEY_CSR)
                private_key = self.state.get(STATE_KEY_PRIVATE_KEY)
                if self.state.get(STATE_KEY_DNS_NAME):
                    dnsname = self.state.get(STATE_KEY_DNS_NAME)

                new_csr = generate_csr(
                    private_key=private_key.encode(),
                    subject=dnsname,
                )
                self.certificates.request_certificate_renewal(
                    old_certificate_signing_request=old_csr,
                    new_certificate_signing_request=new_csr,
                )
                self.state.set(STATE_KEY_CSR, new_csr.decode())
            except RelationNotReadyError:
                event.defer()
                return

        self._update_workload()

    def _on_certificate_revoked(self, event: CertificateRevokedEvent) -> None:
        if self.unit.is_leader():
            old_csr = ""
            private_key = ""
            dnsname = "{}.{}-endpoints.{}.svc.cluster.local".format(
                self.unit.name.replace("/", "-"),
                self.app.name,
                self.model.name,
            )
            try:
                old_csr = self.state.get(STATE_KEY_CSR)
                private_key = self.state.get(STATE_KEY_PRIVATE_KEY)
                if self.state.get(STATE_KEY_DNS_NAME):
                    dnsname = self.state.get(STATE_KEY_DNS_NAME)
            except RelationNotReadyError:
                event.defer()
                return

            new_csr = generate_csr(
                private_key=private_key.encode(),
                subject=dnsname,
            )
            self.certificates.request_certificate_renewal(
                old_certificate_signing_request=old_csr,
                new_certificate_signing_request=new_csr,
            )
            try:
                self.state.set(STATE_KEY_CSR, new_csr.decode())
                self.state.unset(
                    STATE_KEY_CERTIFICATE, STATE_KEY_CA, STATE_KEY_CHAIN
                )
            except RelationNotReadyError:
                event.defer()
                return

        self.unit.status = WaitingStatus("Waiting for new certificate")
        self._update_workload()

    def _on_ingress_ready(self, event: IngressPerAppReadyEvent):
        if self.unit.is_leader():
            try:
                self.state.set(STATE_KEY_DNS_NAME, event.url)
            except RelationNotReadyError:
                event.defer()
                return

        self._update_workload(event)

    def _on_ingress_revoked(self, event: IngressPerAppRevokedEvent):
        if self.unit.is_leader():
            try:
                self.state.unset(STATE_KEY_DNS_NAME)
            except RelationNotReadyError:
                event.defer()
                return

        self._update_workload(event)


if __name__ == "__main__":
    main(JimmOperatorCharm)
