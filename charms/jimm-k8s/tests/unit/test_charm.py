# Copyright 2022 Canonical Ltd
# See LICENSE file for licensing details.
#
# Learn more about testing at: https://juju.is/docs/sdk/testing


import json
import pathlib
import tempfile
import unittest

from ops.model import ActiveStatus, BlockedStatus
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

OPENFGA_PROVIDER_INFO = {
    "address": "openfga.localhost",
    "port": "8080",
    "scheme": "http",
    "store_id": "fake-store-id",
    "token": "fake-token",
}

MINIMAL_CONFIG = {
    "uuid": "1234567890",
    "public-key": "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
    "private-key": "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
    "final-redirect-url": "some-url",
}

EXPECTED_ENV = {
    "JIMM_DASHBOARD_LOCATION": "https://jaas.ai/models",
    "JIMM_DNS_NAME": "juju-jimm-k8s-0.juju-jimm-k8s-endpoints.None.svc.cluster.local",
    "JIMM_ENABLE_JWKS_ROTATOR": "1",
    "JIMM_LISTEN_ADDR": ":8080",
    "JIMM_LOG_LEVEL": "info",
    "JIMM_UUID": "1234567890",
    "JIMM_WATCH_CONTROLLERS": "1",
    "BAKERY_PRIVATE_KEY": "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
    "BAKERY_PUBLIC_KEY": "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
    "JIMM_AUDIT_LOG_RETENTION_PERIOD_IN_DAYS": "0",
    "JIMM_MACAROON_EXPIRY_DURATION": "24h",
    "JIMM_JWT_EXPIRY": "5m",
    "JIMM_ACCESS_TOKEN_EXPIRY_DURATION": "6h",
    "JIMM_OAUTH_ISSUER_URL": OAUTH_PROVIDER_INFO["issuer_url"],
    "JIMM_OAUTH_CLIENT_ID": OAUTH_CLIENT_ID,
    "JIMM_OAUTH_CLIENT_SECRET": OAUTH_CLIENT_SECRET,
    "JIMM_OAUTH_SCOPES": OAUTH_PROVIDER_INFO["scope"],
    "JIMM_DASHBOARD_FINAL_REDIRECT_URL:": "some-url",
    "JIMM_SECURE_SESSION_COOKIES:": True,
    "JIMM_SESSION_COOKIE_MAX_AGE:": 86400,
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

    def add_openfga_relation(self):
        self.openfga_rel_id = self.harness.add_relation("openfga", "openfga")
        self.harness.add_relation_unit(self.openfga_rel_id, "openfga/0")
        self.harness.update_relation_data(
            self.openfga_rel_id,
            "openfga",
            {
                **OPENFGA_PROVIDER_INFO,
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

    def test_app_dns_address(self):
        self.harness.update_config(MINIMAL_CONFIG)
        self.harness.update_config({"dns-name": "jimm.com"})
        oauth_client = self.harness.charm._oauth_client_config
        self.assertEqual(oauth_client.redirect_uri, "https://jimm.com/oauth/callback")

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
        self.assertEqual(data["is_juju"], "False")

    def test_vault_relation_joined(self):

        self.harness.enable_hooks()
        self.harness.charm.on.install.emit()

        id = self.harness.add_relation("vault", "vault-k8s")
        self.harness.add_relation_unit(id, "vault-k8s/0")

        self.harness.update_config(MINIMAL_CONFIG)

        data = self.harness.get_relation_data(id, "juju-jimm-k8s/0")
        self.assertTrue(data)
        self.assertTrue("egress_subnet" in data)
        self.assertTrue("nonce" in data)

        secret_id = self.harness.add_model_secret(
            "vault-k8s/0",
            {"role-id": "111", "role-secret-id": "222"},
        )
        self.harness.grant_secret(secret_id, "juju-jimm-k8s")

        credentials = {data["nonce"]: secret_id}
        self.harness.update_relation_data(
            id,
            "vault-k8s",
            {
                "vault_url": "127.0.0.1:8081",
                "ca_certificate": "abcd",
                "mount": "charm-juju-jimm-k8s-jimm",
                "credentials": json.dumps(credentials, sort_keys=True),
            },
        )

        plan = self.harness.get_container_pebble_plan("jimm")
        expected_env = EXPECTED_ENV.copy()
        expected_env.update(
            {
                "VAULT_ADDR": "127.0.0.1:8081",
                "VAULT_CACERT_BYTES": "abcd",
                "VAULT_PATH": "charm-juju-jimm-k8s-jimm",
                "VAULT_ROLE_ID": "111",
                "VAULT_ROLE_SECRET_ID": "222",
            }
        )
        self.assertEqual(plan.to_dict(), get_expected_plan(expected_env))

    def test_app_blocked_without_private_key(self):
        self.harness.enable_hooks()
        # Fake the Postgres relation.
        self.harness.charm._state.dsn = "postgres-dsn"
        # Setup the OpenFGA relation.
        self.add_openfga_relation()
        self.harness.charm._state.openfga_auth_model_id = 1
        # Set the config with the private-key value missing.
        min_config_no_private_key = MINIMAL_CONFIG.copy()
        del min_config_no_private_key["private-key"]
        self.harness.update_config(min_config_no_private_key)
        self.assertEqual(self.harness.charm.unit.status.name, BlockedStatus.name)
        self.assertEqual(
            self.harness.charm.unit.status.message,
            "BAKERY_PRIVATE_KEY configuration value not set: missing private key configuration",
        )
        # Now check that we can get the app into an active state.
        self.harness.update_config(MINIMAL_CONFIG)
        self.assertEqual(self.harness.charm.unit.status.name, ActiveStatus.name)
        self.assertEqual(self.harness.charm.unit.status.message, "running")
