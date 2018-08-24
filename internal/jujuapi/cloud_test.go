// Copyright 2016 Canonical Ltd.

package jujuapi_test

import (
	jujuparams "github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jujuapi"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
)

type cloudSuite struct{}

var _ = gc.Suite(&cloudSuite{})

var mergeStringsTests = []struct {
	about    string
	s1       []string
	s2       []string
	expectSS []string
}{{
	about: "both empty",
}, {
	about:    "s1 empty",
	s2:       []string{"a", "b"},
	expectSS: []string{"a", "b"},
}, {
	about:    "s2 empty",
	s1:       []string{"a", "b"},
	expectSS: []string{"a", "b"},
}, {
	about:    "both the same",
	s1:       []string{"a", "b"},
	s2:       []string{"a", "b"},
	expectSS: []string{"a", "b"},
}, {
	about:    "overlap",
	s1:       []string{"a", "b"},
	s2:       []string{"b", "c"},
	expectSS: []string{"a", "b", "c"},
}, {
	about:    "all different",
	s1:       []string{"a", "c"},
	s2:       []string{"b", "d"},
	expectSS: []string{"a", "b", "c", "d"},
}, {
	about:    "larger set",
	s1:       []string{"a", "b", "d", "e"},
	s2:       []string{"ab", "d", "de", "df", "f"},
	expectSS: []string{"a", "ab", "b", "d", "de", "df", "e", "f"},
}}

func (s *cloudSuite) TestMergeStrings(c *gc.C) {
	for i, test := range mergeStringsTests {
		c.Logf("%d. %s", i, test.about)
		ss := jujuapi.MergeStrings(test.s1, test.s2)
		c.Assert(ss, jc.DeepEquals, test.expectSS)
	}
}

var mergeRegionsTests = []struct {
	about         string
	regions1      []jujuparams.CloudRegion
	regions2      []jujuparams.CloudRegion
	expectRegions []jujuparams.CloudRegion
}{{
	about: "both empty",
}, {
	about: "regionss1 empty",
	regions2: []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "b",
	}},
	expectRegions: []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "b",
	}},
}, {
	about: "s2 empty",
	regions1: []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "b",
	}},
	expectRegions: []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "b",
	}},
}, {
	about: "both the same",
	regions1: []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "b",
	}},
	regions2: []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "b",
	}},
	expectRegions: []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "b",
	}},
}, {
	about: "overlap",
	regions1: []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "b",
	}},
	regions2: []jujuparams.CloudRegion{{
		Name: "b",
	}, {
		Name: "c",
	}},
	expectRegions: []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "b",
	}, {
		Name: "c",
	}},
}, {
	about: "all different",
	regions1: []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "c",
	}},
	regions2: []jujuparams.CloudRegion{{
		Name: "b",
	}, {
		Name: "d",
	}},
	expectRegions: []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "b",
	}, {
		Name: "c",
	}, {
		Name: "d",
	}},
}}

func (s *cloudSuite) TestMergeRegions(c *gc.C) {
	for i, test := range mergeRegionsTests {
		c.Logf("%d. %s", i, test.about)
		regions := jujuapi.MergeRegions(test.regions1, test.regions2)
		c.Assert(regions, jc.DeepEquals, test.expectRegions)
	}
}

func (s *cloudSuite) TestMakeRegions(c *gc.C) {
	regions := jujuapi.MakeRegions([]mongodoc.Region{{
		Name: "b",
	}, {
		Name: "a",
	}})
	c.Assert(regions, jc.DeepEquals, []jujuparams.CloudRegion{{
		Name: "a",
	}, {
		Name: "b",
	}})
}
