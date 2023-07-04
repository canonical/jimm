def pytest_addoption(parser):
    parser.addoption("--localCharm", action="store_true", help="use local pre-built charm")


def pytest_generate_tests(metafunc):
    if "local_charm" in metafunc.fixturenames:
        if metafunc.config.getoption("localCharm"):
            local = True
        else:
            local = False
        metafunc.parametrize("local_charm", [local])
