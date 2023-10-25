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

	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/params"
)

func (s *jemSuite) TestModelStats(c *gc.C) {
	ctl2Id := params.EntityPath{User: "alice", Name: "controller-2"}
	s.AddController(c, &mongodoc.Controller{Path: ctl2Id})
	ctl3Id := params.EntityPath{User: "alice", Name: "controller-3"}
	s.AddController(c, &mongodoc.Controller{Path: ctl3Id})

	err := s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path: params.EntityPath{
			User: "bob",
			Name: "model-2",
		},
		UUID:       "201852f4-022d-4f98-9b63-a6ff52c0798e",
		Controller: s.Controller.Path,
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

	err = s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path: params.EntityPath{
			User: "bob",
			Name: "model-3",
		},
		UUID:       "4cb1030f-d59d-4c8c-bac7-eb8166c54eb0",
		Controller: s.Controller.Path,
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

	err = s.JEM.DB.InsertModel(testContext, &mongodoc.Model{
		Path: params.EntityPath{
			User: "bob",
			Name: "model-4",
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

	stats := s.Pool.ModelStats(testContext)
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
		"models_running":        4,
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
	ctl2Id := params.EntityPath{"bob", "controller2"}
	s.AddController(c, &mongodoc.Controller{Path: ctl2Id})

	model2 := mongodoc.Model{Path: params.EntityPath{"bob", "model2"}}
	s.CreateModel(c, &model2, nil, nil)
	model3 := mongodoc.Model{
		Path:       params.EntityPath{"bob", "model3"},
		Controller: ctl2Id,
	}
	s.CreateModel(c, &model3, nil, nil)

	statuses := []status.Status{
		status.Started,
		status.Pending,
		status.Stopped,
		status.Down,
		status.Started,
		status.Pending,
		status.Stopped,
		status.Started,
		status.Pending,
		status.Started,
	}
	for i, st := range statuses {
		err := s.JEM.UpdateMachineInfo(testContext, s.Controller.Path, &jujuparams.MachineInfo{
			Id:        fmt.Sprintf("%d", i),
			ModelUUID: s.Model.UUID,
			AgentStatus: jujuparams.StatusInfo{
				Current: st,
			},
		})
		c.Assert(err, gc.Equals, nil)
	}

	err := s.JEM.UpdateMachineInfo(testContext, s.Controller.Path, &jujuparams.MachineInfo{
		Id:        "0",
		ModelUUID: model2.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Started,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.UpdateMachineInfo(testContext, ctl2Id, &jujuparams.MachineInfo{
		Id:        "0",
		ModelUUID: model3.UUID,
		AgentStatus: jujuparams.StatusInfo{
			Current: status.Pending,
		},
	})
	c.Assert(err, gc.Equals, nil)

	stats := s.Pool.MachineStats(testContext)
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
		{s.Controller.Path, jemtest.TestCloudName, jemtest.TestCloudRegionName, status.Started}: 5,
		{s.Controller.Path, jemtest.TestCloudName, jemtest.TestCloudRegionName, status.Pending}: 3,
		{s.Controller.Path, jemtest.TestCloudName, jemtest.TestCloudRegionName, status.Stopped}: 2,
		{s.Controller.Path, jemtest.TestCloudName, jemtest.TestCloudRegionName, status.Down}:    1,
		{ctl2Id, jemtest.TestCloudName, jemtest.TestCloudRegionName, status.Pending}:            1,
	}
	for label, count := range expectCounts {
		name := fmt.Sprintf("jem_health_machines{cloud=%q,controller=%q,region=%q,status=%q}", label.cloud, label.controller, label.region, label.status)
		c.Check(math.Trunc(counts[name]), gc.Equals, float64(count), gc.Commentf("%s", name))
	}
}
