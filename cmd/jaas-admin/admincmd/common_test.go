// Copyright 2015-2016 Canonical Ltd.

package admincmd_test

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/aclstore/aclclient"
	"github.com/juju/cmd"
	"github.com/juju/idmclient/idmtest"
	"github.com/juju/loggo"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	jem "github.com/CanonicalLtd/jimm"
	"github.com/CanonicalLtd/jimm/cmd/jaas-admin/admincmd"
	"github.com/CanonicalLtd/jimm/internal/bakeryadaptor"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/jemclient"
	"github.com/CanonicalLtd/jimm/params"
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
	exitCode = cmd.Main(admincmd.New(), ctxt, allArgs)
	return stdoutBuf.String(), stderrBuf.String(), exitCode
}

type commonSuite struct {
	jemtest.JujuConnSuite

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

	os.Setenv("JIMM_URL", s.httpSrv.URL)
}

// jemClient returns a new JEM client that will act as the given user.
func (s *commonSuite) jemClient(username string) *jemclient.Client {
	return jemclient.New(jemclient.NewParams{
		BaseURL: s.httpSrv.URL,
		Client:  s.idmSrv.Client(username),
	})
}

// aclClient returns a new aclclient.Client that will act as the given user.
func (s *commonSuite) aclClient(username string) *aclclient.Client {
	return aclclient.New(aclclient.NewParams{
		BaseURL: s.httpSrv.URL + "/admin/acls",
		Doer:    bakeryadaptor.Doer{s.idmSrv.Client(username)},
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
		ControllerAdmin:  adminUser,
		IdentityLocation: idmSrv.URL.String(),
		PublicKeyLocator: idmSrv,
	}
	srv, err := jem.NewServer(context.TODO(), config)
	c.Assert(err, gc.Equals, nil)
	return srv
}

const sshKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDOjaOjVRHchF2RFCKQdgBqrIA5nOoqSprLK47l2th5I675jw+QYMIihXQaITss3hjrh3+5ITyBO41PS5rHLNGtlYUHX78p9CHNZsJqHl/z1Ub1tuMe+/5SY2MkDYzgfPtQtVsLasAIiht/5g78AMMXH3HeCKb9V9cP6/lPPq6mCMvg8TDLrPp/P2vlyukAsJYUvVgoaPDUBpedHbkMj07pDJqe4D7c0yEJ8hQo/6nS+3bh9Q1NvmVNsB1pbtk3RKONIiTAXYcjclmOljxxJnl1O50F5sOIi38vyl7Q63f6a3bXMvJEf1lnPNJKAxspIfEu8gRasny3FEsbHfrxEwVj rog@rog-x220"

var dummyEnvConfig = map[string]interface{}{
	"authorized-keys": sshKey,
	"controller":      true,
}

func (s *commonSuite) addModel(c *gc.C, pathStr, srvPathStr, credName string) {
	var path, srvPath params.EntityPath
	err := path.UnmarshalText([]byte(pathStr))
	c.Assert(err, gc.Equals, nil)
	err = srvPath.UnmarshalText([]byte(srvPathStr))
	c.Assert(err, gc.Equals, nil)

	credPath := params.CredentialPath{
		Cloud: "dummy",
		User:  path.User,
		Name:  params.CredentialName(credName),
	}
	err = s.jemClient(string(path.User)).UpdateCredential(&params.UpdateCredential{
		CredentialPath: credPath,
		Credential: params.Credential{
			AuthType: "empty",
		},
	})
	c.Assert(err, gc.Equals, nil)

	_, err = s.jemClient(string(path.User)).NewModel(&params.NewModel{
		User: path.User,
		Info: params.NewModelInfo{
			Name:       path.Name,
			Controller: &srvPath,
			Credential: credPath,
			Config:     dummyEnvConfig,
		},
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *commonSuite) clearCookies(c *gc.C) {
	err := os.Remove(s.cookieFile)
	c.Assert(err, gc.Equals, nil)
}
