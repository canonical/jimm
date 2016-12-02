// Copyright 2016 Canonical Ltd.

package jem_test

import (
	"bufio"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

func (s *jemSuite) TestStats(c *gc.C) {
	ctx := auth.ContextWithUser(testContext, "bob")

	ctl1Id := s.addController(c, params.EntityPath{"bob", "controller1"})
	err := s.jem.DB.SetControllerStats(ctx, ctl1Id, &mongodoc.ControllerStats{
		UnitCount:    35,
		ModelCount:   23,
		ServiceCount: 5,
		MachineCount: 500,
	})
	c.Assert(err, jc.ErrorIsNil)
	ctl2Id := s.addController(c, params.EntityPath{"bob", "controller2"})
	err = s.jem.DB.SetControllerStats(ctx, ctl2Id, &mongodoc.ControllerStats{
		UnitCount:    1,
		ModelCount:   1,
		ServiceCount: 1,
		MachineCount: 1,
	})
	c.Assert(err, jc.ErrorIsNil)

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
		"applications_running": 6,
		"controllers_running":  2,
		"models_running":       24,
		"units_running":        36,
		"machines_running":     501,
	}
	for name, count := range expectCounts {
		name = "jem_health_" + name
		c.Check(math.Trunc(counts[name]), gc.Equals, float64(count), gc.Commentf("%s", name))
	}
}
