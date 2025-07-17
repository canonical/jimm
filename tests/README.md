# Description

This folder is intended for integration tests that are complex in nature
and may involve multiple Juju controllers.

# Structure

Each folder should contain any number of shell scripts, with 1 test per file.
Each folder groups similar tests but a folder may contain only a single test.

# Writing tests

All tests should assume that they will run from the root project directory.
Tests can use helper methods from the local/ directory which contains
helpers for starting Juju controllers, adding them to JIMM, etc.
Tests should run those helpers with `local/<path-to-script>`.

# Running tests

Tests should be run from the root directory:
- To run an invidial test `tests/<folder>/<test-script>`.
- To run all tests `tests/run-tests.sh`
