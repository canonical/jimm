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
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"

	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jemserver"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/servermon"
)

const (
	requestTimeout        = 10 * time.Second
	maxRequestConcurrency = 10
)

// mapError maps JEM errors to errors suitable for use with the juju API.
func mapError(err error) *jujuparams.Error {
	if err == nil {
		return nil
	}
	// TODO the error mapper should really accept a context from the RPC package.
	zapctx.Debug(context.TODO(), "rpc error", zaputil.Error(err))

	return &jujuparams.Error{
		Message: err.Error(),
		Code:    string(errors.ErrorCode(err)),
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
func newWSServer(jimm *jimm.JIMM, jsParams jemserver.Params, uuid string) http.Handler {
	hnd := &wsHandler{
		jimm:      jimm,
		params:    jsParams,
		modelUUID: uuid,
	}
	h := func(w http.ResponseWriter, req *http.Request) {
		conn, err := websocketUpgrader.Upgrade(w, req, nil)
		if err != nil {
			zapctx.Error(req.Context(), "cannot upgrade websocket", zaputil.Error(err))
			return
		}
		hnd.handle(req.Context(), conn)
	}
	return http.HandlerFunc(h)
}

// wsHandler is a handler for a particular WebSocket connection.
type wsHandler struct {
	jimm      *jimm.JIMM
	params    jemserver.Params
	modelUUID string
}

// handle handles the connection.
func (h *wsHandler) handle(ctx context.Context, wsConn *websocket.Conn) {
	codec := jsoncodec.NewWebsocket(wsConn)
	conn := rpc.NewConn(codec, func() rpc.Recorder {
		return recorder{
			start: time.Now(),
		}
	})
	hm := newHeartMonitor(h.params.WebsocketRequestTimeout)
	var root rpc.Root
	if h.modelUUID == "" {
		root = newControllerRoot(h.jimm, h.params, hm)
	} else {
		root = newModelRoot(h.jimm, hm, h.modelUUID)
	}
	defer root.Kill()
	conn.ServeRoot(root, nil, func(err error) error {
		return mapError(err)
	})
	defer conn.Close()
	conn.Start(ctx)
	select {
	case <-hm.Dead():
		zapctx.Info(ctx, "ping timeout")
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
