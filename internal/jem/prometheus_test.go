// Copyright 2016 Canonical Ltd.

package jem_test

import (
	"bufio"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/status"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

func (s *jemSuite) TestModelStats(c *gc.C) {
	ctl1Id := s.addController(c, params.EntityPath{"bob", "controller1"})
	ctl2Id := s.addController(c, params.EntityPath{"bob", "controller2"})
	s.addController(c, params.EntityPath{"bob", "controller3"})
	err := s.jem.DB.AddModel(testContext, &mongodoc.Model{
		Path: params.EntityPath{
			User: "bob",
			Name: "model1",
		},
		UUID:       "201852f4-022d-4f98-9b63-a6ff52c0798e",
		Controller: ctl1Id,
		Counts: map[params.EntityCount]params.Count{
			params.ApplicationCount: {
				Current: 20,
				Max:     20,
			},
			params.MachineCount: {
				Current: 12,
				Max:     12,
			},
			params.UnitCount: {
				Current: 40,
				Max:     40,
			},
		},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.jem.DB.AddModel(testContext, &mongodoc.Model{
		Path: params.EntityPath{
			User: "bob",
			Name: "model2",
		},
		UUID:       "4cb1030f-d59d-4c8c-bac7-eb8166c54eb0",
		Controller: ctl1Id,
		Counts: map[params.EntityCount]params.Count{
			params.ApplicationCount: {
				Current: 0,
				Max:     0,
			},
			params.MachineCount: {
				Current: 0,
				Max:     0,
			},
			params.UnitCount: {
				Current: 0,
				Max:     0,
			},
		},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.jem.DB.AddModel(testContext, &mongodoc.Model{
		Path: params.EntityPath{
			User: "bob",
			Name: "model3",
		},
		UUID:       "05d96523-4f3c-48f8-aab7-43aafb12bbc7",
		Controller: ctl2Id,
		Counts: map[params.EntityCount]params.Count{
			params.ApplicationCount: {
				Current: 10,
				Max:     10,
			},
			params.MachineCount: {
				Current: 10,
				Max:     10,
			},
			params.UnitCount: {
				Current: 10,
				Max:     10,
			},
		},
	})
	c.Assert(err, gc.Equals, nil)

	stats := s.pool.ModelStats(testContext)
	err = prometheus.Register(stats)
	c.Assert(err, gc.Equals, nil)
	defer prometheus.Unregister(stats)
	srv := httptest.NewServer(promhttp.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	defer resp.Body.Close()
	counts := make(map[string]float64)
	for scan := bufio.NewScanner(resp.Body); scan.Scan(); {
		t := scan.Text()
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if i := strings.Index(t, "{"); i >= 0 {
			j := strings.LastIndex(t, "}")
			t = t[0:i] + t[j+1:]
		}
		fields := strings.Fields(t)
		if len(fields) != 2 {
			c.Logf("unexpected prometheus line %q", scan.Text())
			continue
		}
		f, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			c.Logf("bad value in prometheus line %q", scan.Text())
			continue
		}
		counts[fields[0]] += f
	}

	expectCounts := map[string]int{
		"applications_running":  30,
		"controllers_running":   3,
		"models_running":        3,
		"active_models_running": 2,
		"units_running":         50,
		"machines_running":      22,
	}
	for name, count := range expectCounts {
		name = "jem_health_" + name
		c.Check(math.Trunc(counts[name]), gc.Equals, float64(count), gc.Commentf("%s", name))
	}
}

func (s *jemSuite) TestMachineStats(c *gc.C) {
	ctl1Id := s.addController(c, params.EntityPath{"bob", "controller1"})
	ctl2Id := s.addController(c, params.EntityPath{"bob", "controller2"})
	err := jem.UpdateCredential(s.jem.DB, testContext, &mongodoc.Credential{
		Path: mgoCredentialPath("dummy", "bob", "cred1"),
		Type: "empty",
	})
	c.Assert(err, gc.Equals, nil)
	id := jemtest.NewIdentity("bob")
	var m1 jujuparams.ModelInfo
	err = s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model1"},
		ControllerPath: ctl1Id,
		Credential:     credentialPath("dummy", "bob", "cred1"),
		Cloud:          "dummy",
	}, &m1)
	c.Assert(err, gc.Equals, nil)
	var m2 jujuparams.ModelInfo
	err = s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model2"},
		ControllerPath: ctl1Id,
		Credential:     credentialPath("dummy", "bob", "cred1"),
		Cloud:          "dummy",
	}, &m2)
	c.Assert(err, gc.Equals, nil)
	var m3 jujuparams.ModelInfo
	err = s.jem.CreateModel(testContext, id, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "model3"},
		ControllerPath: ctl2Id,
		Credential:     credentialPath("dummy", "bob", "cred1"),
		Cloud:          "dummy",
	}, &m3)
	c.Assert(err, gc.Equals, nil)

	err = s.jem.UpdateMachineInfo(testContext, ctl1Id, &jujuparams.MachineInfo{
		Id:        "0",
		ModelUUID: m1.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Started,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctl1Id, &jujuparams.MachineInfo{
		Id:        "1",
		ModelUUID: m1.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Pending,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctl1Id, &jujuparams.MachineInfo{
		Id:        "2",
		ModelUUID: m1.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Stopped,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctl1Id, &jujuparams.MachineInfo{
		Id:        "3",
		ModelUUID: m1.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Down,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctl1Id, &jujuparams.MachineInfo{
		Id:        "4",
		ModelUUID: m1.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Started,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctl1Id, &jujuparams.MachineInfo{
		Id:        "5",
		ModelUUID: m1.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Pending,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctl1Id, &jujuparams.MachineInfo{
		Id:        "6",
		ModelUUID: m1.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Stopped,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctl1Id, &jujuparams.MachineInfo{
		Id:        "7",
		ModelUUID: m1.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Started,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctl1Id, &jujuparams.MachineInfo{
		Id:        "8",
		ModelUUID: m1.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Pending,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctl1Id, &jujuparams.MachineInfo{
		Id:        "9",
		ModelUUID: m1.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Started,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctl1Id, &jujuparams.MachineInfo{
		Id:        "0",
		ModelUUID: m2.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Started,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.jem.UpdateMachineInfo(testContext, ctl2Id, &jujuparams.MachineInfo{
		Id:        "0",
		ModelUUID: m3.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Pending,
		},
	})
	c.Assert(err, gc.Equals, nil)

	stats := s.pool.MachineStats(testContext)
	err = prometheus.Register(stats)
	c.Assert(err, gc.Equals, nil)
	defer prometheus.Unregister(stats)
	srv := httptest.NewServer(promhttp.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	c.Assert(err, gc.Equals, nil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	defer resp.Body.Close()
	counts := make(map[string]float64)
	for scan := bufio.NewScanner(resp.Body); scan.Scan(); {
		t := scan.Text()
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		fields := strings.Fields(t)
		if len(fields) != 2 {
			c.Logf("unexpected prometheus line %q", scan.Text())
			continue
		}
		f, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			c.Logf("bad value in prometheus line %q", scan.Text())
			continue
		}
		counts[fields[0]] += f
	}

	type labels struct {
		controller params.EntityPath
		cloud      string
		region     string
		status     status.Status
	}

	expectCounts := map[labels]int{
		{ctl1Id, "dummy", "dummy-region", status.Started}: 5,
		{ctl1Id, "dummy", "dummy-region", status.Pending}: 3,
		{ctl1Id, "dummy", "dummy-region", status.Stopped}: 2,
		{ctl1Id, "dummy", "dummy-region", status.Down}:    1,
		{ctl2Id, "dummy", "dummy-region", status.Pending}: 1,
	}
	for label, count := range expectCounts {
		name := fmt.Sprintf("jem_health_machines{cloud=%q,controller=%q,region=%q,status=%q}", label.cloud, label.controller, label.region, label.status)
		c.Check(math.Trunc(counts[name]), gc.Equals, float64(count), gc.Commentf("%s", name))
	}
}
