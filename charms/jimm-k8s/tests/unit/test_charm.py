# Copyright 2022 Canonical Ltd
# See LICENSE file for licensing details.
#
# Learn more about testing at: https://juju.is/docs/sdk/testing


import json
import pathlib
import tempfile
import unittest
from unittest.mock import patch

from ops.model import BlockedStatus
from ops.testing import Harness

from src.charm import JimmOperatorCharm

OAUTH_CLIENT_ID = "jimm_client_id"
OAUTH_CLIENT_SECRET = "test-secret"
OAUTH_PROVIDER_INFO = {
    "authorization_endpoint": "https://example.oidc.com/oauth2/auth",
    "introspection_endpoint": "https://example.oidc.com/admin/oauth2/introspect",
    "issuer_url": "https://example.oidc.com",
    "jwks_endpoint": "https://example.oidc.com/.well-known/jwks.json",
    "scope": "openid profile email phone",
    "token_endpoint": "https://example.oidc.com/oauth2/token",
    "userinfo_endpoint": "https://example.oidc.com/userinfo",
}

MINIMAL_CONFIG = {
    "uuid": "1234567890",
    "candid-url": "test-candid-url",
    "public-key": "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
    "private-key": "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
    "vault-access-address": "10.0.1.123",
}

EXPECTED_ENV = {
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
    "JIMM_AUDIT_LOG_RETENTION_PERIOD_IN_DAYS": "0",
    "JIMM_MACAROON_EXPIRY_DURATION": "24h",
    "JIMM_JWT_EXPIRY": "5m",
    "JIMM_ACCESS_TOKEN_EXPIRY_DURATION": "6h",
    "JIMM_OAUTH_ISSUER_URL": OAUTH_PROVIDER_INFO["issuer_url"],
    "JIMM_OAUTH_CLIENT_ID": OAUTH_CLIENT_ID,
    "JIMM_OAUTH_CLIENT_SECRET": OAUTH_CLIENT_SECRET,
    "JIMM_OAUTH_SCOPES": OAUTH_PROVIDER_INFO["scope"],
}


def get_expected_plan(env):
    return {
        "services": {
            "jimm": {
                "summary": "JAAS Intelligent Model Manager",
                "startup": "disabled",
                "override": "replace",
                "command": "/root/jimmsrv",
                "environment": env,
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

        jimm_id = self.harness.add_relation("peer", "juju-jimm-k8s")
        self.harness.add_relation_unit(jimm_id, "juju-jimm-k8s/1")
        self.harness.container_pebble_ready("jimm")

        self.ingress_rel_id = self.harness.add_relation("ingress", "nginx-ingress")
        self.harness.add_relation_unit(self.ingress_rel_id, "nginx-ingress/0")

        self.oauth_rel_id = self.harness.add_relation("oauth", "hydra")
        self.harness.add_relation_unit(self.oauth_rel_id, "hydra/0")
        secret_id = self.harness.add_model_secret("hydra", {"secret": OAUTH_CLIENT_SECRET})
        self.harness.grant_secret(secret_id, "juju-jimm-k8s")
        self.harness.update_relation_data(
            self.oauth_rel_id,
            "hydra",
            {
                "client_id": OAUTH_CLIENT_ID,
                "client_secret_id": secret_id,
                **OAUTH_PROVIDER_INFO,
            },
        )

    # import ipdb; ipdb.set_trace()
    def test_on_pebble_ready(self):
        self.harness.update_config(MINIMAL_CONFIG)

        container = self.harness.model.unit.get_container("jimm")
        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(plan.to_dict(), get_expected_plan(EXPECTED_ENV))

    def test_on_config_changed(self):
        container = self.harness.model.unit.get_container("jimm")
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        self.harness.update_config(MINIMAL_CONFIG)
        self.harness.set_leader(True)

        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(plan.to_dict(), get_expected_plan(EXPECTED_ENV))

    def test_postgres_secret_storage_config(self):
        container = self.harness.model.unit.get_container("jimm")
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        self.harness.update_config(MINIMAL_CONFIG)
        self.harness.update_config({"postgres-secret-storage": True})
        self.harness.set_leader(True)

        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        expected_env = EXPECTED_ENV.copy()
        expected_env.update({"INSECURE_SECRET_STORAGE": "enabled"})
        self.assertEqual(plan.to_dict(), get_expected_plan(expected_env))

    def test_app_enters_block_states_if_oauth_relation_removed(self):
        self.harness.update_config(MINIMAL_CONFIG)
        self.harness.remove_relation(self.oauth_rel_id)
        container = self.harness.model.unit.get_container("jimm")
        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        # Check the that the plan is empty
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(plan.to_dict(), {})
        self.assertEqual(self.harness.charm.unit.status.name, BlockedStatus.name)
        self.assertEqual(self.harness.charm.unit.status.message, "Waiting for OAuth relation")

    def test_app_enters_block_state_if_oauth_relation_not_ready(self):
        self.harness.update_config(MINIMAL_CONFIG)
        self.harness.remove_relation(self.oauth_rel_id)
        oauth_relation = self.harness.add_relation("oauth", "hydra")
        self.harness.add_relation_unit(oauth_relation, "hydra/0")
        secret_id = self.harness.add_model_secret("hydra", {"secret": OAUTH_CLIENT_SECRET})
        self.harness.grant_secret(secret_id, "juju-jimm-k8s")
        # If the client-id is empty we should detect that the oauth relation is not ready.
        # The readiness check is handled by the OAuth library.
        self.harness.update_relation_data(
            oauth_relation,
            "hydra",
            {"client_id": ""},
        )
        container = self.harness.model.unit.get_container("jimm")
        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        # Check the that the plan is empty
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(plan.to_dict(), {})
        self.assertEqual(self.harness.charm.unit.status.name, BlockedStatus.name)
        self.assertEqual(self.harness.charm.unit.status.message, "Waiting for OAuth relation")

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
        expected_env = EXPECTED_ENV.copy()
        expected_env.update({"BAKERY_AGENT_FILE": "/root/config/agent.json"})
        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(plan.to_dict(), get_expected_plan(expected_env))
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

    def test_audit_log_retention_config(self):
        container = self.harness.model.unit.get_container("jimm")
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        self.harness.update_config(MINIMAL_CONFIG)
        self.harness.update_config({"audit-log-retention-period-in-days": "10"})

        # Emit the pebble-ready event for jimm
        self.harness.charm.on.jimm_pebble_ready.emit(container)
        expected_env = EXPECTED_ENV.copy()
        expected_env.update({"JIMM_AUDIT_LOG_RETENTION_PERIOD_IN_DAYS": "10"})
        # Check the that the plan was updated
        plan = self.harness.get_container_pebble_plan("jimm")
        self.assertEqual(plan.to_dict(), get_expected_plan(expected_env))

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
            data["controller_url"],
            "wss://juju-jimm-k8s-0.juju-jimm-k8s-endpoints.None.svc.cluster.local",
        )
        self.assertEqual(data["identity_provider_url"], "https://candid.example.com")
        self.assertEqual(data["is_juju"], "False")

    @patch("socket.gethostname")
    @patch("hvac.Client.sys")
    def test_vault_relation_joined(self, hvac_client_sys, gethostname):
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
                "vault-access-address": "10.0.1.123",
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
        self.assertEqual(data["access_address"], "10.0.1.123")

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

    def test_app_enters_blocked_state_if_vault_related_but_not_ready(self):
        self.harness.update_config(MINIMAL_CONFIG)
        container = self.harness.model.unit.get_container("jimm")
        # Emit the pebble-ready event for jimm
        self.harness.add_relation("vault", "remote-app-name")
        self.harness.charm.on.jimm_pebble_ready.emit(container)

        self.assertEqual(self.harness.charm.unit.status.name, BlockedStatus.name)
        self.assertEqual(
            self.harness.charm.unit.status.message, "Vault relation present but vault setup is not ready yet"
        )

    def test_app_raises_error_without_vault_config(self):
        self.harness.enable_hooks()
        minim_config_no_vault_config = MINIMAL_CONFIG.copy()
        del minim_config_no_vault_config["vault-access-address"]
        self.harness.update_config(minim_config_no_vault_config)
        id = self.harness.add_relation("vault", "vault")
        with self.assertRaises(ValueError) as e:
            self.harness.add_relation_unit(id, "vault/0")
            self.assertEqual(e, "Missing config vault-access-address for vault relation")
