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

	"github.com/CanonicalLtd/jem/params"
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
		Request: newHTTPRequest("", nil),
		PathVar: httprouter.Params{{
			Key:   "Name",
			Value: "foo",
		}, {
			Key:   "User",
			Value: "bar",
		}},
	},
	val: &struct {
		Name params.Name `httprequest:",path"`
		User params.User `httprequest:",path"`
	}{
		Name: "foo",
		User: "bar",
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
	expectError: `cannot unmarshal into field: invalid name "foo_invalid"`,
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
	expectError: `cannot unmarshal into field: invalid user name "foo_invalid"`,
}}

func (*suite) TestValidators(c *gc.C) {
	for i, test := range validatorsTests {
		c.Logf("test %d: %s", i, test.about)
		v := reflect.New(reflect.TypeOf(test.val).Elem()).Interface()
		err := httprequest.Unmarshal(test.params, v)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(errgo.Cause(err), gc.Equals, httprequest.ErrUnmarshal)
		} else {
			c.Assert(v, jc.DeepEquals, test.val)
		}
	}
}

func newHTTPRequest(path string, body interface{}) *http.Request {
	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
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
