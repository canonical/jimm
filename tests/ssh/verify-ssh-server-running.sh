#!/bin/bash

# This script verifies that the SSH server is running on the JIMM controller.

# TODO(simonedutto): improve this test when juju ssh controller is implemented.
nc -zv localhost 17022
