## Filing Bugs
File bugs at https://github.com/canonical/jimm/issues.

## Testing
Many tests in JIMM require real services to be reachable i.e. Postgres, Vault, OpenFGA 
and an IdP (Identity Provider).

JIMM's docker compose file provides a convenient way of starting these services.

### TLDR
Run:
```
$ make test-env
$ go test ./...
```

### Pre-requisite
To check if your system has all the prequisites installed simply run `make sys-deps`.
This will check for all test prequisites and inform you how to install them if not installed. 
You will need to install `make` first with `sudo apt install make`

### Understanding the test suite
In order to enable testing with Juju's internal suites, it is required to have juju-db 
(mongod) service installed.
This can be installed via: `sudo snap install juju-db --channel=4.4/stable`.

Tests inside of `cmd/` and `internal/jujuapi/` are integration based, spinning up JIMM and 
a Juju controller for testing. To spin up a Juju controller we use the `JujuConnSuite` which 
in turn uses the [gocheck](http://labix.org/gocheck) test library.

Because of the `JujuConnSuite` and its use in JIMM's test suites, there are 2 test libraries in JIMM:
- GoCheck based tests, identified in the function signature with `func Test(c *gc.C)`.
  - These tests normally interact with a Juju controller.
  - GoCheck should only be used when using the suites in `internal/jimmtest`.
- Stdlib `testing.T` tests, identified in the function signature with `func Test(t *testing.T)`.
  - These tests vary in their scope but do not require a Juju controller.
  - To provide assertions, the project uses [quicktest](https://github.com/frankban/quicktest), 
    a lean testing library.

Because many tests rely on PostgreSQL, OpenFGA and Vault which are dockerised 
you may simply run `make test-env` to be integration test ready.

The above command won't start a dockerised instance of JIMM as tests are normally run locally. 
Instead, to start a dockerised JIMM that will auto-reload on code changes, follow the instructions 
in `local/README.md`.

### Manual commands
If using VSCode, we recommend installing the 
[go-test-suite](https://marketplace.visualstudio.com/items?itemName=babakks.vscode-go-test-suite) 
extension to enable running these tests from the GUI as you would with normal Go tests and the Go 
VSCode extension.

Because [gocheck](http://labix.org/gocheck) does not parse the `go test -run` flags, the examples 
below show how to run individual tests in a suite: 
```bash
$ go test -check.f dialSuite.TestDialWithCredentialsStoredInVault`
$ go test -check.f MyTestSuite
$ go test -check.f "Test.*Works"
$ go test -check.f "MyTestSuite.Test.*Works"
```

For more verbose output, add `check.v` and `check.vv`.

**Note:** The `check.f` command only applies to Go Check tests, any package with both Go Check tests
and normal `testing.T` tests will result in both sets of tests running. To avoid this look for where 
Go Check registers its test suite into the Go test runner, normally in a file called `package_test.go`
and only run that test function.  
E.g. in `internal/jujuapi` an example command to only run a single suite test would be:
```
$ go test ./internal/jujuapi -check.f modelManagerSuite.TestListModelSummaries -run TestPackage ./internal/jujuapi
```

## Building/Publishing
Below are instructions on building the various binaries that are part of the project as well as
some information on how they are published.

### jimmsrv
To build the JIMM server run `go build ./cmd/jimmsrv`

The JIMM server is published as an OCI image using 
[Rockcraft](https://documentation.ubuntu.com/rockcraft/en/latest/) 
(a tool to create OCI images based on Ubuntu).

Run `make rock` to pack the rock. The images are published to the Github repo's container registry
for later use by the JIMM-k8s charm.

The JIMM server is also available as a snap and can be built with `make jimm-snap`. This snap is 
not published to the snap store as it is intended to be used as part of a machine charm deployment.

### jimmctl
To build jimmctl run `go build ./cmd/jimmctl`

The jimmctl tool is published as a [Snap](https://snapcraft.io/jimmctl).

Run `make jimmctl-snap` to build the snap. The snaps are published to the Snap Store 
from where they can be conveniently installed.

### jaas plugin
To build the jaas plugin run `go build ./cmd/jaas`

The jaas plugin is published as a [Snap](https://snapcraft.io/jaas).

Run `make jaas-snap` to build the snap. The snaps are published to the Snap Store 
from where they can be conveniently installed.
