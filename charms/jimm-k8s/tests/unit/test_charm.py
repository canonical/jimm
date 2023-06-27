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

from ops.testing import Harness

from src.charm import JimmOperatorCharm

MINIMAL_CONFIG = {
    "uuid": "1234567890",
    "candid-url": "test-candid-url",
    "public-key": "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
    "private-key": "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
}


class MockExec:
    def wait_output():
        return True


class TestCharm(unittest.TestCase):
    def setUp(self):
        self.maxDiff = None
        self.harness = Harness(JimmOperatorCharm)
        self.addCleanup(self.harness.cleanup)
        self.harness.disable_hooks()
        self.harness.add_oci_resource("jimm-image")
        self.harness.set_can_connect("jimm", True)
        self.harness.set_leader(True)
        self.harness.begin()

        self.tempdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tempdir.cleanup)
        self.harness.charm.framework.charm_dir = pathlib.Path(self.tempdir.name)

        self.harness.add_relation("peer", "jimm")
        self.harness.container_pebble_ready("jimm")

        rel_id = self.harness.add_relation("ingress", "nginx-ingress")
        self.harness.add_relation_unit(rel_id, "nginx-ingress/0")

    # import ipdb; ipdb.set_trace()
    def test_on_pebble_ready(self):
        self.harness.update_config(MINIMAL_CONFIG)

        container = self.harness.model.unit.get_container("jimm")
        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(
            plan.to_dict(),
            {
                "services": {
                    "jimm": {
                        "summary": "JAAS Intelligent Model Manager",
                        "startup": "disabled",
                        "override": "merge",
                        "command": "/root/jimmsrv",
                        "environment": {
                            "CANDID_URL": "test-candid-url",
                            "JIMM_DASHBOARD_LOCATION": "https://jaas.ai/models",
                            "JIMM_DNS_NAME": "juju-jimm-k8s-0.juju-jimm-k8s-endpoints.None.svc.cluster.local",
                            "JIMM_ENABLE_JWKS_ROTATOR": "1",
                            "JIMM_LISTEN_ADDR": ":8080",
                            "JIMM_LOG_LEVEL": "info",
                            "JIMM_UUID": "1234567890",
                            "JIMM_WATCH_CONTROLLERS": "1",
                            "PRIVATE_KEY": "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
                            "PUBLIC_KEY": "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
                        },
                    }
                }
            },
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
            {
                "services": {
                    "jimm": {
                        "summary": "JAAS Intelligent Model Manager",
                        "startup": "disabled",
                        "override": "merge",
                        "command": "/root/jimmsrv",
                        "environment": {
                            "CANDID_URL": "test-candid-url",
                            "JIMM_DASHBOARD_LOCATION": "https://jaas.ai/models",
                            "JIMM_DNS_NAME": "juju-jimm-k8s-0.juju-jimm-k8s-endpoints.None.svc.cluster.local",
                            "JIMM_ENABLE_JWKS_ROTATOR": "1",
                            "JIMM_LISTEN_ADDR": ":8080",
                            "JIMM_LOG_LEVEL": "info",
                            "JIMM_UUID": "1234567890",
                            "JIMM_WATCH_CONTROLLERS": "1",
                            "PRIVATE_KEY": "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
                            "PUBLIC_KEY": "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
                        },
                    }
                }
            },
        )

    def test_bakery_configuration(self):
        container = self.harness.model.unit.get_container("jimm")
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        self.harness.update_config(
            {
                "uuid": "1234567890",
                "candid-url": "test-candid-url",
                "candid-agent-username": "test-username",
                "candid-agent-public-key": "test-public-key",
                "candid-agent-private-key": "test-private-key",
                "public-key": "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
                "private-key": "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
            }
        )

        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(
            plan.to_dict(),
            {
                "services": {
                    "jimm": {
                        "summary": "JAAS Intelligent Model Manager",
                        "startup": "disabled",
                        "override": "merge",
                        "command": "/root/jimmsrv",
                        "environment": {
                            "BAKERY_AGENT_FILE": "/root/config/agent.json",
                            "CANDID_URL": "test-candid-url",
                            "JIMM_DASHBOARD_LOCATION": "https://jaas.ai/models",
                            "JIMM_DNS_NAME": "juju-jimm-k8s-0.juju-jimm-k8s-endpoints.None.svc.cluster.local",
                            "JIMM_ENABLE_JWKS_ROTATOR": "1",
                            "JIMM_LISTEN_ADDR": ":8080",
                            "JIMM_LOG_LEVEL": "info",
                            "JIMM_UUID": "1234567890",
                            "JIMM_WATCH_CONTROLLERS": "1",
                            "PRIVATE_KEY": "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
                            "PUBLIC_KEY": "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
                        },
                    }
                }
            },
        )
        agent_data = container.pull("/root/config/agent.json")
        agent_json = json.loads(agent_data.read())
        self.assertEqual(
            agent_json,
            {
                "key": {
                    "public": "test-public-key",
                    "private": "test-private-key",
                },
                "agents": [{"url": "test-candid-url", "username": "test-username"}],
            },
        )

    @patch("ops.model.Container.exec")
    def test_install_dashboard(self, exec):
        exec.unwrap.return_value = MockExec()

        container = self.harness.model.unit.get_container("jimm")

        self.harness.add_resource("dashboard", self.dashboard_tarfile())

        self.harness.update_config(MINIMAL_CONFIG)
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(
            plan.to_dict(),
            {
                "services": {
                    "jimm": {
                        "summary": "JAAS Intelligent Model Manager",
                        "startup": "disabled",
                        "override": "merge",
                        "command": "/root/jimmsrv",
                        "environment": {
                            "CANDID_URL": "test-candid-url",
                            "JIMM_LISTEN_ADDR": ":8080",
                            "JIMM_DASHBOARD_LOCATION": "/root/dashboard",
                            "JIMM_DNS_NAME": "juju-jimm-k8s-0.juju-jimm-k8s-endpoints.None.svc.cluster.local",
                            "JIMM_ENABLE_JWKS_ROTATOR": "1",
                            "JIMM_LOG_LEVEL": "info",
                            "JIMM_UUID": "1234567890",
                            "JIMM_WATCH_CONTROLLERS": "1",
                            "PRIVATE_KEY": "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
                            "PUBLIC_KEY": "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
                        },
                    }
                }
            },
        )

        self.assertEqual(container.exists("/root/dashboard"), True)
        self.assertEqual(container.isdir("/root/dashboard"), True)
        self.assertEqual(container.exists("/root/dashboard/dashboard.tar.bz2"), True)
        self.assertEqual(container.exists("/root/dashboard/hash"), True)

    def dashboard_tarfile(self):
        dashboard_archive = io.BytesIO()

        data = bytes("Hello world", "utf-8")
        f = io.BytesIO(initial_bytes=data)
        with tarfile.open(fileobj=dashboard_archive, mode="w:bz2") as tar:
            info = tarfile.TarInfo("README.md")
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

        id = harness.add_relation("peer", "juju-jimm-k8s")
        harness.add_relation_unit(id, "juju-jimm-k8s/1")
        harness.begin()
        harness.set_leader(True)
        harness.update_config(
            {
                "candid-agent-username": "username@candid",
                "candid-agent-private-key": "agent-private-key",
                "candid-agent-public-key": "agent-public-key",
                "candid-url": "https://candid.example.com",
                "controller-admins": "user1 user2 group1",
                "uuid": "caaa4ba4-e2b5-40dd-9bf3-2bd26d6e17aa",
            }
        )

        id = harness.add_relation("dashboard", "juju-dashboard")
        harness.add_relation_unit(id, "juju-dashboard/0")
        data = harness.get_relation_data(id, "juju-jimm-k8s")

        self.assertTrue(data)
        self.assertEqual(
            data["controller-url"],
            "juju-jimm-k8s-0.juju-jimm-k8s-endpoints.None.svc.cluster.local",
        )
        self.assertEqual(data["identity-provider-url"], "https://candid.example.com")
        self.assertEqual(data["is-juju"], "False")

    @patch("src.charm.JimmOperatorCharm._get_network_address")
    @patch("socket.gethostname")
    @patch("hvac.Client.sys")
    def test_vault_relation_joined(self, hvac_client_sys, gethostname, get_network_address):
        get_network_address.return_value = "127.0.0.1:8080"
        gethostname.return_value = "test-hostname"
        hvac_client_sys.unwrap.return_value = {
            "key1": "value1",
            "data": {"key2": "value2"},
        }

        harness = Harness(JimmOperatorCharm)
        self.addCleanup(harness.cleanup)

        jimm_id = harness.add_relation("peer", "juju-jimm-k8s")
        harness.add_relation_unit(jimm_id, "juju-jimm-k8s/1")

        dashboard_id = harness.add_relation("dashboard", "juju-dashboard")
        harness.add_relation_unit(dashboard_id, "juju-dashboard/0")

        harness.update_config(
            {
                "candid-agent-username": "username@candid",
                "candid-agent-private-key": "agent-private-key",
                "candid-agent-public-key": "agent-public-key",
                "candid-url": "https://candid.example.com",
                "controller-admins": "user1 user2 group1",
                "uuid": "caaa4ba4-e2b5-40dd-9bf3-2bd26d6e17aa",
            }
        )
        harness.set_leader(True)
        harness.begin_with_initial_hooks()

        container = harness.model.unit.get_container("jimm")
        # Emit the pebble-ready event for jimm
        harness.charm.on.jimm_pebble_ready.emit(container)

        id = harness.add_relation("vault", "vault-k8s")
        harness.add_relation_unit(id, "vault-k8s/0")
        data = harness.get_relation_data(id, "juju-jimm-k8s/0")

        self.assertTrue(data)
        self.assertEqual(
            data["secret_backend"],
            '"charm-jimm-k8s-creds"',
        )
        self.assertEqual(data["hostname"], '"test-hostname"')
        self.assertEqual(data["access_address"], '"127.0.0.1:8080"')

        harness.update_relation_data(
            id,
            "vault-k8s/0",
            {
                "vault_url": '"127.0.0.1:8081"',
                "juju-jimm-k8s/0_role_id": '"juju-jimm-k8s-0-test-role-id"',
                "juju-jimm-k8s/0_token": '"juju-jimm-k8s-0-test-token"',
            },
        )

        vault_data = container.pull("/root/config/vault_secret.json")
        vault_json = json.loads(vault_data.read())
        self.assertEqual(
            vault_json,
            {
                "key1": "value1",
                "data": {
                    "key2": "value2",
                    "role_id": "juju-jimm-k8s-0-test-role-id",
                },
            },
        )
