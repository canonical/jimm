// Copyright 2015 Canonical Ltd.

package modelcmd_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/yaml.v1"

	"github.com/CanonicalLtd/jem/params"
)

type createSuite struct {
	commonSuite
}

var _ = gc.Suite(&createSuite{})

var createErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "too few arguments",
	args:         []string{},
	expectStderr: "missing model name argument",
	expectCode:   2,
}, {
	about:        "only one part in model id",
	args:         []string{"a"},
	expectStderr: `invalid entity path "a": need <user>/<name>`,
	expectCode:   2,
}, {
	about:        "controller cannot be specified with location",
	args:         []string{"foo/bar", "-c", "xx/yy", "foo=bar"},
	expectStderr: `cannot specify explicit controller name with location`,
	expectCode:   2,
}, {
	about:        "invalid location key-value",
	args:         []string{"foo/bar", "foobar"},
	expectStderr: `expected "key=value", got "foobar"`,
	expectCode:   2,
}}

func (s *createSuite) TestCreateError(c *gc.C) {
	for i, test := range createErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "create", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}

func (s *createSuite) TestCreate(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add the controller that we're going to use
	// to create the new model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	err := s.jemClient("bob").UpdateCredential(&params.UpdateCredential{
		EntityPath: params.EntityPath{"bob", "cred"},
		Cloud:      "dummy",
		Credential: params.Credential{
			AuthType: "empty",
		},
	})
	c.Assert(err, gc.IsNil)

	configPath := writeConfig(c, map[string]interface{}{
		"authorized-keys": fakeSSHKey,
		"controller":      true,
	})
	stdout, stderr, code = run(c, c.MkDir(),
		"create",
		"-c", "bob/foo",
		"--credential", "cred",
		"--config", configPath,
		"bob/newmodel",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "jem-foo:bob@external/newmodel\n")

	// Check that we can attach to the new model
	// through the usual juju connection mechanism.
	store := jujuclient.NewFileClientStore()
	params, err := newAPIConnectionParams(
		store, "jem-foo", "bob@external/newmodel", httpbakery.NewClient(),
	)
	c.Assert(err, gc.IsNil)
	client, err := juju.NewAPIConnection(params)
	c.Assert(err, jc.ErrorIsNil)
	client.Close()
}

func (s *createSuite) TestCreateWithLocation(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")
	s.idmSrv.AddUser("bob", "admin")

	// First add the controller that we're going to use
	// to create the new model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "--public", "bob/aws")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "add-controller", "bob/azure")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	err := s.jemClient("bob").UpdateCredential(&params.UpdateCredential{
		EntityPath: params.EntityPath{"bob", "cred"},
		Cloud:      "dummy",
		Credential: params.Credential{
			AuthType: "empty",
		},
	})
	c.Assert(err, gc.IsNil)

	configPath := writeConfig(c, map[string]interface{}{
		"authorized-keys": fakeSSHKey,
		"controller":      true,
	})
	stdout, stderr, code = run(c, c.MkDir(),
		"create",
		"--config", configPath,
		"--credential", "cred",
		"bob/newmodel",
		"cloud=dummy",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "jem-aws:bob@external/newmodel\n")

	client := s.jemClient("bob")
	m, err := client.GetModel(&params.GetModel{
		EntityPath: params.EntityPath{"bob", "newmodel"},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(m.ControllerPath.String(), gc.Equals, "bob/aws")
}

func (s *createSuite) TestCreateWithLocationWithExistingModel(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")
	s.idmSrv.AddUser("bob", "admin")

	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "--public", "bob/aws")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	err := s.jemClient("bob").UpdateCredential(&params.UpdateCredential{
		EntityPath: params.EntityPath{"bob", "cred"},
		Cloud:      "dummy",
		Credential: params.Credential{
			AuthType: "empty",
		},
	})
	c.Assert(err, gc.IsNil)

	configPath := writeConfig(c, map[string]interface{}{
		"authorized-keys": fakeSSHKey,
		"controller":      true,
	})
	stdout, stderr, code = run(c, c.MkDir(),
		"create",
		"--config", configPath,
		"--credential", "cred",
		"bob/newmodel",
		"cloud=dummy",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "jem-aws:bob@external/newmodel\n")

	// Create a second model with the same local name.
	// This should be rejected even though we haven't
	// specified a controller, because all jem controllers should
	// be searched.
	stdout, stderr, code = run(c, c.MkDir(),
		"create",
		"--config", configPath,
		"--credential", "cred",
		"--local", "bob@external/newmodel",
		"bob/anothermodel",
		"cloud=dummy",
	)
	c.Assert(code, gc.Equals, 1, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `ERROR local model "bob@external/newmodel" already exists in controller "jem-aws"\n`)
}

func (s *createSuite) TestCreateWithLocationNoMatch(c *gc.C) {
	configPath := writeConfig(c, map[string]interface{}{
		"authorized-keys": fakeSSHKey,
		"controller":      true,
	})
	stdout, stderr, code := run(c, c.MkDir(),
		"create",
		"--config", configPath,
		"bob/newmodel",
	)
	c.Assert(code, gc.Equals, 1, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `ERROR cannot get schema info: GET http://.*: no matching controllers\n`)
}

func writeConfig(c *gc.C, config map[string]interface{}) string {
	data, err := yaml.Marshal(config)
	c.Assert(err, gc.IsNil)
	configPath := filepath.Join(c.MkDir(), "config.yaml")
	err = ioutil.WriteFile(configPath, data, 0666)
	c.Assert(err, gc.IsNil)
	return configPath
}
