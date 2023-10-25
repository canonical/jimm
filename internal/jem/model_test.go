// Copyright 2015 Canonical Ltd.

package jem_test

import (
	jujuparams "github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/jem/jimmdb"
	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/params"
)

type modelSuite struct {
	jemtest.BootstrapSuite
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) TestGetModel(c *gc.C) {
	model1 := mongodoc.Model{Path: s.Model.Path}
	err := s.JEM.GetModel(testContext, jemtest.Bob, jujuparams.ModelReadAccess, &model1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(model1, jc.DeepEquals, s.Model)

	u := new(jimmdb.Update).Unset("cloud").Unset("cloudregion").Unset("credential").Unset("defaultseries")
	u.Unset("providertype").Unset("controlleruuid")
	err = s.JEM.DB.UpdateModel(testContext, &s.Model, u, true)
	c.Assert(err, gc.Equals, nil)

	model2 := mongodoc.Model{UUID: s.Model.UUID}
	err = s.JEM.GetModel(testContext, jemtest.Bob, jujuparams.ModelReadAccess, &model2)
	c.Assert(err, gc.Equals, nil)

	c.Assert(model2, gc.DeepEquals, model1)
}

func (s *modelSuite) TestGetModelUnauthorized(c *gc.C) {
	model1 := mongodoc.Model{Path: s.Model.Path}
	err := s.JEM.GetModel(testContext, jemtest.Charlie, jujuparams.ModelReadAccess, &model1)
	c.Assert(err, gc.ErrorMatches, "unauthorized")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)
}

func (s *modelSuite) TestForEachModel(c *gc.C) {
	model2 := mongodoc.Model{Path: params.EntityPath{User: "bob", Name: "model-2"}}
	s.CreateModel(c, &model2, nil, map[params.User]jujuparams.UserAccessPermission{"charlie": jujuparams.ModelAdminAccess})
	model3 := mongodoc.Model{Path: params.EntityPath{User: "bob", Name: "model-3"}}
	s.CreateModel(c, &model3, nil, map[params.User]jujuparams.UserAccessPermission{"charlie": jujuparams.ModelWriteAccess})
	model4 := mongodoc.Model{Path: params.EntityPath{User: "bob", Name: "model-4"}}
	s.CreateModel(c, &model4, nil, map[params.User]jujuparams.UserAccessPermission{"charlie": jujuparams.ModelReadAccess})

	err := s.JEM.GetModel(testContext, jemtest.Bob, jujuparams.ModelReadAccess, &model2)
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.GetModel(testContext, jemtest.Bob, jujuparams.ModelReadAccess, &model3)
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.GetModel(testContext, jemtest.Bob, jujuparams.ModelReadAccess, &model4)
	c.Assert(err, gc.Equals, nil)

	u := new(jimmdb.Update).Unset("cloud").Unset("cloudregion").Unset("credential").Unset("defaultseries")
	u.Unset("providertype").Unset("controlleruuid")
	err = s.JEM.DB.UpdateModel(testContext, &model2, u, false)
	c.Assert(err, gc.Equals, nil)

	tests := []struct {
		access       jujuparams.UserAccessPermission
		expectModels []*mongodoc.Model
	}{{
		access: jujuparams.ModelReadAccess,
		expectModels: []*mongodoc.Model{
			&model2,
			&model3,
			&model4,
		},
	}, {
		access: jujuparams.ModelWriteAccess,
		expectModels: []*mongodoc.Model{
			&model2,
			&model3,
		},
	}, {
		access: jujuparams.ModelAdminAccess,
		expectModels: []*mongodoc.Model{
			&model2,
		},
	}}

	for i, test := range tests {
		c.Logf("test %d. %s access", i, test.access)
		j := 0
		s.JEM.ForEachModel(testContext, jemtest.Charlie, test.access, func(m *mongodoc.Model) error {
			c.Assert(j < len(test.expectModels), gc.Equals, true)
			c.Check(m, jc.DeepEquals, test.expectModels[j])
			j++
			return nil
		})
		c.Check(j, gc.Equals, len(test.expectModels))
	}
}
