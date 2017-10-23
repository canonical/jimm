// Copyright 2016 Canonical Ltd.

package jem_test

import (
	"bufio"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

func (s *jemSuite) TestStats(c *gc.C) {
	ctx := auth.ContextWithUser(testContext, "bob")

	ctl1Id := s.addController(c, params.EntityPath{"bob", "controller1"})
	ctl2Id := s.addController(c, params.EntityPath{"bob", "controller2"})
	s.addController(c, params.EntityPath{"bob", "controller3"})
	err := s.jem.DB.AddModel(ctx, &mongodoc.Model{
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

	err = s.jem.DB.AddModel(ctx, &mongodoc.Model{
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

	err = s.jem.DB.AddModel(ctx, &mongodoc.Model{
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

	stats := s.pool.Stats(testContext)
	err = prometheus.Register(stats)
	c.Assert(err, gc.IsNil)
	defer prometheus.Unregister(stats)
	srv := httptest.NewServer(prometheus.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	c.Assert(err, gc.IsNil)
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
