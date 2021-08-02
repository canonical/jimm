#!/usr/bin/env python3
# Copyright 2021 Canonical Ltd
# See LICENSE file for licensing details.
#
# Learn more at: https://juju.is/docs/sdk

import json
import logging
import os
import socket
import subprocess
import urllib

import hvac
from jinja2 import Environment, FileSystemLoader

from charmhelpers.contrib.charmsupport.nrpe import NRPE
from ops.main import main
from ops.model import ActiveStatus, BlockedStatus, MaintenanceStatus, ModelError, WaitingStatus

from systemd import SystemdCharm

logger = logging.getLogger(__name__)


class JimmCharm(SystemdCharm):
    """Charm for the JIMM service."""

    def __init__(self, *args):
        super().__init__(*args)
        self.framework.observe(self.on.config_changed, self._on_config_changed)
        self.framework.observe(self.on.db_relation_changed, self._on_db_relation_changed)
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
        self._agent_filename = "/var/snap/jimm/common/agent.json"
        self._vault_secret_filename = "/var/snap/jimm/common/vault_secret.json"
        self._workload_filename = "/snap/bin/jimm"

    def _on_install(self, _):
        """Install the JIMM software."""
        self._write_service_file()
        self._install_snap()
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
        if self._ready():
            self.restart()
        self._on_update_status(None)

    def _on_config_changed(self, _):
        """Update the JIMM configuration that comes from the charm
        config."""

        args = {
            "bakery_agent_file": self._bakery_agent_file(),
            "candid_url": self.config.get('candid-url'),
            "admins": self.config.get('controller-admins', ''),
            "uuid": self.config.get('uuid')
        }
        with open(self._env_filename(), "wt") as f:
            f.write(self._render_template('jimm.env', **args))
        if self._ready():
            self.restart()
        self._on_update_status(None)

    def _on_leader_elected(self, _):
        """Update the JIMM configuration that comes from unit
        leadership."""

        args = {"jimm_watch_controllers": ""}
        if self.model.unit.is_leader():
            args["jimm_watch_controllers"] = "1"
        with open(self._env_filename("leader"), "wt") as f:
            f.write(self._render_template("jimm-leader.env", **args))
        if self._ready():
            self.restart()
        self._on_update_status(None)

    def _on_db_relation_changed(self, event):
        """Update the JIMM configuration that comes from database
        relations."""

        dsn = event.relation.data[event.unit].get("master")
        if not dsn:
            return
        args = {"dsn": "pgx:" + dsn}
        with open(self._env_filename("db"), "wt") as f:
            f.write(self._render_template('jimm-db.env', **args))
        if self._ready():
            self.restart()
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
        if not self.model.get_relation("db"):
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
            check_cmd='check_http -w 2 -c 10 -I {} -p 8080 -u /debug/info'.format(
                self.model.get_binding(event.relation).network.ingress_address,
            )
        )
        nrpe.write()

    def _on_website_relation_joined(self, event):
        """Connect a website relation."""
        event.relation.data[self.unit]["port"] = "8080"

    def _on_vault_relation_joined(self, event):
        event.relation.data[self.unit]['secret_backend'] = json.dumps("charm-jimm-creds")
        event.relation.data[self.unit]['hostname'] = json.dumps(socket.gethostname())
        event.relation.data[self.unit]['access_address'] = \
            json.dumps(str(
                self.model.get_binding(event.relation).network.egress_subnets[0].network_address))
        event.relation.data[self.unit]['isolated'] = json.dumps(False)

    def _on_vault_relation_changed(self, event):
        if not os.path.exists(os.path.dirname(self._vault_secret_filename)):
            # if the snap is yet to be installed wait for it.
            event.defer()
            return

        addr = _json_data(event, 'vault_url')
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
        secret['data']['role_id'] = role_id
        with open(self._vault_secret_filename, "wt") as f:
            json.dump(secret, f)
        args = {
            "vault_secret_file": self._vault_secret_filename,
            "vault_addr": addr,
            "vault_auth_path": "/auth/approle/login",
            "vault_path": "charm-jimm-creds"
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
        self._snap('install', '--dangerous', path)

    def _bakery_agent_file(self):
        url = self.config.get('candid-url', '')
        username = self.config.get('candid-agent-username', '')
        private_key = self.config.get('candid-agent-private-key', '')
        public_key = self.config.get('candid-agent-public-key', '')
        if not url or not username or not private_key or not public_key:
            return ""
        data = {
            "key": {"public": public_key, "private": private_key},
            "agents": [{"url": url, "username": username}]
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
            "vault_file": self._env_filename("vault")
        }
        with open(self.service_file, "wt") as f:
            f.write(self._render_template('jimm.service', **args))

    def _render_template(self, name, **kwargs):
        """Load the template with the given name."""
        loader = FileSystemLoader(os.path.join(self.charm_dir, 'templates'))
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
        cmd = ['snap']
        cmd.extend(args)
        subprocess.run(cmd, capture_output=True, check=True)


def _json_data(event, key):
    logger.debug("getting relation data {}".format(key))
    try:
        return json.loads(event.relation.data[event.unit][key])
    except KeyError:
        return None


if __name__ == "__main__":
    main(JimmCharm)
