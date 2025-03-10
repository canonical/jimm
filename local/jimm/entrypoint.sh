#!/bin/sh

set -e

# This container expects the source code to be mounted at /jimm
cd /jimm
go build -buildvcs=false -o jimmsrv ./cmd/jimmsrv
# Exec ensures jimm is PID 1 for testing shutdown.
exec ./jimmsrv
