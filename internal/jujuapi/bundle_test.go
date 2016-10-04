// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	"encoding/pem"
	"net/http/httptest"
	"net/url"

	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/apitest"
)

type bundleSuite struct {
	apitest.Suite
	wsServer *httptest.Server
}

var _ = gc.Suite(&bundleSuite{})

func (s *bundleSuite) SetUpTest(c *gc.C) {
	s.Suite.SetUpTest(c)
	s.wsServer = httptest.NewTLSServer(s.JEMSrv)
}

func (s *bundleSuite) TearDownTest(c *gc.C) {
	s.wsServer.Close()
	s.Suite.TearDownTest(c)
}

func (s *bundleSuite) getChanges(args jujuparams.BundleChangesParams) (jujuparams.BundleChangesResults, error) {
	var result jujuparams.BundleChangesResults
	var info api.Info
	u, err := url.Parse(s.wsServer.URL)
	if err != nil {
		return result, errgo.Mask(err)
	}
	info.Addrs = []string{
		u.Host,
	}
	cert := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.wsServer.TLS.Certificates[0].Certificate[0],
	})
	info.CACert = string(cert)
	conn, err := api.Open(&info, api.DialOpts{
		InsecureSkipVerify: true,
		BakeryClient:       s.IDMSrv.Client("bob"),
	})
	if err != nil {
		return result, errgo.Mask(err)
	}
	defer conn.Close()
	err = conn.APICall("Bundle", 1, "", "GetChanges", args, &result)
	if err != nil {
		return result, errgo.Mask(err, errgo.Any)
	}
	return result, nil
}

func (s *bundleSuite) TestGetChangesBundleContentError(c *gc.C) {
	args := jujuparams.BundleChangesParams{
		BundleDataYAML: ":",
	}
	r, err := s.getChanges(args)
	c.Assert(err, gc.ErrorMatches, `cannot read bundle YAML: cannot unmarshal bundle data: yaml: did not find expected key`)
	c.Assert(r, gc.DeepEquals, jujuparams.BundleChangesResults{})
}

func (s *bundleSuite) TestGetChangesBundleVerificationErrors(c *gc.C) {
	args := jujuparams.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    to: [1]
                haproxy:
                    charm: 42
                    num_units: -1
        `,
	}
	r, err := s.getChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`placement "1" refers to a machine not defined in this bundle`,
		`too many units specified in unit placement for application "django"`,
		`invalid charm URL in application "haproxy": URL has invalid charm or bundle name: "42"`,
		`negative number of units specified on application "haproxy"`,
	})
}

func (s *bundleSuite) TestGetChangesBundleConstraintsError(c *gc.C) {
	args := jujuparams.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    constraints: bad=wolf
        `,
	}
	r, err := s.getChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`invalid constraints "bad=wolf" in application "django": unknown constraint "bad"`,
	})
}

func (s *bundleSuite) TestGetChangesBundleStorageError(c *gc.C) {
	args := jujuparams.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    storage:
                        bad: 0,100M
        `,
	}
	r, err := s.getChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`invalid storage "bad" in application "django": cannot parse count: count must be greater than zero, got "0"`,
	})
}

func (s *bundleSuite) TestGetChangesSuccess(c *gc.C) {
	args := jujuparams.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    options:
                        debug: true
                    storage:
                        tmpfs: tmpfs,1G
                haproxy:
                    charm: cs:trusty/haproxy-42
            relations:
                - - django:web
                  - haproxy:web
        `,
	}
	r, err := s.getChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, jc.DeepEquals, []*jujuparams.BundleChange{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Args:   []interface{}{"django", ""},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Args: []interface{}{
			"$addCharm-0",
			"",
			"django",
			map[string]interface{}{"debug": true},
			"",
			map[string]interface{}{"tmpfs": "tmpfs,1G"},
			map[string]interface{}{},
			map[string]interface{}{},
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Args:   []interface{}{"cs:trusty/haproxy-42", "trusty"},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Args: []interface{}{
			"$addCharm-2",
			"trusty",
			"haproxy",
			map[string]interface{}{},
			"",
			map[string]interface{}{},
			map[string]interface{}{},
			map[string]interface{}{},
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:       "addRelation-4",
		Method:   "addRelation",
		Args:     []interface{}{"$deploy-1:web", "$deploy-3:web"},
		Requires: []string{"deploy-1", "deploy-3"},
	}})
	c.Assert(r.Errors, gc.IsNil)
}

func (s *bundleSuite) TestGetChangesBundleEndpointBindingsSuccess(c *gc.C) {
	args := jujuparams.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    bindings:
                        url: public
        `,
	}
	r, err := s.getChanges(args)
	c.Assert(err, jc.ErrorIsNil)

	for _, change := range r.Changes {
		if change.Method == "deploy" {
			c.Assert(change, jc.DeepEquals, &jujuparams.BundleChange{
				Id:     "deploy-1",
				Method: "deploy",
				Args: []interface{}{
					"$addCharm-0",
					"",
					"django",
					map[string]interface{}{},
					"",
					map[string]interface{}{},
					map[string]interface{}{"url": "public"},
					map[string]interface{}{},
				},
				Requires: []string{"addCharm-0"},
			})
		}
	}
}
