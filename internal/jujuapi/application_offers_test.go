// Copyright 2020 Canonical Ltd.

package jujuapi_test

import (
	"github.com/juju/juju/api/applicationoffers"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
)

type applicationOffersSuite struct {
	websocketSuite
}

var _ = gc.Suite(&applicationOffersSuite{})

func (s *applicationOffersSuite) SetUpTest(c *gc.C) {
	s.ServerParams.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.ServerParams.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.websocketSuite.SetUpTest(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *applicationOffersSuite) TestOffer(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	modelUUID := utils.MustNewUUID().String()

	results, err := client.Offer(modelUUID, "test application", []string{}, "test offer", "test offer description")
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))
}
