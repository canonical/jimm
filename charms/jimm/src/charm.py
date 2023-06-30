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
import tarfile
import urllib

import hvac
from charmhelpers.contrib.charmsupport.nrpe import NRPE
from charms.data_platform_libs.v0.data_interfaces import (
    DatabaseRequires,
    DatabaseRequiresEvent,
)
from charms.grafana_agent.v0.cos_agent import COSAgentProvider
from charms.openfga_k8s.v0.openfga import OpenFGARequires, OpenFGAStoreCreateEvent
from jinja2 import Environment, FileSystemLoader
from ops.main import main
from ops.model import (
    ActiveStatus,
    BlockedStatus,
    MaintenanceStatus,
    ModelError,
    WaitingStatus,
)

from systemd import SystemdCharm

logger = logging.getLogger(__name__)

DATABASE_NAME = "jimm"
OPENFGA_STORE_NAME = "jimm"


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
        self._agent_filename = "/var/snap/jimm/common/agent.json"
        self._vault_secret_filename = "/var/snap/jimm/common/vault_secret.json"
        self._workload_filename = "/snap/bin/jimm"
        self._dashboard_path = "/var/snap/jimm/common/dashboard"
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
        self._install_dashboard()
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
        self._install_dashboard()
        self._setup_logging()
        if self._ready():
            self.restart()
        self._on_update_status(None)

    def _on_config_changed(self, _):
        """Update the JIMM configuration that comes from the charm
        config."""

        args = {
            "admins": self.config.get("controller-admins", ""),
            "bakery_agent_file": self._bakery_agent_file(),
            "candid_url": self.config.get("candid-url"),
            "dns_name": self.config.get("dns-name"),
            "log_level": self.config.get("log-level"),
            "uuid": self.config.get("uuid"),
            "dashboard_location": self.config.get("juju-dashboard-location"),
            "public_key": self.config.get("public-key"),
            "private_key": self.config.get("private-key"),
        }
        if os.path.exists(self._dashboard_path):
            args["dashboard_location"] = self._dashboard_path

        with open(self._env_filename(), "wt") as f:
            f.write(self._render_template("jimm.env", **args))

        if self._ready():
            self.restart()
        self._on_update_status(None)

        dashboard_relation = self.model.get_relation("dashboard")
        if dashboard_relation:
            dashboard_relation.data[self.app].update(
                {
                    "controller-url": self.config["dns-name"],
                    "identity-provider-url": self.config["candid-url"],
                    "is-juju": str(False),
                }
            )

    def _on_leader_elected(self, _):
        """Update the JIMM configuration that comes from unit
        leadership."""

        args = {"jimm_watch_controllers": ""}
        if self.model.unit.is_leader():
            args["jimm_watch_controllers"] = "1"
            args["jimm_enable_jwks_rotator"] = "1"
        with open(self._env_filename("leader"), "wt") as f:
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

        # get the first endpoint from a comma separate list
        host = event.endpoints.split(",", 1)[0]
        # compose the db connection string
        uri = f"postgresql://{event.username}:{event.password}@{host}/{DATABASE_NAME}"
        logger.info("received database uri: {}".format(uri))

        args = {"dsn": uri}
        with open(self._env_filename("db"), "wt") as f:
            f.write(self._render_template("jimm-db.env", **args))
        if self._ready():
            self.restart()
        self._on_update_status(None)

    def _on_database_relation_broken(self, event) -> None:
        """Database relation broken handler."""
        if not self._ready():
            event.defer()
            logger.warning("Unit is not ready")
            return
        logger.info("database relation removed")
        self._on_update_status(None)

    def _on_stop(self, _):
        """Stop the JIMM service."""
        self.stop()
        self.disable()
        self._on_update_status(None)

    def _on_update_status(self, _):
        """Update the status of the charm."""

        if not os.path.exists(self._workload_filename):
            self.unit.status = BlockedStatus("waiting for jimm-snap resource")
            return
        if not self.model.get_relation("database"):
            self.unit.status = BlockedStatus("waiting for database")
            return
        if not os.path.exists(self._env_filename("db")):
            self.unit.status = WaitingStatus("waiting for database")
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
        with open(self._env_filename("vault"), "wt") as f:
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
        self._snap("install", "--dangerous", path)

    def _install_dashboard(self):
        try:
            path = self.model.resources.fetch("dashboard")
        except ModelError:
            path = None

        if not path:
            return

        if self._dashboard_resource_nonempty():
            new_dashboard_path = self._dashboard_path + ".new"
            old_dashboard_path = self._dashboard_path + ".old"
            shutil.rmtree(new_dashboard_path, ignore_errors=True)
            shutil.rmtree(old_dashboard_path, ignore_errors=True)
            os.mkdir(new_dashboard_path)

            self.unit.status = MaintenanceStatus("installing dashboard")
            with tarfile.open(path, mode="r:bz2") as tf:
                tf.extractall(new_dashboard_path)

                # Change the owner/group of all extracted files to root/wheel.
                for name in tf.getnames():
                    os.chown(os.path.join(new_dashboard_path, name), 0, 0)

            if os.path.exists(self._dashboard_path):
                os.rename(self._dashboard_path, old_dashboard_path)
            os.rename(new_dashboard_path, self._dashboard_path)

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

    def _dashboard_resource_nonempty(self):
        dashboard_file = self.model.resources.fetch("dashboard")
        if dashboard_file:
            return os.path.getsize(dashboard_file) != 0
        return False

    def _bakery_agent_file(self):
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
        try:
            with open(self._agent_filename, "wt") as f:
                json.dump(data, f)
        except FileNotFoundError:
            return ""
        return self._agent_filename

    def _write_service_file(self):
        args = {
            "conf_file": self._env_filename(),
            "db_file": self._env_filename("db"),
            "leader_file": self._env_filename("leader"),
            "vault_file": self._env_filename("vault"),
            "openfga_file": self._env_filename("openfga"),
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
            return False
        if not os.path.exists(self._env_filename("db")):
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
        event.relation.data[self.app].update(
            {
                "controller-url": self.config["dns-name"],
                "identity-provider-url": self.config["candid-url"],
                "is-juju": str(False),
            }
        )

    def _on_openfga_store_created(self, event: OpenFGAStoreCreateEvent):
        if not event.store_id:
            return

        logger.error("token secret {}".format(event.token_secret_id))
        secret = self.model.get_secret(id=event.token_secret_id)
        secret_content = secret.get_content()

        args = {
            "openfga_host": event.address,
            "openfga_port": event.port,
            "openfga_scheme": event.scheme,
            "openfga_store": event.store_id,
            "openfga_token": secret_content["token"],
        }

        with open(self._env_filename("openfga"), "wt") as f:
            f.write(self._render_template("jimm-openfga.env", **args))


def _json_data(event, key):
    logger.debug("getting relation data {}".format(key))
    try:
        return json.loads(event.relation.data[event.unit][key])
    except KeyError:
        return None


if __name__ == "__main__":
    main(JimmCharm)
