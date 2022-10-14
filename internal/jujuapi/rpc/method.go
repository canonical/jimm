// Copyright 2020 Canonical Ltd.

package rpc

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/juju/apiserver/errors"
	"github.com/juju/rpcreflect"
)

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType   = reflect.TypeOf((*error)(nil)).Elem()
	stringType  = reflect.TypeOf("")
)

// Method converts the given function to an RPC method that can be used
// with Root. The function must have a signature like:
//
//     f([ctx context.Context, ][objId string, ][params ParamsT]) ([ResultT, ][error])
//
// Note that all parameters and return values are optional. Method will
// panic if the given value is not a function of the correct type.
func Method(f interface{}) rpcreflect.MethodCaller {
	v := reflect.ValueOf(f)
	if k := v.Kind(); k != reflect.Func {
		panic(fmt.Sprintf("method must be a func not %s", k))
	}
	t := v.Type()

	m := methodCaller{
		call: v,
	}

	var n int
	if t.NumIn() > 0 && t.In(0) == contextType {
		m.flags |= inContext
		n++
	}
	if t.NumIn() > n && t.In(n) == stringType {
		m.flags |= inObjectID
		n++
	}
	if t.NumIn() > n && t.In(n).Kind() == reflect.Struct {
		m.flags |= inParams
		m.paramsType = t.In(n)
		n++
	}
	if t.NumIn() != n {
		panic("method has invalid signature")
	}

	n = 0
	if t.NumOut() > 0 && t.Out(0).Kind() == reflect.Struct {
		m.flags |= outResult
		m.resultType = t.Out(0)
		n++
	}
	if t.NumOut() > n && t.Out(n) == errorType {
		m.flags |= outError
		n++
	}
	if t.NumOut() != n {
		panic("method has invalid signature")
	}

	return m
}

type flags int

const (
	inContext flags = 1 << iota
	inObjectID
	inParams
	outResult
	outError
)

type methodCaller struct {
	paramsType, resultType reflect.Type
	flags                  flags
	call                   reflect.Value
}

func (c methodCaller) ParamsType() reflect.Type {
	return c.paramsType
}

func (c methodCaller) ResultType() reflect.Type {
	return c.resultType
}

func (c methodCaller) Call(ctx context.Context, objId string, arg reflect.Value) (reflect.Value, error) {
	var pv []reflect.Value
	if c.flags&inContext == inContext {
		pv = append(pv, reflect.ValueOf(ctx))
	}
	if c.flags&inObjectID == inObjectID {
		pv = append(pv, reflect.ValueOf(objId))
	} else {
		if objId != "" {
			return reflect.Value{}, errors.ErrBadId
		}
	}
	if c.flags&inParams == inParams {
		pv = append(pv, arg)
	}

	rv := c.call.Call(pv)

	var n int
	var res reflect.Value
	var err error
	if c.flags&outResult == outResult {
		res = rv[n]
		n++
	}
	if c.flags&outError == outError {
		if !rv[n].IsNil() {
			err = rv[n].Interface().(error)
		}
	}
	return res, err
}
