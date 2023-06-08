import glob
import logging
from typing import Dict

from juju.unit import Unit

LOGGER = logging.getLogger(__name__)


async def get_unit_by_name(unit_name: str, unit_index: str, unit_list: Dict[str, Unit]) -> Unit:
    return unit_list.get("{unitname}/{unitindex}".format(unitname=unit_name, unitindex=unit_index))


def get_local_charm():
    charm = glob.glob("./*.charm")
    if len(charm) != 1:
        raise ValueError(f"Found {len(charm)} file(s) with .charm extension.")
    return charm[0]
