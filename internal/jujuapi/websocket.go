// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
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
	params.ErrModelNotFound:    jujuparams.CodeModelNotFound,
	params.ErrUnauthorized:     jujuparams.CodeUnauthorized,
}

// mapError maps JEM errors to errors suitable for use with the juju API.
func mapError(err error) *jujuparams.Error {
	if err == nil {
		return nil
	}
	// TODO the error mapper should really accept a context from the RPC package.
	zapctx.Debug(context.TODO(), "rpc error", zaputil.Error(err))
	if perr, ok := errgo.Cause(err).(*jujuparams.Error); ok {
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

// Use a 64k frame size for the websockets while we need to deal
// with x/net/websocket connections that don't deal with recieving
// fragmented messages.
const websocketFrameSize = 65536

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	// In order to deal with the remote side not handling message
	// fragmentation, we default to largeish frames.
	ReadBufferSize:  websocketFrameSize,
	WriteBufferSize: websocketFrameSize,
}

// newWSServer creates a new WebSocket server suitible for handling the API for modelUUID.
func newWSServer(ctx context.Context, jem *jem.JEM, ap *auth.Pool, jsParams jemserver.Params, modelUUID string) http.Handler {
	hnd := &wsHandler{
		context:   ctx,
		jem:       jem,
		authPool:  ap,
		params:    jsParams,
		modelUUID: modelUUID,
	}
	h := func(w http.ResponseWriter, req *http.Request) {
		conn, err := websocketUpgrader.Upgrade(w, req, nil)
		if err != nil {
			zapctx.Error(ctx, "cannot upgrade websocket", zaputil.Error(err))
			return
		}
		hnd.handle(conn)
	}
	return http.HandlerFunc(h)
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
	conn := rpc.NewConn(codec, func() rpc.Recorder {
		return recorder{
			start: time.Now(),
		}
	})
	hm := newHeartMonitor(h.params.WebsocketRequestTimeout)
	var root rpc.Root
	if h.modelUUID == "" {
		root = newControllerRoot(h.context, h.jem, h.authPool, h.params, hm)
	} else {
		root = newModelRoot(h.context, h.jem, hm, h.modelUUID)
	}
	defer root.Kill()
	conn.ServeRoot(root, nil, func(err error) error {
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

// recorder implements an rpc.Recorder.
type recorder struct {
	start time.Time
}

// HandleRequest implements rpc.Recorder.
func (recorder) HandleRequest(*rpc.Header, interface{}) error {
	return nil
}

// HandleReply implements rpc.Recorder.
func (o recorder) HandleReply(r rpc.Request, _ *rpc.Header, _ interface{}) error {
	d := time.Since(o.start)
	servermon.WebsocketRequestDuration.WithLabelValues(r.Type, r.Action).Observe(float64(d) / float64(time.Second))
	return nil
}
