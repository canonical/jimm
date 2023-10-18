// Copyright 2020 Canonical Ltd.

package rpc

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/juju/rpcreflect"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
)

// A Root provides the root of an RPC server connection.
type Root struct {
	methodMu sync.RWMutex
	methods  map[string]rpcreflect.MethodCaller

	inflightMu sync.Mutex
	callID     uint64
	inflight   map[uint64]func()
}

// AddMethod adds, or replaces, the given method in the Root.
func (r *Root) AddMethod(rootName string, version int, methodName string, mc rpcreflect.MethodCaller) {
	r.methodMu.Lock()
	defer r.methodMu.Unlock()
	if r.methods == nil {
		r.methods = make(map[string]rpcreflect.MethodCaller)
	}
	r.methods[fmt.Sprintf("%s-%d-%s", rootName, version, methodName)] = mc
}

// RemoveMethod removes the given method from the Root.
func (r *Root) RemoveMethod(rootName string, version int, methodName string) {
	r.methodMu.Lock()
	defer r.methodMu.Unlock()
	delete(r.methods, fmt.Sprintf("%s-%d-%s", rootName, version, methodName))
}

// FindMethod implements rpc.Root.
func (r *Root) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	key := fmt.Sprintf("%s-%d-%s", rootName, version, methodName)
	zapctx.Debug(context.Background(), "finding method", zap.String("root", rootName), zap.Int("version", version), zap.String("methodName", methodName))
	r.methodMu.RLock()
	defer r.methodMu.RUnlock()
	if caller, ok := r.methods[key]; ok {
		return rootMethodCaller{
			MethodCaller: caller,
			r:            r,
		}, nil
	}
	return nil, &rpcreflect.CallNotImplementedError{
		RootMethod: rootName,
		Version:    version,
		Method:     methodName,
	}
}

// Kill implements rpc.Root.
func (r *Root) Kill() {
	r.inflightMu.Lock()
	defer r.inflightMu.Unlock()
	// cancel all inflight requests.
	for _, cancel := range r.inflight {
		cancel()
	}
}

func (r *Root) start(ctx context.Context) (context.Context, uint64) {
	ctx, cancel := context.WithCancel(ctx)
	r.inflightMu.Lock()
	defer r.inflightMu.Unlock()
	if r.inflight == nil {
		r.inflight = make(map[uint64]func())
	}
	callID := r.callID
	r.callID++
	r.inflight[callID] = cancel
	return ctx, callID
}

func (r *Root) end(callID uint64) {
	r.inflightMu.Lock()
	defer r.inflightMu.Unlock()
	cancel := r.inflight[callID]
	if cancel != nil {
		cancel()
	}
	delete(r.inflight, callID)
}

// rootMethodCaller wraps an rpcreflect.MethodCaller so that if the
// root's Kill method is called the context of the method will also be
// canceled.
type rootMethodCaller struct {
	rpcreflect.MethodCaller

	r *Root
}

// Call implements rpcreflect.MethodCaller.Call.
func (c rootMethodCaller) Call(ctx context.Context, objID string, arg reflect.Value) (reflect.Value, error) {
	ctx, callID := c.r.start(ctx)
	defer c.r.end(callID)
	return c.MethodCaller.Call(ctx, objID, arg)
}
