// Copyright 2018 Canonical Ltd.

package admin_test

import (
	"context"
	"io"
	"net/http"

	"github.com/juju/aclstore/aclclient"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	errgo "gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"

	"github.com/CanonicalLtd/jem/internal/apitest"
)

type APISuite struct {
	apitest.Suite
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) TestGetACL(c *gc.C) {
	users, err := s.client("controller-admin").Get(context.Background(), "admin")
	c.Assert(err, gc.Equals, nil)
	c.Assert(users, jc.DeepEquals, []string{"controller-admin"})
}

func (s *APISuite) TestUnauthorized(c *gc.C) {
	users, err := s.client("bob").Get(context.Background(), "admin")
	c.Assert(err, gc.ErrorMatches, `Get http.*/admin/acls/admin: forbidden`)
	c.Assert(users, gc.IsNil)
}

func (s *APISuite) TestSetACL(c *gc.C) {
	client := s.client("controller-admin")
	err := client.Set(context.Background(), "admin", []string{"controller-admin", "bob"})
	c.Assert(err, gc.Equals, nil)
	users, err := client.Get(context.Background(), "admin")
	c.Assert(err, gc.Equals, nil)
	c.Assert(users, jc.DeepEquals, []string{"bob", "controller-admin"})
}

func (s *APISuite) TestModifyACL(c *gc.C) {
	client := s.client("controller-admin")
	err := client.Add(context.Background(), "admin", []string{"alice"})
	c.Assert(err, gc.Equals, nil)
	users, err := client.Get(context.Background(), "admin")
	c.Assert(err, gc.Equals, nil)
	c.Assert(users, jc.DeepEquals, []string{"alice", "controller-admin"})
}

func (s *APISuite) client(user string) *aclclient.Client {
	return aclclient.New(aclclient.NewParams{
		BaseURL: s.HTTPSrv.URL + "/admin/acls",
		Doer:    bakeryDoer{s.IDMSrv.Client(user)},
	})
}

type bakeryDoer struct {
	client *httpbakery.Client
}

func (d bakeryDoer) Do(req *http.Request) (*http.Response, error) {
	if req.Body == nil {
		return d.client.Do(req)
	}
	body, ok := req.Body.(io.ReadSeeker)
	if !ok {
		return nil, errgo.Newf("unsupported body type")
	}
	req1 := *req
	req1.Body = nil
	return d.client.DoWithBody(&req1, body)
}
