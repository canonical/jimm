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
async def test_build_and_deploy_with_ngingx(ops_test: OpsTest, local_charm):
    """Build the charm-under-test and deploy it together with related charms.

    Assert on the unit status before any relations/configurations take place.
    """
    # Build and deploy charm from local source folder
    # (Optionally build) and deploy charm from local source folder
    if local_charm:
        charm = Path(utils.get_local_charm()).resolve()
    else:
        charm = await ops_test.build_charm(".")
    resources = {"jimm-image": "localhost:32000/jimm:latest"}

    # Deploy the charm and wait for active/idle status
    logger.debug("deploying charms")
    await ops_test.model.deploy(
        charm,
        resources=resources,
        application_name=APP_NAME,
        series="focal",
        config={
            "uuid": "f4dec11e-e2b6-40bb-871a-cc38e958af49",
            "dns-name": "test.jimm.local",
            "candid-url": "https://api.jujucharms.com/identity",
            "public-key": "izcYsQy3TePp6bLjqOo3IRPFvkQd2IKtyODGqC6SdFk=",
            "private-key": "ly/dzsI9Nt/4JxUILQeAX79qZ4mygDiuYGqc2ZEiDEc=",
        },
    )
    await ops_test.model.deploy(
        "nginx-ingress-integrator",
        application_name="nginx",
    )
    await asyncio.gather(
        ops_test.model.deploy(
            "postgresql-k8s",
            application_name="postgresql",
            channel="edge",
        ),
    )

    logger.info("waiting for postgresql")
    await ops_test.model.wait_for_idle(
        apps=["postgresql", "nginx"],
        status="active",
        raise_on_blocked=True,
        timeout=40000,
    )

    logger.info("adding ingress relation")
    await ops_test.model.relate("{}:nginx-route".format(APP_NAME), "nginx")

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
