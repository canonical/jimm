#!/usr/bin/env python3
# Copyright 2022 Canonical Ltd
# See LICENSE file for licensing details.

import asyncio
import logging
from pathlib import Path

import pytest
import utils
import yaml
from pytest_operator.plugin import OpsTest

logger = logging.getLogger(__name__)

METADATA = yaml.safe_load(Path("./metadata.yaml").read_text())
APP_NAME = "juju-jimm-k8s"


@pytest.mark.abort_on_fail
async def test_upgrade_running_application(ops_test: OpsTest, local_charm):
    """Deploy latest published charm and upgrade it with charm-under-test.

    Assert on the application status and health check endpoint after upgrade/refresh took place.
    """

    # Deploy the charm and wait for active/idle status
    logger.debug("deploying charms")
    await ops_test.model.deploy(
        METADATA["name"],
        channel="edge",
        application_name=APP_NAME,
        series="focal",
        config={
            "uuid": "f4dec11e-e2b6-40bb-871a-cc38e958af49",
            "candid-url": "https://api.jujucharms.com/identity",
        },
    )
    await ops_test.model.deploy(
        "traefik-k8s",
        application_name="traefik",
        config={
            "external_hostname": "traefik.test.canonical.com",
        },
    )
    await asyncio.gather(
        ops_test.model.deploy("postgresql-k8s", application_name="postgresql", channel="14/stable", trust=True),
    )

    logger.info("waiting for postgresql and traefik")
    await ops_test.model.wait_for_idle(
        apps=["postgresql", "traefik"],
        status="active",
        raise_on_blocked=True,
        timeout=40000,
    )

    logger.info("adding traefik relation")
    await ops_test.model.relate("{}:ingress".format(APP_NAME), "traefik")

    logger.info("adding postgresql relation")
    await ops_test.model.relate(APP_NAME, "postgresql:database")

    logger.info("waiting for jimm")
    await ops_test.model.wait_for_idle(
        apps=[APP_NAME],
        status="active",
        # raise_on_blocked=True,
        timeout=40000,
    )

    assert ops_test.model.applications[APP_NAME].status == "active"

    # Starting upgrade/refresh
    logger.info("starting upgrade test")

    # Build and deploy charm from local source folder
    logger.info("building local charm")

    # (Optionally build) and deploy charm from local source folder
    if local_charm:
        charm = Path(utils.get_local_charm()).resolve()
    else:
        charm = await ops_test.build_charm(".")
    resources = {"jimm-image": "localhost:32000/jimm:latest"}

    # Deploy the charm and wait for active/idle status
    logger.info("refreshing running application with the new local charm")

    await ops_test.model.applications[APP_NAME].refresh(
        path=charm,
        resources=resources,
    )

    logger.info("waiting for the upgraded unit to be ready")
    async with ops_test.fast_forward():
        await ops_test.model.wait_for_idle(
            apps=[APP_NAME],
            status="active",
            timeout=60,
        )

    assert ops_test.model.applications[APP_NAME].status == "active"

    logger.info("checking status of the running unit")
    upgraded_jimm_unit = await utils.get_unit_by_name(APP_NAME, "0", ops_test.model.units)

    health = await upgraded_jimm_unit.run("curl -i http://localhost:8080/debug/status")
    await health.wait()
    assert health.results.get("return-code") == 0
    assert health.results.get("stdout").strip().splitlines()[0].endswith("200 OK")