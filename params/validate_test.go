// Copyright 2015 Canonical Ltd.

package params_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"

	"github.com/juju/httprequest"
	jc "github.com/juju/testing/checkers"
	"github.com/julienschmidt/httprouter"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/params"
)

type suite struct{}

var _ = gc.Suite(&suite{})

var validatorsTests = []struct {
	about       string
	params      httprequest.Params
	val         interface{}
	expectError string
}{{
	about: "valid params",
	params: httprequest.Params{
		Request: newHTTPRequest("/foo/bar/x?Path=foo/bar", "baz/frob"),
		PathVar: httprouter.Params{{
			Key:   "Name",
			Value: "foo",
		}, {
			Key:   "User",
			Value: "bar",
		}, {
			Key:   "slname",
			Value: "x",
		}},
	},
	val: &struct {
		Name             params.Name       `httprequest:",path"`
		User             params.User       `httprequest:",path"`
		Path             params.EntityPath `httprequest:",form"`
		Body             params.EntityPath `httprequest:",body"`
		SingleLetterName params.Name       `httprequest:"slname,path"`
	}{
		Name:             "foo",
		User:             "bar",
		SingleLetterName: "x",
		Path:             params.EntityPath{"foo", "bar"},
		Body:             params.EntityPath{"baz", "frob"},
	},
}, {
	about: "invalid Name in path",
	params: httprequest.Params{
		Request: newHTTPRequest("", nil),
		PathVar: httprouter.Params{{
			Key:   "Name",
			Value: "foo_invalid",
		}},
	},
	val: new(struct {
		Name params.Name `httprequest:",path"`
	}),
	expectError: `cannot unmarshal into field.*: invalid name "foo_invalid"`,
}, {
	about: "invalid User in path",
	params: httprequest.Params{
		Request: newHTTPRequest("", nil),
		PathVar: httprouter.Params{{
			Key:   "User",
			Value: "foo_invalid",
		}},
	},
	val: new(struct {
		User params.User `httprequest:",path"`
	}),
	expectError: `cannot unmarshal into field.*: invalid user name "foo_invalid"`,
}, {
	about: "double hyphen in user",
	params: httprequest.Params{
		Request: newHTTPRequest("", nil),
		PathVar: httprouter.Params{{
			Key:   "User",
			Value: "foo--valid",
		}},
	},
	val: &struct {
		User params.User `httprequest:",path"`
	}{
		User: "foo--valid",
	},
}, {
	about: "double hyphen in name",
	params: httprequest.Params{
		Request: newHTTPRequest("", nil),
		PathVar: httprouter.Params{{
			Key:   "Name",
			Value: "foo--valid",
		}},
	},
	val: &struct {
		Name params.Name `httprequest:",path"`
	}{
		Name: "foo--valid",
	},
}, {
	about: "bad user in entity path",
	params: httprequest.Params{
		Request: newHTTPRequest("/?Path=/xx", nil),
	},
	val: new(struct {
		Path params.EntityPath `httprequest:",form"`
	}),
	expectError: `cannot unmarshal into field.*: invalid user name ""`,
}, {
	about: "bad name in entity path",
	params: httprequest.Params{
		Request: newHTTPRequest("/?Path=xx/", nil),
	},
	val: new(struct {
		Path params.EntityPath `httprequest:",form"`
	}),
	expectError: `cannot unmarshal into field.*: invalid name ""`,
}}

func (*suite) TestValidators(c *gc.C) {
	for i, test := range validatorsTests {
		c.Logf("test %d: %s", i, test.about)
		v := reflect.New(reflect.TypeOf(test.val).Elem()).Interface()
		err := httprequest.Unmarshal(test.params, v)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(errgo.Cause(err), gc.Equals, httprequest.ErrUnmarshal)
			continue
		}
		c.Assert(err, gc.Equals, nil)
		c.Assert(v, jc.DeepEquals, test.val)
	}
}

func (*suite) TestEntityPathMarshalText(c *gc.C) {
	ep := params.EntityPath{
		User: "foo",
		Name: "bar",
	}
	data, err := ep.MarshalText()
	c.Assert(err, gc.Equals, nil)
	c.Assert(string(data), gc.Equals, "foo/bar")
}

var isValidLocationAttrTests = []struct {
	attr     string
	expectOK bool
}{
	{"foo", true},
	{"x", true},
	{"", false},
	{"foo-bar", true},
	{"foo bar", false},
	{"foo--bar", false},
	{"foobar-", false},
	{"-foobar", false},
	{"x.y", false},
	{"$field", false},
}

func (*suite) TestIsValidLocationAttr(c *gc.C) {
	for i, test := range isValidLocationAttrTests {
		c.Logf("test %d: %q", i, test.attr)
		c.Assert(params.IsValidLocationAttr(test.attr), gc.Equals, test.expectOK)
	}
}

func newHTTPRequest(path string, body interface{}) *http.Request {
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		panic(err)
	}
	if err := req.ParseForm(); err != nil {
		panic(err)
	}
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		req.Body = ioutil.NopCloser(bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}
