// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	jujuparams "github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
)

type bundleSuite struct {
	websocketSuite
}

var _ = gc.Suite(&bundleSuite{})

func (s *bundleSuite) getChanges(c *gc.C, args jujuparams.BundleChangesParams) (jujuparams.BundleChangesResults, error) {
	var result jujuparams.BundleChangesResults
	conn := s.open(c, nil, "bob")
	defer conn.Close()
	err := conn.APICall("Bundle", 1, "", "GetChanges", args, &result)
	if err != nil {
		return result, errgo.Mask(err, errgo.Any)
	}
	return result, nil
}

func (s *bundleSuite) TestGetChangesBundleContentError(c *gc.C) {
	args := jujuparams.BundleChangesParams{
		BundleDataYAML: ":",
	}
	r, err := s.getChanges(c, args)
	c.Assert(err, gc.ErrorMatches, `cannot read bundle YAML: unmarshal document 0: yaml: did not find expected key`)
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
	r, err := s.getChanges(c, args)
	c.Assert(err, gc.Equals, nil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`too many units specified in unit placement for application "django"`,
		`placement "1" refers to a machine not defined in this bundle`,
		`invalid charm URL in application "haproxy": cannot parse name and/or revision in URL "42": name "42" not valid`,
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
	r, err := s.getChanges(c, args)
	c.Assert(err, gc.Equals, nil)
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
	r, err := s.getChanges(c, args)
	c.Assert(err, gc.Equals, nil)
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
	r, err := s.getChanges(c, args)
	c.Assert(err, gc.Equals, nil)
	c.Assert(r.Changes, jc.DeepEquals, []*jujuparams.BundleChange{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Args:   []interface{}{"django", "", ""},
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
			0.0,
			"",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Args:   []interface{}{"cs:trusty/haproxy-42", "trusty", ""},
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
			0.0,
			"",
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
	r, err := s.getChanges(c, args)
	c.Assert(err, gc.Equals, nil)

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
					0.0,
					"",
				},
				Requires: []string{"addCharm-0"},
			})
		}
	}
}
