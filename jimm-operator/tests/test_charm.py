# Copyright 2022 Canonical Ltd
# See LICENSE file for licensing details.
#
# Learn more about testing at: https://juju.is/docs/sdk/testing


import io
import json
import pathlib
import tarfile
import tempfile
import unittest
from unittest.mock import patch

from charm import JimmOperatorCharm
from ops.testing import Harness

MINIMAL_CONFIG = {
    "uuid": "1234567890",
    "dns-name": "jimm.testing",
    "dsn": "test-dsn",
    "candid-url": "test-candid-url"
}


class TestCharm(unittest.TestCase):
    def setUp(self):
        self.maxDiff = None
        self.harness = Harness(JimmOperatorCharm)
        self.addCleanup(self.harness.cleanup)
        self.harness.disable_hooks()
        self.harness.add_oci_resource("jimm-image")
        self.harness.begin()

        self.tempdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tempdir.cleanup)
        self.harness.charm.framework.charm_dir = pathlib.Path(self.tempdir.name)

        self.harness.container_pebble_ready('jimm')

    def test_on_pebble_ready(self):
        self.harness.update_config(MINIMAL_CONFIG)
        self.harness.set_leader(True)

        container = self.harness.model.unit.get_container("jimm")
        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(
            plan.to_dict(),
            {'services': {
                'jimm': {
                    'summary': 'JAAS Intelligent Model Manager',
                    'startup': 'disabled',
                    'override': 'merge',
                    'command': '/root/jimmsrv',
                    'environment': {
                        'CANDID_URL': 'test-candid-url',
                        'JIMM_DNS_NAME': 'jimm.testing',
                        'JIMM_DSN': 'test-dsn',
                        'JIMM_DASHBOARD_LOCATION': 'https://jaas.ai/models',
                        'JIMM_LISTEN_ADDR': ':8080',
                        'JIMM_LOG_LEVEL': 'info',
                        'JIMM_UUID': '1234567890',
                        'JIMM_WATCH_CONTROLLERS': '1'
                    }
                }
            }}
        )

    def test_on_config_changed(self):
        container = self.harness.model.unit.get_container("jimm")
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        self.harness.update_config(MINIMAL_CONFIG)
        self.harness.set_leader(True)

        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(
            plan.to_dict(),
            {'services': {
                'jimm': {
                    'summary': 'JAAS Intelligent Model Manager',
                    'startup': 'disabled',
                    'override': 'merge',
                    'command': '/root/jimmsrv',
                    'environment': {
                        'CANDID_URL': 'test-candid-url',
                        'JIMM_DNS_NAME': 'jimm.testing',
                        'JIMM_DSN': 'test-dsn',
                        'JIMM_DASHBOARD_LOCATION': 'https://jaas.ai/models',
                        'JIMM_LISTEN_ADDR': ':8080',
                        'JIMM_LOG_LEVEL': 'info',
                        'JIMM_UUID': '1234567890',
                        'JIMM_WATCH_CONTROLLERS': '1'
                    }
                }
            }}
        )

    def test_bakery_configuration(self):
        container = self.harness.model.unit.get_container("jimm")
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        self.harness.update_config({
            "uuid": "1234567890",
            "dns-name": "jimm.testing",
            "dsn": "test-dsn",
            "candid-url": "test-candid-url",
            'candid-agent-username': 'test-username',
            'candid-agent-public-key': 'test-public-key',
            'candid-agent-private-key': 'test-private-key'
        })

        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(
            plan.to_dict(),
            {'services': {
                'jimm': {
                    'summary': 'JAAS Intelligent Model Manager',
                    'startup': 'disabled',
                    'override': 'merge',
                    'command': '/root/jimmsrv',
                    'environment': {
                        'BAKERY_AGENT_FILE': '/root/config/agent.json',
                        'CANDID_URL': 'test-candid-url',
                        'JIMM_DNS_NAME': 'jimm.testing',
                        'JIMM_DSN': 'test-dsn',
                        'JIMM_DASHBOARD_LOCATION': 'https://jaas.ai/models',
                        'JIMM_LISTEN_ADDR': ':8080',
                        'JIMM_LOG_LEVEL': 'info',
                        'JIMM_UUID': '1234567890'
                    }
                }
            }}
        )
        agent_data = container.pull("/root/config/agent.json")
        agent_json = json.loads(agent_data.read())
        self.assertEqual(
            agent_json, {
                'key': {
                    'public': 'test-public-key',
                    'private': 'test-private-key'
                },
                'agents': [{
                    'url': 'test-candid-url',
                    'username': 'test-username'
                }]
            }
        )

    def test_on_leader_elected(self):
        container = self.harness.model.unit.get_container("jimm")
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        rel_id = self.harness.add_relation('dashboard', 'juju-dashboard')
        self.harness.add_relation_unit(rel_id, 'juju-dashboard/0')


        self.harness.set_leader(True)
        self.harness.update_config(MINIMAL_CONFIG)

        self.harness.charm.on.leader_elected.emit()

        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(
            plan.to_dict(),
            {'services': {
                'jimm': {
                    'summary': 'JAAS Intelligent Model Manager',
                    'startup': 'disabled',
                    'override': 'merge',
                    'command': '/root/jimmsrv',
                    'environment': {
                        'CANDID_URL': 'test-candid-url',
                        'JIMM_DASHBOARD_LOCATION': 'https://jaas.ai/models',
                        'JIMM_DNS_NAME': 'jimm.testing',
                        'JIMM_DSN': 'test-dsn',
                        'JIMM_LISTEN_ADDR': ':8080',
                        'JIMM_LOG_LEVEL': 'info',
                        'JIMM_UUID': '1234567890',
                        'JIMM_WATCH_CONTROLLERS': '1'
                    }
                }
            }}
        )

    def test_on_website_relation_joined(self):
        self.harness.disable_hooks()
        self.harness.update_config(MINIMAL_CONFIG)
        self.harness.enable_hooks()

        container = self.harness.model.unit.get_container('jimm')
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        rel_id = self.harness.add_relation('website', 'haproxy')
        self.harness.add_relation_unit(rel_id, 'haproxy/0')

        rel_data = self.harness.get_relation_data(
            rel_id,
            self.harness.charm.unit.name
        )
        self.assertEqual(rel_data.get('port'), '8080')

    @patch('hvac.Client.sys')
    def test_vault_configuration(self, client):
        client.unwrap.return_value = {
            'key1': 'value1',
            'data': {
                'key2': 'value2'
            }
        }
        container = self.harness.model.unit.get_container("jimm")
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        self.harness.disable_hooks()
        self.harness.update_config({
            'uuid': '1234567890',
            'dns-name': 'jimm.testing',
            'dsn': 'test-dsn',
            'candid-url': 'test-candid-url',
            'vault-url': 'test-vault-url',
            'vault-role-id': 'test-vault-role-id',
            'vault-token': 'test-vault-token'
        })
        self.harness.enable_hooks()

        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(
            plan.to_dict(),
            {'services': {
                'jimm': {
                    'summary': 'JAAS Intelligent Model Manager',
                    'startup': 'disabled',
                    'override': 'merge',
                    'command': '/root/jimmsrv',
                    'environment': {
                        'CANDID_URL': 'test-candid-url',
                        'JIMM_DNS_NAME': 'jimm.testing',
                        'JIMM_DSN': 'test-dsn',
                        'JIMM_DASHBOARD_LOCATION': 'https://jaas.ai/models',
                        'JIMM_LISTEN_ADDR': ':8080',
                        'JIMM_LOG_LEVEL': 'info',
                        'JIMM_UUID': '1234567890',
                        'VAULT_ADDR': 'test-vault-url',
                        'VAULT_AUTH_PATH': '/auth/approle/login',
                        'VAULT_PATH': 'charm-jimm-creds',
                        'VAULT_SECRET_FILE': '/root/config/vault_secret.json'
                    }
                }
            }}
        )

        vault_data = container.pull("/root/config/vault_secret.json")
        vault_json = json.loads(vault_data.read())
        self.assertEqual(
            vault_json, {
                'key1': 'value1',
                'data': {
                    'key2': 'value2',
                    'role_id': 'test-vault-role-id'
                }
            }
        )

    def test_install_dashboard(self):
        container = self.harness.model.unit.get_container("jimm")

        self.harness.add_resource("dashboard", self.dashboard_tarfile())

        self.harness.update_config(MINIMAL_CONFIG)
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(
            plan.to_dict(),
            {'services': {
                'jimm': {
                    'summary': 'JAAS Intelligent Model Manager',
                    'startup': 'disabled',
                    'override': 'merge',
                    'command': '/root/jimmsrv',
                    'environment': {
                        'CANDID_URL': 'test-candid-url',
                        'JIMM_DNS_NAME': 'jimm.testing',
                        'JIMM_DSN': 'test-dsn',
                        'JIMM_LISTEN_ADDR': ':8080',
                        'JIMM_DASHBOARD_LOCATION': '/root/dashboard',
                        'JIMM_LOG_LEVEL': 'info',
                        'JIMM_UUID': '1234567890'
                    }
                }
            }}
        )

        self.assertEqual(
            container.exists('/root/dashboard'),
            True
        )
        self.assertEqual(
            container.isdir('/root/dashboard'),
            True
        )
        self.assertEqual(
            container.exists('/root/dashboard/README.md'),
            True
        )
        self.assertEqual(
            container.exists('/root/dashboard/hash'),
            True
        )

        readme_data = container.pull('/root/dashboard/README.md')
        self.assertEqual(
            readme_data.read(),
            'Hello world'
        )

    def dashboard_tarfile(self):
        dashboard_archive = io.BytesIO()

        data = bytes('Hello world', 'utf-8')
        f = io.BytesIO(initial_bytes=data)
        with tarfile.open(fileobj=dashboard_archive, mode='w:bz2') as tar:
            info = tarfile.TarInfo('README.md')
            info.size = len(data)
            tar.addfile(info, f)
            tar.close()

        dashboard_archive.flush()
        dashboard_archive.seek(0)
        data = dashboard_archive.read()
        return data

    def test_dashboard_relation_joined(self):
        harness = Harness(JimmOperatorCharm)
        self.addCleanup(harness.cleanup)
        harness.begin()
        harness.set_leader(True)
        harness.update_config({
            "dns-name": "https://jimm.example.com",
            "candid-agent-username": "username@candid",
            "candid-agent-private-key": "agent-private-key",
            "candid-agent-public-key": "agent-public-key",
            "candid-url": "https://candid.example.com",
            "controller-admins": "user1 user2 group1",
            "uuid": "caaa4ba4-e2b5-40dd-9bf3-2bd26d6e17aa",
        })

        id = harness.add_relation('dashboard', 'juju-dashboard')
        harness.add_relation_unit(id, 'juju-dashboard/0')
        data = harness.get_relation_data(id, "jimm-k8s")
        self.assertTrue(data)
        self.assertEqual(data["controller-url"], "https://jimm.example.com")
        self.assertEqual(data["identity-provider-url"], "https://candid.example.com")
        self.assertEqual(data["is-juju"], "False")
