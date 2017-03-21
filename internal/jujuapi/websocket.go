// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"time"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"golang.org/x/net/context"
	"golang.org/x/net/websocket"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/jemserver"
	"github.com/CanonicalLtd/jem/internal/servermon"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
	"github.com/CanonicalLtd/jem/params"
)

const (
	requestTimeout        = 10 * time.Second
	maxRequestConcurrency = 10
)

var errorCodes = map[error]string{
	params.ErrAlreadyExists:    jujuparams.CodeAlreadyExists,
	params.ErrBadRequest:       jujuparams.CodeBadRequest,
	params.ErrForbidden:        jujuparams.CodeForbidden,
	params.ErrMethodNotAllowed: jujuparams.CodeMethodNotAllowed,
	params.ErrNotFound:         jujuparams.CodeNotFound,
	params.ErrUnauthorized:     jujuparams.CodeUnauthorized,
}

// mapError maps JEM errors to errors suitable for use with the juju API.
func mapError(err error) *jujuparams.Error {
	if err == nil {
		return nil
	}
	// TODO the error mapper should really accept a context from the RPC package.
	zapctx.Debug(context.TODO(), "rpc error", zaputil.Error(err))
	if perr, ok := err.(*jujuparams.Error); ok {
		return perr
	}
	return &jujuparams.Error{
		Message: err.Error(),
		Code:    errorCodes[errgo.Cause(err)],
	}
}

// heartMonitor is a interface that will monitor a connection and fail it
// if a heartbeat is not received within a certain time.
type heartMonitor interface {
	// Heartbeat signals to the HeartMonitor that the connection is still alive.
	Heartbeat()

	// Dead returns a channel that will be signalled if the heartbeat
	// is not detected quickly enough.
	Dead() <-chan time.Time

	// Stop stops the HeartMonitor from monitoring. It return true if
	// the connection is already dead when Stop was called.
	Stop() bool
}

// timerHeartMonitor implements heartMonitor using a standard time.Timer.
type timerHeartMonitor struct {
	*time.Timer
	duration time.Duration
}

// Heartbeat implements HeartMonitor.Heartbeat.
func (h timerHeartMonitor) Heartbeat() {
	h.Timer.Reset(h.duration)
}

// Dead implements HeartMonitor.Dead.
func (h timerHeartMonitor) Dead() <-chan time.Time {
	return h.Timer.C
}

// newHeartMonitor is defined as a variable so that it can be overriden in tests.
var newHeartMonitor = func(d time.Duration) heartMonitor {
	return timerHeartMonitor{
		Timer:    time.NewTimer(d),
		duration: d,
	}
}

// newWSServer creates a new WebSocket server suitible for handling the API for modelUUID.
func newWSServer(ctx context.Context, jem *jem.JEM, ap *auth.Pool, jsParams jemserver.Params, modelUUID string) websocket.Server {
	hnd := &wsHandler{
		context:   ctx,
		jem:       jem,
		authPool:  ap,
		params:    jsParams,
		modelUUID: modelUUID,
	}
	return websocket.Server{
		Handler: hnd.handle,
	}
}

// wsHandler is a handler for a particular WebSocket connection.
type wsHandler struct {
	// TODO Make the context per-RPC-call instead of global across the handler.
	context   context.Context
	jem       *jem.JEM
	authPool  *auth.Pool
	params    jemserver.Params
	modelUUID string
}

// handle handles the connection.
func (h *wsHandler) handle(wsConn *websocket.Conn) {
	codec := jsoncodec.NewWebsocket(wsConn)
	conn := rpc.NewConn(codec, observerFactory{})
	hm := newHeartMonitor(h.params.WebsocketRequestTimeout)
	var root rpc.Root
	if h.modelUUID == "" {
		root = newControllerRoot(h.context, h.jem, h.authPool, h.params, hm)
	} else {
		root = newModelRoot(h.context, h.jem, hm, h.modelUUID)
	}
	defer root.Kill()
	conn.ServeRoot(root, func(err error) error {
		return mapError(err)
	})
	defer conn.Close()
	conn.Start()
	select {
	case <-hm.Dead():
		zapctx.Info(h.context, "ping timeout")
	case <-conn.Dead():
		hm.Stop()
	}
}

// observerFactory implemnts an rpc.ObserverFactory.
type observerFactory struct{}

// RPCObserver implements rpc.ObserverFactory.RPCObserver.
func (observerFactory) RPCObserver() rpc.Observer {
	return observer{
		start: time.Now(),
	}
}

// observer implements an rpc.Observer.
type observer struct {
	start time.Time
}

// ServerRequest implements rpc.Observer.ServerRequest.
func (o observer) ServerRequest(*rpc.Header, interface{}) {
}

// ServerReply implements rpc.Observer.ServerReply.
func (o observer) ServerReply(r rpc.Request, _ *rpc.Header, _ interface{}) {
	d := time.Since(o.start)
	servermon.WebsocketRequestDuration.WithLabelValues(r.Type, r.Action).Observe(float64(d) / float64(time.Second))
}
