// Copyright 2025 Canonical.

package cmd_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/jujuclient"
	jjclient "github.com/juju/juju/jujuclient"
	gc "gopkg.in/check.v1"
	"sigs.k8s.io/yaml"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

type registerControllerDryRunSuite struct {
}

var _ = gc.Suite(&registerControllerDryRunSuite{})

func (s *registerControllerDryRunSuite) TestRegisterControllerDryRun(c *gc.C) {
	store := jjclient.NewMemStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: "982b16d9-a945-4762-b684-fd4fd885aa11",
		APIEndpoints:   []string{"127.0.0.1:17070"},
		PublicDNSName:  "controller1.example.com",
		CACert:         `foo`,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     "test-user",
		Password: "super-secret-password",
	}

	ctx, err := cmdtesting.RunCommand(c, cmd.NewRegisterControllerCommandForTesting(store, nil), "controller-1", "--dry-run")
	c.Assert(err, gc.IsNil)

	data := cmdtesting.Stdout(ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Matches, `uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
name: controller-1
publicaddress: controller1.example.com
tlshostname: ""
apiaddresses:
- 127.0.0.1:17070
cacertificate: ""
username: test-user
password: super-secret-password
`)
}

func (s *registerControllerDryRunSuite) TestControllerInfoWithLocalFlag(c *gc.C) {
	store := jjclient.NewMemStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: "982b16d9-a945-4762-b684-fd4fd885aa11",
		APIEndpoints:   []string{"127.0.0.1:17070"},
		PublicDNSName:  "controller1.example.com",
		CACert:         `foo`,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     "test-user",
		Password: "super-secret-password",
	}

	ctx, err := cmdtesting.RunCommand(c, cmd.NewRegisterControllerCommandForTesting(store, nil), "controller-1", "--dry-run", "--local")
	c.Assert(err, gc.IsNil)

	data := cmdtesting.Stdout(ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Matches, `uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
name: controller-1
publicaddress: ""
tlshostname: ""
apiaddresses:
- 127.0.0.1:17070
cacertificate: foo
username: test-user
password: super-secret-password
`)
}

func (s *registerControllerDryRunSuite) TestControllerInfoWithTlsFlag(c *gc.C) {
	store := jjclient.NewMemStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: "982b16d9-a945-4762-b684-fd4fd885aa11",
		APIEndpoints:   []string{"127.0.0.1:17070"},
		PublicDNSName:  "controller1.example.com",
		CACert:         `foo`,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     "test-user",
		Password: "super-secret-password",
	}

	ctx, err := cmdtesting.RunCommand(c, cmd.NewRegisterControllerCommandForTesting(store, nil), "controller-1", "--dry-run", "--tls-hostname", "foo")
	c.Assert(err, gc.IsNil)

	data := cmdtesting.Stdout(ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Matches, `uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
name: controller-1
publicaddress: controller1.example.com
tlshostname: foo
apiaddresses:
- 127.0.0.1:17070
cacertificate: ""
username: test-user
password: super-secret-password
`)
}

func (s *registerControllerDryRunSuite) TestControllerInfoWithCustomPublicAddress(c *gc.C) {
	store := jjclient.NewMemStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: "982b16d9-a945-4762-b684-fd4fd885aa11",
		APIEndpoints:   []string{"127.0.0.1:17070"},
		PublicDNSName:  "controller1.example.com",
		CACert:         `foo`,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     "test-user",
		Password: "super-secret-password",
	}

	ctx, err := cmdtesting.RunCommand(c, cmd.NewRegisterControllerCommandForTesting(store, nil), "controller-1", "--dry-run", "--public-address", "my-address.com:1234")
	c.Assert(err, gc.IsNil)

	data := cmdtesting.Stdout(ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Matches, `uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
name: controller-1
publicaddress: my-address.com:1234
tlshostname: ""
apiaddresses:
- 127.0.0.1:17070
cacertificate: ""
username: test-user
password: super-secret-password
`)
}

func (s *registerControllerDryRunSuite) TestControllerInfoWithLocalFlagAndCustomPublicAddress(c *gc.C) {
	store := jjclient.NewMemStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: "982b16d9-a945-4762-b684-fd4fd885aa11",
		APIEndpoints:   []string{"127.0.0.1:17070"},
		PublicDNSName:  "controller1.example.com",
		CACert:         `foo`,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     "test-user",
		Password: "super-secret-password",
	}

	ctx, err := cmdtesting.RunCommand(c, cmd.NewRegisterControllerCommandForTesting(store, nil), "controller-1", "--dry-run", "--local", "--public-address", "my-address.com:1234")
	c.Assert(err, gc.IsNil)

	data := cmdtesting.Stdout(ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Matches, `uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
name: controller-1
publicaddress: my-address.com:1234
tlshostname: ""
apiaddresses:
- 127.0.0.1:17070
cacertificate: foo
username: test-user
password: super-secret-password
`)
}

type registerControllerSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&registerControllerSuite{})

func (s *registerControllerSuite) TestRegisterControllerSuperuserByFile(c *gc.C) {
	info := s.APIInfo(c)
	params := apiparams.AddControllerRequest{
		UUID:          info.ControllerUUID,
		Name:          "controller-1",
		CACertificate: info.CACert,
		APIAddresses:  info.Addrs,
		Username:      info.Tag.Id(),
		Password:      info.Password,
	}
	tmpdir, tmpfile := writeYAMLTempFile(c, params)
	defer os.RemoveAll(tmpdir)

	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")
	ctx, err := cmdtesting.RunCommand(c, cmd.NewRegisterControllerCommandForTesting(s.ClientStore(), bClient), "controller-1", "--file", tmpfile)
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Matches, `(?s)name: controller-1
uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
publicaddress: ""
apiaddresses:
- localhost:.*
cacertificate: \|
  -----BEGIN CERTIFICATE-----
  .*
  -----END CERTIFICATE-----
cloudtag: cloud-`+jimmtest.TestCloudName+`
cloudregion: `+jimmtest.TestCloudRegionName+`
agentversion: .*
status:
  status: available
  info: ""
  data: .*
  since: null
`)

	username, password, err := s.JIMM.CredentialStore.GetControllerCredentials(context.Background(), "controller-1")
	c.Assert(err, gc.IsNil)
	c.Assert(username, gc.Equals, info.Tag.Id())
	c.Assert(password, gc.Equals, info.Password)
}

func (s *registerControllerSuite) TestRegisterControllerSuperuserByClientStore(c *gc.C) {
	info := s.APIInfo(c)
	// alice is superuser
	bClient := s.SetupCLIAccess(c, "alice")

	store := s.ClientStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: info.ControllerUUID,
		APIEndpoints:   info.Addrs,
		CACert:         info.CACert,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     info.Tag.Id(),
		Password: info.Password,
	}
	ctx, err := cmdtesting.RunCommand(c, cmd.NewRegisterControllerCommandForTesting(store, bClient), "controller-1", "--local")
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Matches, `(?s)name: controller-1
uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
publicaddress: ""
apiaddresses:
- localhost:.*
cacertificate: \|
  -----BEGIN CERTIFICATE-----
  .*
  -----END CERTIFICATE-----
cloudtag: cloud-`+jimmtest.TestCloudName+`
cloudregion: `+jimmtest.TestCloudRegionName+`
agentversion: .*
status:
  status: available
  info: ""
  data: .*
  since: null
`)

	username, password, err := s.JIMM.CredentialStore.GetControllerCredentials(context.Background(), "controller-1")
	c.Assert(err, gc.IsNil)
	c.Assert(username, gc.Equals, info.Tag.Id())
	c.Assert(password, gc.Equals, info.Password)
}

func (s *registerControllerSuite) TestRegisterControllerNotAuthorised(c *gc.C) {
	info := s.APIInfo(c)
	store := s.ClientStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: info.ControllerUUID,
		APIEndpoints:   info.Addrs,
		CACert:         info.CACert,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     info.Tag.Id(),
		Password: info.Password,
	}
	// bob is not superuser
	bClient := s.SetupCLIAccess(c, "bob")
	_, err := cmdtesting.RunCommand(c, cmd.NewRegisterControllerCommandForTesting(store, bClient), "controller-1")
	c.Assert(err, gc.ErrorMatches, `failed to add controller: unauthorized \(unauthorized access\)`)
}

func writeYAMLTempFile(c *gc.C, payload interface{}) (string, string) {
	data, err := yaml.Marshal(payload)
	c.Assert(err, gc.Equals, nil)

	dir, err := os.MkdirTemp("", "add-controller-test")
	c.Assert(err, gc.Equals, nil)

	tmpfn := filepath.Join(dir, "tmp.yaml")
	err = os.WriteFile(tmpfn, data, 0600)
	c.Assert(err, gc.Equals, nil)
	return dir, tmpfn
}
