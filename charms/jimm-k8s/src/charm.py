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


import hashlib
import json
import logging
import socket

import hvac
from charms.data_platform_libs.v0.database_requires import (
    DatabaseEvent,
    DatabaseRequires,
)
from charms.grafana_k8s.v0.grafana_dashboard import GrafanaDashboardProvider
from charms.loki_k8s.v0.loki_push_api import LogProxyConsumer
from charms.nginx_ingress_integrator.v0.nginx_route import require_nginx_route
from charms.prometheus_k8s.v0.prometheus_scrape import MetricsEndpointProvider
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
from ops.charm import CharmBase, RelationJoinedEvent
from ops.main import main
from ops.model import ActiveStatus, BlockedStatus, WaitingStatus

from state import State, requires_state, requires_state_setter

logger = logging.getLogger(__name__)

WORKLOAD_CONTAINER = "jimm"

REQUIRED_SETTINGS = [
    "JIMM_UUID",
    "JIMM_DSN",
    "CANDID_URL",
]

JIMM_SERVICE_NAME = "jimm"
DATABASE_NAME = "jimm"
LOG_FILE = "/var/log/jimm"
# This likely will just be JIMM's port.
PROMETHEUS_PORT = 8080


class JimmOperatorCharm(CharmBase):
    """JIMM Operator Charm."""

    def __init__(self, *args):
        super().__init__(*args)

        self._state = State(self.app, lambda: self.model.get_relation("peer"))
        self._unit_state = State(self.unit, lambda: self.model.get_relation("peer"))

        self.framework.observe(self.on.peer_relation_changed, self._on_peer_relation_changed)
        self.framework.observe(self.on.jimm_pebble_ready, self._on_jimm_pebble_ready)
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

        # Traefik ingress relation
        self.ingress = IngressPerAppRequirer(
            self,
            relation_name="ingress",
            port=8080,
        )
        self.framework.observe(self.ingress.on.ready, self._on_ingress_ready)
        self.framework.observe(
            self.ingress.on.revoked,
            self._on_ingress_revoked,
        )

        # Nginx ingress relation
        require_nginx_route(
            charm=self, service_hostname=self.config.get("dns-name", ""), service_name=self.app.name, service_port=8080
        )

        # Database relation
        self.database = DatabaseRequires(
            self,
            relation_name="database",
            database_name=DATABASE_NAME,
        )
        self.framework.observe(self.database.on.database_created, self._on_database_event)
        self.framework.observe(
            self.database.on.endpoints_changed,
            self._on_database_event,
        )
        self.framework.observe(self.on.database_relation_broken, self._on_database_relation_broken)

        # Vault relation
        self.framework.observe(self.on.vault_relation_joined, self._on_vault_relation_joined)
        self.framework.observe(self.on.vault_relation_changed, self._on_vault_relation_changed)

        # Grafana relation
        self._grafana_dashboards = GrafanaDashboardProvider(self, relation_name="grafana-dashboard")

        # Loki relation
        self._log_proxy = LogProxyConsumer(self, log_files=[LOG_FILE], relation_name="log-proxy")

        # Prometheus relation
        self._prometheus_scraping = MetricsEndpointProvider(
            self,
            relation_name="metrics-endpoint",
            jobs=[{"static_configs": [{"targets": [f"*:{PROMETHEUS_PORT}"]}]}],
            refresh_event=self.on.config_changed,
        )

        self._local_agent_filename = "agent.json"
        self._local_vault_secret_filename = "vault_secret.js"
        self._agent_filename = "/root/config/agent.json"
        self._vault_secret_filename = "/root/config/vault_secret.json"
        self._vault_path = "charm-jimm-k8s-creds"

    def _on_peer_relation_changed(self, event):
        self._update_workload(event)

    def _on_jimm_pebble_ready(self, event):
        self._update_workload(event)

    def _on_config_changed(self, event):
        self._update_workload(event)

    @requires_state_setter
    def _on_leader_elected(self, event):
        if not self._state.private_key:
            private_key: bytes = generate_private_key(key_size=4096)
            self._state.private_key = private_key.decode()

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

    @requires_state
    def _update_workload(self, event):
        """' Update workload with all available configuration
        data."""

        container = self.unit.get_container(WORKLOAD_CONTAINER)
        if not container.can_connect():
            logger.info("cannot connect to the workload container - deferring the event")
            event.defer()
            return

        self._ensure_bakery_agent_file(event)
        self._ensure_vault_file(event)

        dns_name = self._get_dns_name(event)
        if not dns_name:
            logger.warning("dns name not set")
            return

        config_values = {
            "CANDID_PUBLIC_KEY": self.config.get("candid-public-key", ""),
            "CANDID_URL": self.config.get("candid-url", ""),
            "JIMM_ADMINS": self.config.get("controller-admins", ""),
            "JIMM_DNS_NAME": dns_name,
            "JIMM_LOG_LEVEL": self.config.get("log-level", ""),
            "JIMM_UUID": self.config.get("uuid", ""),
            "JIMM_DASHBOARD_LOCATION": self.config.get("juju-dashboard-location", "https://jaas.ai/models"),
            "JIMM_LISTEN_ADDR": ":8080",
        }
        if self._state.dsn:
            config_values["JIMM_DSN"] = self._state.dsn

        if container.exists(self._agent_filename):
            config_values["BAKERY_AGENT_FILE"] = self._agent_filename

        if container.exists(self._vault_secret_filename):
            config_values["VAULT_ADDR"] = self._state.vault_address
            config_values["VAULT_PATH"] = self._vault_path
            config_values["VAULT_SECRET_FILE"] = self._vault_secret_filename
            config_values["VAULT_AUTH_PATH"] = "/auth/approle/login"

        if self.model.unit.is_leader():
            config_values["JIMM_WATCH_CONTROLLERS"] = "1"

        # remove empty configuration values
        config_values = {key: value for key, value in config_values.items() if value}

        pebble_layer = {
            "summary": "jimm layer",
            "description": "pebble config layer for jimm",
            "services": {
                JIMM_SERVICE_NAME: {
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
            if container.get_service(JIMM_SERVICE_NAME).is_running():
                container.replan()
            else:
                container.start(JIMM_SERVICE_NAME)
            self.unit.status = ActiveStatus("running")
        else:
            logger.info("workload container not ready - defering")
            event.defer()
            return

        dashboard_relation = self.model.get_relation("dashboard")
        if dashboard_relation and self.unit.is_leader():
            dashboard_relation.data[self.app].update(
                {
                    "controller_url": "wss://{}".format(dns_name),
                    "identity_provider_url": self.config.get("candid-url"),
                    "is_juju": str(False),
                }
            )

    def _on_start(self, event):
        """Start JIMM."""
        self._update_workload(event)

    def _on_stop(self, _):
        """Stop JIMM."""
        container = self.unit.get_container(WORKLOAD_CONTAINER)
        if container.can_connect():
            container.stop(JIMM_SERVICE_NAME)
        self._ready()

    def _on_update_status(self, _):
        """Update the status of the charm."""
        self._ready()

    @requires_state_setter
    def _on_dashboard_relation_joined(self, event: RelationJoinedEvent):
        dns_name = self._get_dns_name(event)
        if not dns_name:
            return

        event.relation.data[self.app].update(
            {
                "controller_url": "wss://{}".format(dns_name),
                "identity_provider_url": self.config["candid-url"],
                "is_juju": str(False),
            }
        )

    @requires_state_setter
    def _on_database_event(self, event: DatabaseEvent) -> None:
        """Database event handler."""

        if event.username is None or event.password is None:
            event.defer()
            logger.info(
                "(postgresql) Relation data is not complete (missing `username` or `password` field); "
                "deferring the event."
            )
            return

        # get the first endpoint from a comma separate list
        ep = event.endpoints.split(",", 1)[0]
        # compose the db connection string
        uri = f"postgresql://{event.username}:{event.password}@{ep}/{DATABASE_NAME}"

        logger.info("received database uri: {}".format(uri))

        # record the connection string
        self._state.dsn = uri

        self._update_workload(event)

    @requires_state_setter
    def _on_database_relation_broken(self, event: DatabaseEvent) -> None:
        """Database relation broken handler."""

        # when the database relation is broken, we unset the
        # connection string and schema-created from the application
        # bucket of the peer relation
        del self._state.dsn

        self._update_workload(event)

    def _ready(self):
        container = self.unit.get_container(WORKLOAD_CONTAINER)

        if container.can_connect():
            plan = container.get_plan()
            if plan.services.get(JIMM_SERVICE_NAME) is None:
                logger.error("waiting for service")
                self.unit.status = WaitingStatus("waiting for service")
                return False

            env_vars = plan.services.get(JIMM_SERVICE_NAME).environment

            for setting in REQUIRED_SETTINGS:
                if not env_vars.get(setting, ""):
                    self.unit.status = BlockedStatus(
                        "{} configuration value not set".format(setting),
                    )
                    return False

            if container.get_service(JIMM_SERVICE_NAME).is_running():
                self.unit.status = ActiveStatus("running")
            else:
                self.unit.status = WaitingStatus("stopped")
            return True
        else:
            logger.error("cannot connect to workload container")
            self.unit.status = WaitingStatus("waiting for jimm workload")
            return False

    def _get_network_address(self, event):
        return str(self.model.get_binding(event.relation).network.egress_subnets[0].network_address)

    def _on_vault_relation_joined(self, event):
        event.relation.data[self.unit]["secret_backend"] = json.dumps(self._vault_path)
        event.relation.data[self.unit]["hostname"] = json.dumps(socket.gethostname())
        event.relation.data[self.unit]["access_address"] = json.dumps(self._get_network_address(event))
        event.relation.data[self.unit]["isolated"] = json.dumps(False)

    def _ensure_vault_file(self, event):
        container = self.unit.get_container(WORKLOAD_CONTAINER)

        if not self._unit_state.is_ready():
            logger.info("unit state not ready")
            event.defer()
            return

        # if we can't connect to the container we should defer
        # this event.
        if not container.can_connect():
            event.defer()
            return

        if container.exists(self._vault_secret_filename):
            container.remove_path(self._vault_secret_filename)

        secret_data = self._unit_state.vault_secret_data
        if secret_data:
            self._push_to_workload(self._vault_secret_filename, secret_data, event)

    def _on_vault_relation_changed(self, event):
        if not self._unit_state.is_ready() or not self._state.is_ready():
            logger.info("state not ready")
            event.defer()
            return

        addr = _json_data(event, "vault_url")
        if not addr:
            return
        role_id = _json_data(event, "{}_role_id".format(self.unit.name))
        if not role_id:
            return
        token = _json_data(event, "{}_token".format(self.unit.name))
        if not token:
            return
        client = hvac.Client(url=addr, token=token)
        secret = client.sys.unwrap()
        secret["data"]["role_id"] = role_id

        secret_data = json.dumps(secret)

        logger.error("setting unit state data {}".format(secret_data))
        self._unit_state.vault_secret_data = secret_data
        if self.unit.is_leader():
            self._state.vault_address = addr

        self._update_workload(event)

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
            logger.info("pushing file {} to the workload containe".format(filename))
            container.push(filename, content, make_dirs=True)
        else:
            logger.info("workload container not ready - defering")
            event.defer()

    def _hash(self, filename):
        buffer_size = 65536
        md5 = hashlib.md5()

        with open(filename, "rb") as f:
            while True:
                data = f.read(buffer_size)
                if not data:
                    break
                md5.update(data)
            return md5.hexdigest()

    @requires_state
    def _get_dns_name(self, event):
        if not self._state.is_ready():
            event.defer()
            logger.warning("State is not ready")
            return None

        default_dns_name = "{}.{}-endpoints.{}.svc.cluster.local".format(
            self.unit.name.replace("/", "-"),
            self.app.name,
            self.model.name,
        )
        dns_name = self.config.get("dns-name", default_dns_name)
        if self._state.dns_name:
            dns_name = self._state.dns_name

        return dns_name

    @requires_state_setter
    def _on_certificates_relation_joined(self, event: RelationJoinedEvent) -> None:
        dns_name = self._get_dns_name(event)
        if not dns_name:
            return

        csr = generate_csr(
            private_key=self._state.private_key.encode(),
            subject=dns_name,
        )

        self._state.csr = csr.decode()

        self.certificates.request_certificate_creation(certificate_signing_request=csr)

    @requires_state_setter
    def _on_certificate_available(self, event: CertificateAvailableEvent) -> None:
        self._state.certificate = event.certificate
        self._state.ca = event.ca
        self._state.chain = event.chain

        self._update_workload(event)

    @requires_state_setter
    def _on_certificate_expiring(self, event: CertificateExpiringEvent) -> None:
        old_csr = self._state.csr
        private_key = self._state.private_key
        dns_name = self._get_dns_name(event)
        if not dns_name:
            return

        new_csr = generate_csr(
            private_key=private_key.encode(),
            subject=dns_name,
        )
        self.certificates.request_certificate_renewal(
            old_certificate_signing_request=old_csr,
            new_certificate_signing_request=new_csr,
        )
        self._state.csr = new_csr.decode()

        self._update_workload(event)

    @requires_state_setter
    def _on_certificate_revoked(self, event: CertificateRevokedEvent) -> None:
        old_csr = self._state.csr
        private_key = self._state.private_key
        dns_name = self._get_dns_name(event)
        if not dns_name:
            return

        new_csr = generate_csr(
            private_key=private_key.encode(),
            subject=dns_name,
        )
        self.certificates.request_certificate_renewal(
            old_certificate_signing_request=old_csr,
            new_certificate_signing_request=new_csr,
        )

        self._state.csr = new_csr.decode()
        del self._state.certificate
        del self._state.ca
        del self._state.chain

        self.unit.status = WaitingStatus("Waiting for new certificate")
        self._update_workload(event)

    @requires_state_setter
    def _on_ingress_ready(self, event: IngressPerAppReadyEvent):
        self._state.dns_name = event.url

        self._update_workload(event)

    @requires_state_setter
    def _on_ingress_revoked(self, event: IngressPerAppRevokedEvent):
        del self._state.dns_name

        self._update_workload(event)


def _json_data(event, key):
    logger.debug("getting relation data {}".format(key))
    try:
        return json.loads(event.relation.data[event.unit][key])
    except KeyError:
        return None


if __name__ == "__main__":
    main(JimmOperatorCharm)
