// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/params"
)

type jimmSuite struct {
	websocketSuite
}

var _ = gc.Suite(&jimmSuite{})

func (s *jimmSuite) TestJIMMFacadeVersion(c *gc.C) {
	conn := s.open(c, nil, "test")
	defer conn.Close()
	c.Assert(conn.AllFacadeVersions()["JIMM"], jc.DeepEquals, []int{2, 3})
}

func (s *jimmSuite) TestListControllers(c *gc.C) {
	s.AddController(c, "dummy-0", s.APIInfo(c))
	s.AddController(c, "dummy-2", s.APIInfo(c))

	// Open the API connection as user "alice".
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	var resp params.ListControllerResponse
	err := conn.APICall("JIMM", 2, "", "ListControllers", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path:     params.EntityPath{"admin", "dummy-0"},
			Location: map[string]string{"cloud": "dummy", "region": "dummy-region"},
			Public:   true,
			UUID:     s.Model.Controller.UUID,
			Version:  s.Model.Controller.AgentVersion,
		}, {
			Path:     params.EntityPath{"admin", "dummy-1"},
			Location: map[string]string{"cloud": "dummy", "region": "dummy-region"},
			Public:   true,
			UUID:     s.Model.Controller.UUID,
			Version:  s.Model.Controller.AgentVersion,
		}, {
			Path:     params.EntityPath{"admin", "dummy-2"},
			Location: map[string]string{"cloud": "dummy", "region": "dummy-region"},
			Public:   true,
			UUID:     s.Model.Controller.UUID,
			Version:  s.Model.Controller.AgentVersion,
		}},
	})
}

func (s *jimmSuite) TestListControllersUnauthorizedUser(c *gc.C) {
	s.AddController(c, "dummy-0", s.APIInfo(c))
	s.AddController(c, "dummy-2", s.APIInfo(c))

	// Open the API connection as user "bob".
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	var resp params.ListControllerResponse
	err := conn.APICall("JIMM", 2, "", "ListControllers", nil, &resp)
	c.Assert(err, gc.Equals, nil)

	c.Assert(resp, jc.DeepEquals, params.ListControllerResponse{
		Controllers: []params.ControllerResponse{{
			Path:    params.EntityPath{User: "admin", Name: "jaas"},
			Public:  true,
			UUID:    "914487b5-60e7-42bb-bd63-1adc3fd3a388",
			Version: jujuversion.Current.String(),
		}},
	})
}

func (s *jimmSuite) TestListControllersV3(c *gc.C) {
	s.AddController(c, "dummy-0", s.APIInfo(c))
	s.AddController(c, "dummy-2", s.APIInfo(c))

	conn := s.open(c, nil, "alice")
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, gc.Equals, nil)
	c.Check(cis, jc.DeepEquals, []apiparams.ControllerInfo{{
		Name:          "dummy-0",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      "cloud-dummy",
		CloudRegion:   "dummy-region",
		Username:      "admin",
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}, {
		Name:          "dummy-1",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      "cloud-dummy",
		CloudRegion:   "dummy-region",
		Username:      "admin",
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}, {
		Name:          "dummy-2",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      "cloud-dummy",
		CloudRegion:   "dummy-region",
		Username:      "admin",
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}})
}

func (s *jimmSuite) TestListControllersV3Unauthorized(c *gc.C) {
	s.AddController(c, "dummy-0", s.APIInfo(c))
	s.AddController(c, "dummy-2", s.APIInfo(c))

	conn := s.open(c, nil, "bob")
	defer conn.Close()

	client := api.NewClient(conn)
	cis, err := client.ListControllers()
	c.Assert(err, gc.Equals, nil)
	c.Check(cis, jc.DeepEquals, []apiparams.ControllerInfo{{
		Name:         "jaas",
		UUID:         "914487b5-60e7-42bb-bd63-1adc3fd3a388",
		AgentVersion: s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	}})
}

func (s *jimmSuite) TestAddController(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)

	acr := apiparams.AddControllerRequest{
		Name:          "dummy-2",
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		Username:      s.APIInfo(c).Tag.Id(),
		Password:      s.APIInfo(c).Password,
	}

	ci, err := client.AddController(&acr)
	c.Assert(err, gc.Equals, nil)
	c.Check(ci, jc.DeepEquals, apiparams.ControllerInfo{
		Name:          "dummy-2",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      "cloud-dummy",
		CloudRegion:   "dummy-region",
		Username:      s.APIInfo(c).Tag.Id(),
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	})

	_, err = client.AddController(&acr)
	c.Check(err, gc.ErrorMatches, `controller "dummy-2" already exists \(already exists\)`)
	c.Check(jujuparams.IsCodeAlreadyExists(err), gc.Equals, true)

	conn = s.open(c, nil, "bob")
	defer conn.Close()
	client = api.NewClient(conn)
	acr.Name = "dummy-3"
	_, err = client.AddController(&acr)
	c.Check(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.IsCodeUnauthorized(err), gc.Equals, true)
}

func (s *jimmSuite) TestRemoveController(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.RemoveController(&apiparams.RemoveControllerRequest{
		Name: "dummy-1",
	})
	c.Check(err, gc.ErrorMatches, `controller is still alive \(still alive\)`)
	c.Check(jujuparams.ErrCode(err), gc.Equals, apiparams.CodeStillAlive)

	conn2 := s.open(c, nil, "bob")
	defer conn2.Close()
	client2 := api.NewClient(conn2)

	_, err = client2.RemoveController(&apiparams.RemoveControllerRequest{
		Name: "dummy-1",
	})
	c.Check(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeUnauthorized)

	ci, err := client.RemoveController(&apiparams.RemoveControllerRequest{
		Name:  "dummy-1",
		Force: true,
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(ci, jc.DeepEquals, apiparams.ControllerInfo{
		Name:          "dummy-1",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      "cloud-dummy",
		CloudRegion:   "dummy-region",
		Username:      s.APIInfo(c).Tag.Id(),
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	})
}

func (s *jimmSuite) TestSetControllerDeprecated(c *gc.C) {
	conn := s.open(c, nil, "alice")
	defer conn.Close()
	client := api.NewClient(conn)

	ci, err := client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "dummy-1",
		Deprecated: true,
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(ci, jc.DeepEquals, apiparams.ControllerInfo{
		Name:          "dummy-1",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      "cloud-dummy",
		CloudRegion:   "dummy-region",
		Username:      s.APIInfo(c).Tag.Id(),
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "deprecated",
		},
	})

	ci, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "dummy-1",
		Deprecated: false,
	})
	c.Assert(err, gc.Equals, nil)
	c.Check(ci, jc.DeepEquals, apiparams.ControllerInfo{
		Name:          "dummy-1",
		UUID:          s.Model.Controller.UUID,
		APIAddresses:  s.APIInfo(c).Addrs,
		CACertificate: s.APIInfo(c).CACert,
		CloudTag:      "cloud-dummy",
		CloudRegion:   "dummy-region",
		Username:      s.APIInfo(c).Tag.Id(),
		AgentVersion:  s.Model.Controller.AgentVersion,
		Status: jujuparams.EntityStatus{
			Status: "available",
		},
	})

	ci, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "dummy-2",
		Deprecated: true,
	})
	c.Check(err, gc.ErrorMatches, `controller not found \(not found\)`)
	c.Check(jujuparams.IsCodeNotFound(err), gc.Equals, true)

	conn = s.open(c, nil, "bob")
	defer conn.Close()
	client = api.NewClient(conn)
	ci, err = client.SetControllerDeprecated(&apiparams.SetControllerDeprecatedRequest{
		Name:       "dummy-1",
		Deprecated: true,
	})
	c.Check(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.IsCodeUnauthorized(err), gc.Equals, true)
}

func (s *jimmSuite) TestAuditLog(c *gc.C) {
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	c.Check(err, gc.ErrorMatches, `unauthorized \(unauthorized access\)`)
	c.Check(jujuparams.ErrCode(err), gc.Equals, jujuparams.CodeUnauthorized)

	mmclient := modelmanager.NewClient(conn)
	err = mmclient.DestroyModel(s.Model.Tag().(names.ModelTag), nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	conn2 := s.open(c, nil, "alice")
	defer conn2.Close()
	client2 := api.NewClient(conn2)

	evs, err := client2.FindAuditEvents(&apiparams.FindAuditEventsRequest{})
	c.Assert(err, gc.Equals, nil)

	c.Check(evs, jc.DeepEquals, apiparams.AuditEvents{
		Events: []apiparams.AuditEvent{{
			Time:    evs.Events[0].Time,
			Tag:     s.Model.Controller.Tag().String(),
			UserTag: names.NewUserTag("alice@external").String(),
			Action:  "add",
			Success: true,
			Params: map[string]string{
				"name": "dummy-1",
			},
		}, {
			Time:    evs.Events[1].Time,
			Tag:     s.Model.CloudCredential.Tag().String(),
			UserTag: s.Model.Owner.Tag().String(),
			Action:  "update",
			Success: true,
			Params: map[string]string{
				"skip-check":  "true",
				"skip-update": "false",
			},
		}, {
			Time:    evs.Events[2].Time,
			Tag:     s.Model.Tag().String(),
			UserTag: s.Model.Owner.Tag().String(),
			Action:  "create",
			Success: true,
			Params: map[string]string{
				"cloud":            "cloud-dummy",
				"cloud-credential": s.Model.CloudCredential.Tag().String(),
				"name":             "model-1",
				"owner":            s.Model.Owner.Tag().String(),
				"region":           "dummy-region",
			},
		}, {
			Time:    evs.Events[3].Time,
			Tag:     s.Model2.CloudCredential.Tag().String(),
			UserTag: s.Model2.Owner.Tag().String(),
			Action:  "update",
			Success: true,
			Params: map[string]string{
				"skip-check":  "true",
				"skip-update": "false",
			},
		}, {
			Time:    evs.Events[4].Time,
			Tag:     s.Model2.Tag().String(),
			UserTag: s.Model2.Owner.Tag().String(),
			Action:  "create",
			Success: true,
			Params: map[string]string{
				"cloud":            "cloud-dummy",
				"cloud-credential": s.Model2.CloudCredential.Tag().String(),
				"name":             "model-2",
				"owner":            s.Model2.Owner.Tag().String(),
				"region":           "dummy-region",
			},
		}, {
			Time:    evs.Events[5].Time,
			Tag:     s.Model3.Tag().String(),
			UserTag: s.Model3.Owner.Tag().String(),
			Action:  "create",
			Success: true,
			Params: map[string]string{
				"cloud":            "cloud-dummy",
				"cloud-credential": s.Model3.CloudCredential.Tag().String(),
				"name":             "model-3",
				"owner":            s.Model3.Owner.Tag().String(),
				"region":           "dummy-region",
			},
		}, {
			Time:    evs.Events[6].Time,
			Tag:     s.Model.Tag().String(),
			UserTag: s.Model.Owner.Tag().String(),
			Action:  "destroy",
			Success: true,
			Params:  map[string]string{},
		}},
	})
}

func (s *jimmSuite) TestFullModelStatus(c *gc.C) {
	s.AddController(c, "controller-1", s.APIInfo(c))
	mt := s.AddModel(c, names.NewUserTag("charlie@external"), "model-1", names.NewCloudTag("dummy"), "dummy-region", s.Model2.CloudCredential.Tag().(names.CloudCredentialTag))

	conn := s.open(c, nil, "bob")
	defer conn.Close()
	client := api.NewClient(conn)

	_, err := client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: "invalid-model-tag",
	})
	c.Assert(err, gc.ErrorMatches, `"invalid-model-tag" is not a valid tag \(bad request\)`)

	_, err = client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: mt.String(),
	})
	c.Assert(err, gc.ErrorMatches, "unauthorized.*")

	conn = s.open(c, nil, "alice@external")
	defer conn.Close()
	client = api.NewClient(conn)

	status, err := client.FullModelStatus(&apiparams.FullModelStatusRequest{
		ModelTag: mt.String(),
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(status, jemtest.CmpEquals(cmpopts.EquateEmpty(), cmpopts.IgnoreTypes(&time.Time{})), jujuparams.FullStatus{
		Model: jujuparams.ModelStatusInfo{
			Name:        "model-1",
			Type:        "iaas",
			CloudTag:    "cloud-dummy",
			CloudRegion: "dummy-region",
			Version:     "2.9-rc7",
			ModelStatus: jujuparams.DetailedStatus{
				Status: "available",
			},
			SLA: "unsupported",
		},
	})
}
