// Copyright 2016 Canonical Ltd.

package jimmdb_test

import (
	"context"
	"strings"

	"github.com/google/go-cmp/cmp/cmpopts"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
)

type credentialSuite struct {
	jemtest.IsolatedMgoSuite
	database *jimmdb.Database
}

var _ = gc.Suite(&credentialSuite{})

func (s *credentialSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jimmdb.NewDatabase(context.TODO(), pool, "jem")
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
	pool.Close()
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *credentialSuite) TearDownTest(c *gc.C) {
	s.database.Session.Close()
	s.database = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *credentialSuite) checkDBOK(c *gc.C) {
	c.Check(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *credentialSuite) TestUpsertCredential(c *gc.C) {
	cPath := s.path(c, "dummy/bob/test")
	cred := mongodoc.Credential{
		Id:   "ignored",
		Path: cPath,
	}
	err := s.database.UpsertCredential(testContext, &cred)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cred, jc.DeepEquals, mongodoc.Credential{
		Id:   "dummy/bob/test",
		Path: cPath,
	})

	cred1 := mongodoc.Credential{Path: cPath}
	err = s.database.GetCredential(testContext, &cred1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cred1, jemtest.CmpEquals(cmpopts.EquateEmpty()), cred)

	cred2 := mongodoc.Credential{
		Path:    cPath,
		Revoked: true,
	}
	err = s.database.UpsertCredential(testContext, &cred2)
	c.Assert(err, gc.Equals, nil)

	cred3 := mongodoc.Credential{Path: cPath}
	err = s.database.GetCredential(testContext, &cred3)
	c.Assert(err, gc.Equals, nil)
	c.Assert(cred3, jemtest.CmpEquals(cmpopts.EquateEmpty()), cred2)

	s.checkDBOK(c)
}

func (s *credentialSuite) TestForEachCredential(c *gc.C) {
	err := s.database.UpsertCredential(testContext, &mongodoc.Credential{
		Path:  s.path(c, "dummy/bob/c1"),
		Type:  "empty",
		Label: "l1",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.UpsertCredential(testContext, &mongodoc.Credential{
		Path:  s.path(c, "dummy/bob/c2"),
		Type:  "userpass",
		Label: "l2",
	})
	c.Assert(err, gc.Equals, nil)
	err = s.database.UpsertCredential(testContext, &mongodoc.Credential{
		Path:  s.path(c, "dummy/bob/c3"),
		Type:  "empty",
		Label: "l3",
	})
	c.Assert(err, gc.Equals, nil)

	paths := []mongodoc.CredentialPath{
		s.path(c, "dummy/bob/c3"),
		s.path(c, "dummy/bob/c1"),
		s.path(c, "dummy/bob/c2"),
	}
	f := func(cred *mongodoc.Credential) error {
		if len(paths) == 0 || cred.Path != paths[0] {
			return errgo.Newf("unexpected credential, %s", cred.Path)
		}
		paths = paths[1:]
		return nil
	}

	err = s.database.ForEachCredential(testContext, jimmdb.Eq("type", "empty"), []string{"-label"}, f)
	c.Assert(err, gc.Equals, nil)
	err = s.database.ForEachCredential(testContext, jimmdb.Eq("type", "userpass"), []string{"-label"}, f)
	c.Assert(err, gc.Equals, nil)
	c.Assert(paths, gc.HasLen, 0)

	s.checkDBOK(c)
}

func (s *credentialSuite) TestForEachCredentialReturnsError(c *gc.C) {
	err := s.database.UpsertCredential(testContext, &mongodoc.Credential{
		Path: s.path(c, "dummy/bob/c1"),
	})
	c.Assert(err, gc.Equals, nil)

	testError := errgo.New("test")

	f := func(m *mongodoc.Credential) error {
		return testError
	}

	err = s.database.ForEachCredential(testContext, jimmdb.Eq("path.cloud", "dummy"), nil, f)
	c.Assert(errgo.Cause(err), gc.Equals, testError)

	s.checkDBOK(c)
}

func (s *credentialSuite) path(c *gc.C, t string) mongodoc.CredentialPath {
	parts := strings.Split(t, "/")
	c.Assert(parts, gc.HasLen, 3)
	return mongodoc.CredentialPath{
		Cloud: parts[0],
		EntityPath: mongodoc.EntityPath{
			User: parts[1],
			Name: parts[2],
		},
	}
}
