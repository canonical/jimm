// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	jujufeature "github.com/juju/juju/feature"
	corejujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/loggo"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem"
	"github.com/CanonicalLtd/jem/cmd/juju-jem/jemcmd"
	"github.com/CanonicalLtd/jem/internal/idmtest"
	"github.com/CanonicalLtd/jem/jemclient"
)

// run runs a jem plugin subcommand with the given arguments,
// its context directory set to dir. It returns the output of the command
// and its exit code.
func run(c *gc.C, dir string, cmdName string, args ...string) (stdout, stderr string, exitCode int) {
	c.Logf("run %q %q", cmdName, args)
	// Remove the warning writer usually registered by cmd.Log.Start, so that
	// it is possible to run multiple commands in the same test.
	// We are not interested in possible errors here.
	defer loggo.RemoveWriter("warning")
	var stdoutBuf, stderrBuf bytes.Buffer
	ctxt := &cmd.Context{
		Dir:    dir,
		Stdin:  strings.NewReader(""),
		Stdout: &stdoutBuf,
		Stderr: &stderrBuf,
	}
	allArgs := append([]string{cmdName}, args...)
	exitCode = cmd.Main(jemcmd.New(), ctxt, allArgs)
	return stdoutBuf.String(), stderrBuf.String(), exitCode
}

type commonSuite struct {
	corejujutesting.JujuConnSuite

	jemSrv  jem.HandleCloser
	idmSrv  *idmtest.Server
	httpSrv *httptest.Server

	cookieFile string
}

func (s *commonSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.cookieFile = filepath.Join(c.MkDir(), "cookies")
	s.PatchEnvironment("JUJU_COOKIEFILE", s.cookieFile)
	s.PatchEnvironment("JUJU_LOGGING_CONFIG", "<root>=DEBUG")

	s.idmSrv = idmtest.NewServer()
	s.jemSrv = s.newServer(c, s.Session, s.idmSrv)
	s.httpSrv = httptest.NewServer(s.jemSrv)

	// Set up the client to act as "testuser" by default.
	s.idmSrv.SetDefaultUser("testuser")

	os.Setenv("JUJU_DEV_FEATURE_FLAGS", jujufeature.JES)
	featureflag.SetFlagsFromEnvironment("JUJU_DEV_FEATURE_FLAGS")

	os.Setenv("JUJU_JEM", s.httpSrv.URL)

}

// jemClient returns a new JEM client that will act as the given user.
func (s *commonSuite) jemClient(username string) *jemclient.Client {
	return jemclient.New(jemclient.NewParams{
		BaseURL: s.httpSrv.URL,
		Client:  s.idmSrv.Client(username),
	})
}

func (s *commonSuite) TearDownTest(c *gc.C) {
	s.idmSrv.Close()
	s.jemSrv.Close()
	s.httpSrv.Close()
	s.JujuConnSuite.TearDownTest(c)
}

const adminUser = "admin"

func (s *commonSuite) newServer(c *gc.C, session *mgo.Session, idmSrv *idmtest.Server) jem.HandleCloser {
	db := session.DB("jem")
	config := jem.ServerParams{
		DB:               db,
		StateServerAdmin: adminUser,
		IdentityLocation: idmSrv.URL.String(),
		PublicKeyLocator: idmSrv,
	}
	srv, err := jem.NewServer(config)
	c.Assert(err, gc.IsNil)
	return srv
}

func (s *commonSuite) clearCookies(c *gc.C) {
	err := os.Remove(s.cookieFile)
	c.Assert(err, gc.IsNil)
}

const fakeSSHKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCcEHVJtQyjN0eaNMAQIwhwknKj+8uZCqmzeA6EfnUEsrOHisoKjRVzb3bIRVgbK3SJ2/1yHPpL2WYynt3LtToKgp0Xo7LCsspL2cmUIWNYCbcgNOsT5rFeDsIDr9yQito8A3y31Mf7Ka7Rc0EHtCW4zC5yl/WZjgmMmw930+V1rDa5GjkqivftHE5AvLyRGvZJPOLH8IoO+sl02NjZ7dRhniBO9O5UIwxSkuGA5wvfLV7dyT/LH56gex7C2fkeBkZ7YGqTdssTX6DvFTHjEbBAsdWd8/rqXWtB6Xdi8sb3+aMpg9DRomZfb69Y+JuqWTUaq+q30qG2CTiqFRbgwRpp bob@somewhere"
