// Copyright 2018 Canonical Ltd.

package admin_test

import (
	"context"
	"io"
	"net/http"

	"github.com/juju/aclstore/aclclient"
	"github.com/juju/aclstore/params"
	"github.com/juju/httprequest"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	errgo "gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/apitest"
)

type APISuite struct {
	apitest.Suite
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) TestGetACL(c *gc.C) {
	acls, err := s.client("controller-admin").GetACL(context.Background(), &params.GetACLRequest{
		Name: "admin",
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acls, jc.DeepEquals, &params.GetACLResponse{Users: []string{"controller-admin"}})
}

func (s *APISuite) TestUnauthorized(c *gc.C) {
	acls, err := s.client("bob").GetACL(context.Background(), &params.GetACLRequest{
		Name: "admin",
	})
	c.Assert(err, gc.ErrorMatches, `Get http.*/admin/acls/admin: forbidden`)
	c.Assert(acls, gc.IsNil)
}

func (s *APISuite) TestSetACL(c *gc.C) {
	client := s.client("controller-admin")
	err := client.SetACL(context.Background(), &params.SetACLRequest{
		Name: "admin",
		Body: params.SetACLRequestBody{
			Users: []string{"controller-admin", "bob"},
		},
	})
	c.Assert(err, gc.Equals, nil)
	acls, err := client.GetACL(context.Background(), &params.GetACLRequest{
		Name: "admin",
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acls, jc.DeepEquals, &params.GetACLResponse{Users: []string{"bob", "controller-admin"}})
}

func (s *APISuite) TestModifyACL(c *gc.C) {
	client := s.client("controller-admin")
	err := client.ModifyACL(context.Background(), &params.ModifyACLRequest{
		Name: "admin",
		Body: params.ModifyACLRequestBody{
			Add: []string{"alice"},
		},
	})
	c.Assert(err, gc.Equals, nil)
	acls, err := client.GetACL(context.Background(), &params.GetACLRequest{
		Name: "admin",
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(acls, jc.DeepEquals, &params.GetACLResponse{Users: []string{"alice", "controller-admin"}})
}

func (s *APISuite) client(user string) *aclclient.Client {
	return aclclient.New(aclclient.NewParams{
		BaseURL: s.HTTPSrv.URL + "/admin/acls",
		Doer:    doadaptor{s.IDMSrv.Client(user)},
	})
}

type doer interface {
	httprequest.Doer
	httprequest.DoerWithBody
}

type doadaptor struct {
	doer
}

func (d doadaptor) Do(req *http.Request) (*http.Response, error) {
	if req.Body == nil {
		return d.doer.Do(req)
	}
	body, ok := req.Body.(io.ReadSeeker)
	if !ok {
		return nil, errgo.Newf("invalid body")
	}
	req.Body = nil
	return d.doer.DoWithBody(req, body)
}
