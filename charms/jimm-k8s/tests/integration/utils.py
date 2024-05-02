import asyncio
import glob
import logging
import os
import time
from pathlib import Path
from typing import Dict
from urllib.parse import ParseResult

import utils
import yaml
from juju.action import Action
from juju.unit import Unit
from oauth_tools.conftest import *  # noqa
from oauth_tools.constants import APPS
from oauth_tools.dex import ExternalIdpManager
from oauth_tools.oauth_test_helper import deploy_identity_bundle
from pytest_operator.plugin import OpsTest

logger = logging.getLogger(__name__)
METADATA = yaml.safe_load(Path("./metadata.yaml").read_text())
APP_NAME = "juju-jimm-k8s"


async def get_unit_by_name(unit_name: str, unit_index: str, unit_list: Dict[str, Unit]) -> Unit:
    return unit_list.get("{unitname}/{unitindex}".format(unitname=unit_name, unitindex=unit_index))


def get_local_charm():
    charm = glob.glob("./*.charm")
    if len(charm) != 1:
        raise ValueError(f"Found {len(charm)} file(s) with .charm extension.")
    return charm[0]


class JimmEnv:
    def __init__(self, jimm_address: ParseResult, idp_manager: ExternalIdpManager) -> None:
        self.jimm_address = jimm_address
        self.idp_manager = idp_manager


async def deploy_jimm(ops_test: OpsTest, local_charm: bool) -> JimmEnv:
    """(Optionally) Build and then deploy JIMM and all dependencies.

    Args:
        ops_test (OpsTest): Fixture for testing operator charms
        local_charm (bool): Flag to indicate whether to build the charm under test.

    Returns:
        JimmEnv: A class with member variables that are useful for test functions.
    """
    # Build and deploy charm from local source folder
    # (Optionally build) and deploy charm from local source folder
    if local_charm:
        charm = Path(utils.get_local_charm()).resolve()
    else:
        charm = await ops_test.build_charm(".")
    resources = {"jimm-image": "localhost:32000/jimm:latest"}

    jimm_address = ParseResult(scheme="http", netloc="test.jimm.localhost", path="", params="", query="", fragment="")
    # Instantiating the ExternalIdpManager object deploys the external identity provider.
    external_idp_manager = ExternalIdpManager(ops_test=ops_test)

    # Deploy the identity bundle first because it checks everything is in an active state and if we deploy JIMM apps
    # at the same time, then that check will fail.
    logger.debug("deploying identity bundle")
    async with ops_test.fast_forward():
        await asyncio.gather(
            deploy_identity_bundle(ops_test=ops_test, external_idp_manager=external_idp_manager),
        )

    # Deploy the charm and wait for active/idle status
    logger.debug("deploying charms")
    async with ops_test.fast_forward():
        await asyncio.gather(
            ops_test.model.deploy(
                charm,
                resources=resources,
                application_name=APP_NAME,
                series="focal",
                config={
                    "uuid": "f4dec11e-e2b6-40bb-871a-cc38e958af49",
                    "dns-name": jimm_address.netloc,
                    "final-redirect-url": os.path.join(jimm_address.netloc, "debug/info"),
                    "public-key": "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
                    "private-key": "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
                    "postgres-secret-storage": True,
                },
            ),
            ops_test.model.deploy("nginx-ingress-integrator", application_name="jimm-ingress", channel="latest/stable"),
            ops_test.model.deploy(
                "postgresql-k8s",
                application_name="jimm-db",
                channel="14/stable",
            ),
            ops_test.model.deploy(
                "openfga-k8s",
                application_name="openfga",
                channel="latest/stable",
            ),
        )

    logger.info("waiting for postgresql")
    await ops_test.model.wait_for_idle(
        apps=["jimm-db"],
        status="active",
        raise_on_blocked=True,
        timeout=2000,
    )

    logger.info("adding custom ca cert relation")
    await ops_test.model.relate("{}:receive-ca-cert".format(APP_NAME), APPS.SELF_SIGNED_CERTIFICATES)

    logger.info("adding ingress relation")
    await ops_test.model.relate("{}:nginx-route".format(APP_NAME), "jimm-ingress")

    logger.info("adding openfga postgresql relation")
    await ops_test.model.relate("openfga:database", "jimm-db:database")

    logger.info("adding openfga relation")
    await ops_test.model.relate(APP_NAME, "openfga")

    logger.info("adding postgresql relation")
    await ops_test.model.relate(APP_NAME, "jimm-db:database")

    logger.info("adding ouath relation")
    await ops_test.model.integrate(f"{APP_NAME}:oauth", APPS.HYDRA)

    logger.info("waiting for jimm to be blocked pending auth model creation")
    await ops_test.model.wait_for_idle(
        apps=[APP_NAME],
        status="blocked",
        timeout=2000,
    )

    logger.info("running the create authorization model action")
    jimm_unit = await utils.get_unit_by_name(APP_NAME, "0", ops_test.model.units)
    with open("../../local/openfga/authorisation_model.json", "r") as model_file:
        model_data = model_file.read()
        for i in range(10):
            action: Action = await jimm_unit.run_action(
                "create-authorization-model",
                model=model_data,
            )
            result = await action.wait()
            logger.info("attempt {} -> action result {} {}".format(i, result.status, result.results))
            if result.results == {"return-code": 0}:
                break
            time.sleep(2)
    assert ops_test.model.applications[APP_NAME].status == "active"
    logger.info("jimm is active and ready")
    return JimmEnv(jimm_address, external_idp_manager)
