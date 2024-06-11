#!/usr/bin/env python3
# Copyright 2021 Canonical Ltd
# See LICENSE file for licensing details.
#
# Learn more at: https://juju.is/docs/sdk

import json
import logging
import os
import shutil
import socket
import subprocess
import urllib
from urllib.parse import urljoin, urlparse

import hvac
from charmhelpers.contrib.charmsupport.nrpe import NRPE
from charms.data_platform_libs.v0.data_interfaces import (
    DatabaseRequires,
    DatabaseRequiresEvent,
)
from charms.grafana_agent.v0.cos_agent import COSAgentProvider
from charms.hydra.v0.oauth import ClientConfig, OAuthInfoChangedEvent, OAuthRequirer
from charms.openfga_k8s.v1.openfga import OpenFGARequires, OpenFGAStoreCreateEvent
from jinja2 import Environment, FileSystemLoader
from ops.main import main
from ops.model import (
    ActiveStatus,
    BlockedStatus,
    MaintenanceStatus,
    ModelError,
    Relation,
)

from systemd import SystemdCharm

logger = logging.getLogger(__name__)

DATABASE_NAME = "jimm"
OPENFGA_STORE_NAME = "jimm"
OAUTH = "oauth"
OAUTH_SCOPES = "openid email offline_access"
# TODO: Add "device_code" below once the charm interface supports it.
OAUTH_GRANT_TYPES = ["authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:device_code"]

# Env file parts
DB_PART = "db"
VAULT_PART = "vault"
OAUTH_PART = "oauth"
LEADER_PART = "leader"
OPENFGA_PART = "openfga"


class JimmCharm(SystemdCharm):
    """Charm for the JIMM service."""

    def __init__(self, *args):
        super().__init__(*args)
        self.framework.observe(self.on.config_changed, self._on_config_changed)
        self.framework.observe(self.on.install, self._on_install)
        self.framework.observe(self.on.leader_elected, self._on_leader_elected)
        self.framework.observe(self.on.start, self._on_start)
        self.framework.observe(self.on.stop, self._on_stop)
        self.framework.observe(self.on.update_status, self._on_update_status)
        self.framework.observe(self.on.upgrade_charm, self._on_upgrade_charm)
        self.framework.observe(self.on.nrpe_relation_joined, self._on_nrpe_relation_joined)
        self.framework.observe(self.on.website_relation_joined, self._on_website_relation_joined)
        self.framework.observe(self.on.vault_relation_joined, self._on_vault_relation_joined)
        self.framework.observe(self.on.vault_relation_changed, self._on_vault_relation_changed)
        self.framework.observe(
            self.on.dashboard_relation_joined,
            self._on_dashboard_relation_joined,
        )
        self._vault_secret_filename = "/var/snap/jimm/common/vault_secret.json"
        self._workload_filename = "/snap/bin/jimm"
        self._rsyslog_conf_path = "/etc/rsyslog.d/10-jimm.conf"
        self._logrotate_conf_path = "/etc/logrotate.d/jimm"

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

        self.openfga = OpenFGARequires(self, OPENFGA_STORE_NAME)
        self.framework.observe(
            self.openfga.on.openfga_store_created,
            self._on_openfga_store_created,
        )

        self.oauth = OAuthRequirer(self, self._oauth_client_config, relation_name=OAUTH)
        self.framework.observe(self.oauth.on.oauth_info_changed, self._on_oauth_info_changed)
        self.framework.observe(self.oauth.on.oauth_info_removed, self._on_oauth_info_removed)

        # Grafana agent relation
        self._grafana_agent = COSAgentProvider(
            self,
            relation_name="cos-agent",
            metrics_endpoints=[
                {"path": "/metrics", "port": 8080},
            ],
            metrics_rules_dir="./src/alert_rules/prometheus",
            logs_rules_dir="./src/alert_rules/loki",
            recurse_rules_dirs=True,
            dashboard_dirs=["./src/grafana_dashboards"],
        )

    def _on_install(self, _):
        """Install the JIMM software."""
        self._write_service_file()
        self._install_snap()
        self._setup_logging()
        self._on_update_status(None)

    def _on_start(self, _):
        """Start the JIMM software."""
        self.enable()
        if self._ready():
            self.start()
        self._on_update_status(None)

    def _on_upgrade_charm(self, _):
        """Upgrade the charm software."""
        self._write_service_file()
        self._install_snap()
        self._setup_logging()
        if self._ready():
            self.restart()
        self._on_update_status(None)

    def _on_config_changed(self, _):
        """Update the JIMM configuration that comes from the charm
        config."""

        args = {
            "admins": self.config.get("controller-admins", ""),
            "dns_name": self.config.get("dns-name"),
            "log_level": self.config.get("log-level"),
            "uuid": self.config.get("uuid"),
            "dashboard_location": self.config.get("juju-dashboard-location"),
            "bakery_public_key": self.config.get("public-key", ""),
            "bakery_private_key": self.config.get("private-key", ""),
            "audit_retention_period": self.config.get("audit-log-retention-period-in-days", ""),
            "jwt_expiry": self.config.get("jwt-expiry", "5m"),
            "macaroon_expiry_duration": self.config.get("macaroon-expiry-duration"),
            "session_expiry_duration": self.config.get("session-expiry-duration"),
            "secure_session_cookies": self.config.get("secure-session-cookies"),
            "session_cookie_max_age": self.config.get("session-cookie-max-age"),
        }

        self.oauth.update_client_config(client_config=self._oauth_client_config)

        if self.config.get("postgres-secret-storage", False):
            args["insecure_secret_storage"] = "enabled"  # Value doesn't matter, only checks env var exists.

        with open(self._env_filename(), "wt") as f:
            f.write(self._render_template("jimm.env", **args))
        if self._ready():
            self.restart()
        self._on_update_status(None)

        dashboard_relation = self.model.get_relation("dashboard")
        if dashboard_relation:
            self._update_dashboard_relation(dashboard_relation)

    def _on_leader_elected(self, _):
        """Update the JIMM configuration that comes from unit
        leadership."""

        args = {"jimm_watch_controllers": ""}
        if self.model.unit.is_leader():
            args["jimm_watch_controllers"] = "1"
            args["jimm_enable_jwks_rotator"] = "1"
        with open(self._env_filename(LEADER_PART), "wt") as f:
            f.write(self._render_template("jimm-leader.env", **args))
        if self._ready():
            self.restart()
        self._on_update_status(None)

    def _on_database_event(self, event: DatabaseRequiresEvent):
        """Handle database event"""
        if not event.endpoints:
            logger.info("received empty database host address")
            event.defer()
            return

        if event.username is None or event.password is None:
            event.defer()
            logger.info(
                "(postgresql) Relation data is not complete (missing `username` or `password` field); "
                "deferring the event."
            )
            return

        # get the first endpoint from a comma separate list
        host = event.endpoints.split(",", 1)[0]
        # compose the db connection string
        uri = f"postgresql://{event.username}:{event.password}@{host}/{DATABASE_NAME}"
        logger.info("received database uri: {}".format(uri))

        args = {"dsn": uri}
        with open(self._env_filename(DB_PART), "wt") as f:
            f.write(self._render_template("jimm-db.env", **args))
        if self._ready():
            self.restart()
        self._on_update_status(None)

    def _on_database_relation_broken(self, event) -> None:
        """Database relation broken handler."""
        logger.info("database relation removed")
        try:
            os.remove(self._env_filename(DB_PART))
        except OSError:
            pass
        self.stop()
        self._on_update_status(None)

    def _on_oauth_info_changed(self, event: OAuthInfoChangedEvent):
        if not self.oauth.is_client_created():
            logger.warning("OAuth relation is not ready yet")
            return
        oauth_provider_info = self.oauth.get_provider_info()
        oauth_info = {
            "issuer_url": oauth_provider_info.issuer_url,
            "client_id": oauth_provider_info.client_id,
            "client_secret": oauth_provider_info.client_secret,
            "scope": oauth_provider_info.scope,
        }
        with open(self._env_filename(OAUTH_PART), "wt") as f:
            f.write(self._render_template("jimm-oauth.env", **oauth_info))
        if self._ready():
            self.restart()
        self._on_update_status(event)

    def _on_oauth_info_removed(self, event: OAuthInfoChangedEvent):
        logger.info("oauth relation removed")
        try:
            os.remove(self._env_filename(OAUTH_PART))
        except OSError:
            pass
        self.stop()
        self._on_update_status(event)

    def _on_stop(self, _):
        """Stop the JIMM service."""
        self.stop()
        self.disable()
        self._on_update_status(None)

    def _on_update_status(self, _):
        """Update the status of the charm."""

        if not self._ready():
            return
        try:
            url = "http://localhost:8080/debug/info"
            with urllib.request.urlopen(url) as resp:
                data = json.loads(resp.read().decode("utf-8"))
                v = data.get("Version", "")
                if v:
                    self.unit.set_workload_version(v)
                self.unit.status = ActiveStatus()
        except Exception as e:
            logger.error("getting version: %s (%s)", str(e), type(e))
            self.unit.status = MaintenanceStatus("starting")

    def _on_nrpe_relation_joined(self, event):
        """Connect a NRPE relation."""
        nrpe = NRPE()
        nrpe.add_check(
            shortname="JIMM",
            description="check JIMM running",
            check_cmd="check_http -w 2 -c 10 -I {} -p 8080 -u /debug/info".format(
                self.model.get_binding(event.relation).network.ingress_address,
            ),
        )
        nrpe.write()

    def _on_website_relation_joined(self, event):
        """Connect a website relation."""
        event.relation.data[self.unit]["port"] = "8080"

    def _on_vault_relation_joined(self, event):
        event.relation.data[self.unit]["secret_backend"] = json.dumps("charm-jimm-creds")
        event.relation.data[self.unit]["hostname"] = json.dumps(socket.gethostname())
        event.relation.data[self.unit]["access_address"] = json.dumps(
            str(self.model.get_binding(event.relation).network.egress_subnets[0].network_address)
        )
        event.relation.data[self.unit]["isolated"] = json.dumps(False)

    def _on_vault_relation_changed(self, event):
        if not os.path.exists(os.path.dirname(self._vault_secret_filename)):
            # if the snap is yet to be installed wait for it.
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
        with open(self._vault_secret_filename, "wt") as f:
            json.dump(secret, f)
        args = {
            "vault_secret_file": self._vault_secret_filename,
            "vault_addr": addr,
            "vault_auth_path": "/auth/approle/login",
            "vault_path": "charm-jimm-creds",
        }
        with open(self._env_filename(VAULT_PART), "wt") as f:
            f.write(self._render_template("jimm-vault.env", **args))

    def _install_snap(self):
        self.unit.status = MaintenanceStatus("installing snap")
        try:
            path = self.model.resources.fetch("jimm-snap")
        except ModelError:
            path = None
        if not path:
            self.unit.status = BlockedStatus("waiting for jimm-snap resource")
            return
        # remove the jimm snap if it is already installed.
        self._snap("remove", "jimm")
        # install the new jimm snap.
        self._snap("install", "--dangerous", path)

    def _setup_logging(self):
        """Install the logging configuration."""
        shutil.copy(
            os.path.join(self.charm_dir, "files", "logrotate"),
            self._logrotate_conf_path,
        )
        shutil.copy(
            os.path.join(self.charm_dir, "files", "rsyslog"),
            self._rsyslog_conf_path,
        )
        self._systemctl("restart", "rsyslog")

    def _write_service_file(self):
        args = {
            "conf_file": self._env_filename(),
            "db_file": self._env_filename(DB_PART),
            "leader_file": self._env_filename(LEADER_PART),
            "vault_file": self._env_filename(VAULT_PART),
            "openfga_file": self._env_filename(OPENFGA_PART),
            "oauth_file": self._env_filename(OAUTH_PART),
        }
        with open(self.service_file, "wt") as f:
            f.write(self._render_template("jimm.service", **args))

    def _render_template(self, name, **kwargs):
        """Load the template with the given name."""
        loader = FileSystemLoader(os.path.join(self.charm_dir, "templates"))
        env = Environment(loader=loader)
        return env.get_template(name).render(**kwargs)

    def _ready(self):
        if not os.path.exists(self._env_filename()):
            logger.warning("Missing base environment file")
            self.unit.status = BlockedStatus("Waiting for environment")
            return False
        if not os.path.exists(self._env_filename(DB_PART)):
            logger.warning("Missing database environment file")
            self.unit.status = BlockedStatus("Waiting for database relation")
            return False
        if not os.path.exists(self._env_filename(OAUTH_PART)):
            logger.warning("Missing oauth environment file")
            self.unit.status = BlockedStatus("Waiting for oauth relation")
            return False
        if not os.path.exists(self._env_filename(OPENFGA_PART)):
            logger.warning("Missing openfga environment file")
            self.unit.status = BlockedStatus("Waiting for openfga relation")
            return False
        return True

    def _env_filename(self, part=None):
        """Calculate the filename for a JIMM configuration environment file."""
        if part:
            filename = "{}-{}.env".format(self.app.name, part)
        else:
            filename = "{}.env".format(self.app.name)
        return self.charm_dir.joinpath(filename)

    def _snap(self, *args):
        cmd = ["snap"]
        cmd.extend(args)
        subprocess.run(cmd, capture_output=True, check=True)

    def _on_dashboard_relation_joined(self, event):
        if self.model.unit.is_leader():
            self._update_dashboard_relation(event.relation)

    def _update_dashboard_relation(self, relation: Relation):
        if self.model.unit.is_leader():
            relation.data[self.app].update(
                {
                    "controller_url": "wss://{}".format(self.config["dns-name"]),
                    "is_juju": str(False),
                }
            )

    def _on_openfga_store_created(self, event: OpenFGAStoreCreateEvent):
        if not event.store_id:
            return

        info = self.openfga.get_store_info()
        if not info:
            logger.warning("openfga info not ready yet")
            return

        o = urlparse(info.http_api_url)
        args = {
            "openfga_host": o.hostname,
            "openfga_port": o.port,
            "openfga_scheme": o.scheme,
            "openfga_store": info.store_id,
            "openfga_token": info.token,
        }

        with open(self._env_filename(OPENFGA_PART), "wt") as f:
            f.write(self._render_template("jimm-openfga.env", **args))
        if self._ready():
            self.restart()
        self._on_update_status(None)

    @property
    def _oauth_client_config(self) -> ClientConfig:
        dns = self.config.get("dns-name")
        if dns is None or dns == "":
            dns = "http://localhost"
        dns = ensureFQDN(dns)
        return ClientConfig(
            urljoin(dns, "/oauth/callback"),
            OAUTH_SCOPES,
            OAUTH_GRANT_TYPES,
        )


def ensureFQDN(dns: str):  # noqa: N802
    """Ensures a domain name has an https:// prefix."""
    if not dns.startswith("http"):
        dns = "https://" + dns
    return dns


def _json_data(event, key):
    logger.debug("getting relation data {}".format(key))
    try:
        return json.loads(event.relation.data[event.unit][key])
    except KeyError:
        return None


if __name__ == "__main__":
    main(JimmCharm)
