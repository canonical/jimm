// Copyright 2025 Canonical.

package cmd

import (
	"bytes"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"go.uber.org/mock/gomock"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func TestRegisterControllerRun_DryRun_Defaults(t *testing.T) {
	c := qt.New(t)

	store := jujuclient.NewMemStore()
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

	inner := &registerControllerCommand{}
	inner.SetClientStore(store)
	command := modelcmd.WrapBase(inner)
	initCommand(c, command, "controller-1", "--dry-run")

	ctxt := newTestContext(c)
	err := command.Run(ctxt)
	c.Assert(err, qt.IsNil)

	c.Assert(ctxt.Stdout.(*bytes.Buffer).String(), qt.Matches, `uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
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

func TestRegisterControllerRun_DryRun_Local(t *testing.T) {
	c := qt.New(t)

	store := jujuclient.NewMemStore()
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

	inner := &registerControllerCommand{}
	inner.SetClientStore(store)
	command := modelcmd.WrapBase(inner)
	initCommand(c, command, "controller-1", "--dry-run", "--local")

	ctxt := newTestContext(c)
	err := command.Run(ctxt)
	c.Assert(err, qt.IsNil)

	c.Assert(ctxt.Stdout.(*bytes.Buffer).String(), qt.Matches, `uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
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

func TestRegisterControllerRun_DryRun_TLSHostname(t *testing.T) {
	c := qt.New(t)

	store := jujuclient.NewMemStore()
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

	inner := &registerControllerCommand{}
	inner.SetClientStore(store)
	command := modelcmd.WrapBase(inner)
	initCommand(c, command, "controller-1", "--dry-run", "--tls-hostname", "foo")

	ctxt := newTestContext(c)
	err := command.Run(ctxt)
	c.Assert(err, qt.IsNil)

	c.Assert(ctxt.Stdout.(*bytes.Buffer).String(), qt.Matches, `uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
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

func TestRegisterControllerRun_DryRun_CustomPublicAddress(t *testing.T) {
	c := qt.New(t)

	store := jujuclient.NewMemStore()
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

	inner := &registerControllerCommand{}
	inner.SetClientStore(store)
	command := modelcmd.WrapBase(inner)
	initCommand(c, command, "controller-1", "--dry-run", "--public-address", "my-address.com:1234")

	ctxt := newTestContext(c)
	err := command.Run(ctxt)
	c.Assert(err, qt.IsNil)

	c.Assert(ctxt.Stdout.(*bytes.Buffer).String(), qt.Matches, `uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
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

func TestRegisterControllerRun_Success_FromFile(t *testing.T) {
	c := qt.New(t)

	cmdMocks := setupCmdMocks(c)

	inner := &registerControllerCommand{}
	inner.SetClientStore(cmdMocks.store)
	inner.jimmAPIFunc = func() (JIMMAPI, error) { return cmdMocks.client, nil }
	command := modelcmd.WrapBase(inner)

	payload := `uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
name: controller-1
publicaddress: ""
apiaddresses:
- localhost:17070
cacertificate: |
  -----BEGIN CERTIFICATE-----
  abc
  -----END CERTIFICATE-----
username: admin
password: secret
`

	initCommand(c, command, "controller-1", "--file", "-")

	ctxt := newTestContext(c)
	ctxt.Stdin = bytes.NewBufferString(payload)

	cmdMocks.client.EXPECT().
		AddController(gomock.Any()).
		DoAndReturn(func(req *apiparams.AddControllerRequest) (apiparams.ControllerInfo, error) {
			c.Assert(req.Name, qt.Equals, "controller-1")
			c.Assert(req.UUID, qt.Equals, "deadbeef-1bad-500d-9000-4b1d0d06f00d")
			return apiparams.ControllerInfo{Name: req.Name, UUID: req.UUID}, nil
		}).
		Times(1)
	cmdMocks.client.EXPECT().Close().Times(1)

	err := command.Run(ctxt)
	c.Assert(err, qt.IsNil)
	c.Assert(ctxt.Stdout.(*bytes.Buffer).String(), qt.Matches, `(?s).*name: controller-1\s+uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d.*`)
}
