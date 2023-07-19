// Copyright 2020 Canonical Ltd.

package rpc_test

import (
	"context"
	"reflect"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/internal/jujuapi/rpc"
)

type S1 struct{}
type S2 struct{}

var procTests = []struct {
	name             string
	f                interface{}
	expectPanic      string
	expectParamsType reflect.Type
	expectResultType reflect.Type
}{{
	name:             "full",
	f:                func(ctx context.Context, objID string, params S1) (resutlt S2, err error) { return },
	expectParamsType: reflect.TypeOf(S1{}),
	expectResultType: reflect.TypeOf(S2{}),
}, {
	name:             "no context",
	f:                func(objID string, params S1) (resutlt S2, err error) { return },
	expectParamsType: reflect.TypeOf(S1{}),
	expectResultType: reflect.TypeOf(S2{}),
}, {
	name:             "no object ID",
	f:                func(ctx context.Context, params S1) (resutlt S2, err error) { return },
	expectParamsType: reflect.TypeOf(S1{}),
	expectResultType: reflect.TypeOf(S2{}),
}, {
	name:             "no params",
	f:                func(ctx context.Context, objId string) (resutlt S2, err error) { return },
	expectParamsType: nil,
	expectResultType: reflect.TypeOf(S2{}),
}, {
	name:             "no result",
	f:                func(ctx context.Context, objId string, params S1) (err error) { return },
	expectParamsType: reflect.TypeOf(S1{}),
	expectResultType: nil,
}, {
	name:             "no error",
	f:                func(ctx context.Context, objId string, params S1) (result S2) { return },
	expectParamsType: reflect.TypeOf(S1{}),
	expectResultType: reflect.TypeOf(S2{}),
}, {
	name:             "empty function",
	f:                func() {},
	expectParamsType: nil,
	expectResultType: nil,
}, {
	name:        "not function",
	f:           1,
	expectPanic: "method must be a func not int",
}, {
	name:        "bad parameter order",
	f:           func(objID string, ctx context.Context, params S1) (resutlt S2, err error) { return },
	expectPanic: "method has invalid signature",
}, {
	name:        "bad result order",
	f:           func(ctx context.Context, objID string, params S1) (err error, resutlt S2) { return },
	expectPanic: "method has invalid signature",
}}

func TestProcedure(t *testing.T) {
	c := qt.New(t)

	for _, test := range procTests {
		c.Run(test.name, func(c *qt.C) {
			if test.expectPanic != "" {
				c.Assert(func() { rpc.Method(test.f) }, qt.PanicMatches, test.expectPanic)
				return
			}
			mc := rpc.Method(test.f)
			c.Assert(mc.ParamsType(), qt.Equals, test.expectParamsType)
			c.Assert(mc.ResultType(), qt.Equals, test.expectResultType)
		})
	}
}
