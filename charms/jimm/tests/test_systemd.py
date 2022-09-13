# Copyright 2021 Canonical Ltd
# See LICENSE file for licensing details.

import subprocess
import unittest
from unittest.mock import Mock

from systemd import SystemdCharm
from ops.testing import Harness


class TestSystemdCharm(unittest.TestCase):
    def setUp(self):
        self.harness = Harness(SystemdCharm)
        self.addCleanup(self.harness.cleanup)
        self.harness.begin()
        self.harness.charm._systemctl = Mock()

    def test_service(self):
        self.assertEqual(self.harness.charm.service, 'jimm.service')

    def test_service_file(self):
        self.assertEqual(self.harness.charm.service_file,
                         self.harness.charm.charm_dir.joinpath('jimm.service'))

    def test_is_enabled(self):
        self.assertTrue(self.harness.charm.is_enabled())
        self.harness.charm._systemctl.assert_called_once_with(
            'is-enabled',
            self.harness.charm.service)
        self.harness.charm._systemctl = Mock(
            side_effect=subprocess.CalledProcessError(1, None, output="linked"))
        self.assertFalse(self.harness.charm.is_enabled())
        self.harness.charm._systemctl.assert_called_once_with(
            'is-enabled',
            self.harness.charm.service)

    def test_enable(self):
        self.harness.charm.enable()
        self.harness.charm._systemctl.assert_called_once_with(
            'enable',
            str(self.harness.charm.service_file))

    def test_disable(self):
        self.harness.charm.disable()
        self.harness.charm._systemctl.assert_called_with(
            'disable',
            self.harness.charm.service)

    def test_start(self):
        self.harness.charm.start()
        self.harness.charm._systemctl.assert_called_with(
            'start',
            self.harness.charm.service)

    def test_restart(self):
        self.harness.charm.restart()
        self.harness.charm._systemctl.assert_called_with(
            'restart',
            self.harness.charm.service)

    def test_stop(self):
        self.harness.charm.stop()
        self.harness.charm._systemctl.assert_called_with(
            'stop',
            self.harness.charm.service)
