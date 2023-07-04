# Copyright 2021 Canonical Ltd
# See LICENSE file for licensing details.

import logging
import pathlib
import subprocess

from ops.charm import CharmBase

logger = logging.getLogger(__name__)


class SystemdCharm(CharmBase):
    """Support class for charms the run systemd services."""

    def __init__(self, *args):
        super().__init__(*args)

    @property
    def service(self) -> str:
        """Name of the service."""
        return "{}.service".format(self.app.name)

    @property
    def service_file(self) -> pathlib.Path:
        """Path of the service file that will be used."""
        return self.charm_dir.joinpath(self.service)

    def is_enabled(self) -> bool:
        """Return whether the service is currently enabled."""
        try:
            self._systemctl("is-enabled", self.service)
            return True
        except subprocess.CalledProcessError as e:
            logger.info("is-enabled %s: %s", self.service, e.output.strip())
            return False

    def enable(self):
        """Enable the service."""
        self._systemctl("enable", str(self.service_file))

    def disable(self):
        """Disable the service, if it is enabled."""
        if self.is_enabled():
            self._systemctl("disable", self.service)

    def start(self):
        """Start the service, if it is enabled."""
        if self.is_enabled():
            self._systemctl("start", self.service)

    def restart(self):
        """Restart the service, if it is enabled."""
        if self.is_enabled():
            self._systemctl("restart", self.service)

    def stop(self):
        """Stop the service, if it is enabled."""
        if self.is_enabled():
            self._systemctl("stop", self.service)

    def _systemctl(self, *args):
        """Run the requested systemctl command."""
        cmd = ["systemctl"]
        cmd.extend(args)
        logger.debug("running: %s", " ".join(cmd))
        subprocess.run(cmd, capture_output=True, check=True)
