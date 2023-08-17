// Copyright 2020 Canonical Ltd.

package rpc_test

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujurpc "github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"

	"github.com/canonical/jimm/internal/jujuapi/rpc"
)

func TestRPC(t *testing.T) {
	c := qt.New(t)

	cl, srv := pipe()
	c.Cleanup(func() {
		if err := cl.Close(); err != nil {
			c.Logf("error closing RPC connection: %s", err)
		}
	})

	r := new(rpc.Root)
	srv.ServeRoot(r, nil, nil)

	req := jujurpc.Request{
		Type:    "Calc",
		Version: 1,
		Id:      "",
		Action:  "Add",
	}
	params := AddRequest{
		A: 1,
		B: 2,
	}
	var res AddResult
	err := cl.Call(req, params, &res)
	c.Assert(err, qt.ErrorMatches, `no such request - method Calc\(1\).Add is not implemented \(not implemented\)`)

	r.AddMethod("Calc", 1, "Add", rpc.Method(add))
	err = cl.Call(req, params, &res)
	c.Assert(err, qt.Equals, nil)
	c.Assert(res.Sum, qt.Equals, 3)

	r.RemoveMethod("Calc", 1, "Add")
	err = cl.Call(req, params, &res)
	c.Assert(err, qt.ErrorMatches, `no such request - method Calc\(1\).Add is not implemented \(not implemented\)`)
}

func TestKill(t *testing.T) {
	c := qt.New(t)

	cl, srv := pipe()
	c.Cleanup(func() {
		if err := cl.Close(); err != nil {
			c.Logf("error closing RPC connection: %s", err)
		}
	})

	r := new(rpc.Root)
	srv.ServeRoot(r, nil, nil)

	var started int32
	var ended int32
	wait := func(ctx context.Context) {
		atomic.AddInt32(&started, 1)
		defer atomic.AddInt32(&ended, 1)
		<-ctx.Done()
	}

	r.AddMethod("Test", 1, "Wait", rpc.Method(wait))
	req := jujurpc.Request{
		Type:    "Test",
		Version: 1,
		Id:      "",
		Action:  "Wait",
	}
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			err := cl.Call(req, nil, nil)
			c.Check(err, qt.Equals, nil)
		}()
	}
	for atomic.LoadInt32(&started) < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	r.Kill()
	wg.Wait()
	c.Check(atomic.LoadInt32(&started), qt.Equals, int32(2))
	c.Check(atomic.LoadInt32(&ended), qt.Equals, int32(2))
}

func pipe() (*jujurpc.Conn, *jujurpc.Conn) {
	c1, c2 := net.Pipe()
	rpc1 := jujurpc.NewConn(jsoncodec.NewNet(c1), nil)
	rpc2 := jujurpc.NewConn(jsoncodec.NewNet(c2), nil)
	ctx := context.Background()
	rpc1.Start(ctx)
	rpc2.Start(ctx)
	return rpc1, rpc2
}

type AddRequest struct {
	A, B int
}

type AddResult struct {
	Sum int
}

func add(ctx context.Context, req AddRequest) (AddResult, error) {
	return AddResult{
		Sum: req.A + req.B,
	}, nil
}
