// Copyright 2016 Canonical Ltd.

package jujuapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmhttp"
	"github.com/CanonicalLtd/jimm/internal/jimmjwx"
	"github.com/CanonicalLtd/jimm/internal/jujuclient"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	ofgaNames "github.com/CanonicalLtd/jimm/internal/openfga/names"
	jimmRPC "github.com/CanonicalLtd/jimm/internal/rpc"
	"github.com/CanonicalLtd/jimm/internal/servermon"
)

const (
	requestTimeout        = 1 * time.Minute
	maxRequestConcurrency = 10
	pingTimeout           = 90 * time.Second
)

// A root is an rpc.Root enhanced so that it can notify on ping requests.
type root interface {
	rpc.Root
	setPingF(func())
}

// An apiServer is a jimmhttp.WSServer that serves the controller API.
type apiServer struct {
	jimm    *jimm.JIMM
	cleanup func()
	params  Params
}

// ServeWS implements jimmhttp.WSServer.
func (s apiServer) ServeWS(_ context.Context, conn *websocket.Conn) {
	controllerRoot := newControllerRoot(s.jimm, s.params)
	s.cleanup = controllerRoot.cleanup
	serveRoot(context.Background(), controllerRoot, conn)
}

// Kill implements the rpc.Killer interface.
func (s *apiServer) Kill() {
	if s.cleanup != nil {
		s.cleanup()
	}
}

type modelAPIServer struct {
	jimm *jimm.JIMM
}

// ServeWS implements jimmhttp.WSServer.
func (s modelAPIServer) ServeWS(ctx context.Context, conn *websocket.Conn) {
	uuid := jimmhttp.PathElementFromContext(ctx, "uuid")
	ctx = zapctx.WithFields(context.Background(), zap.String("model-uuid", uuid))
	root := newModelRoot(s.jimm, uuid)
	serveRoot(ctx, root, conn)
}

// serveRoot serves an RPC root object on a websocket connection.
func serveRoot(ctx context.Context, root root, wsConn *websocket.Conn) {
	ctx = zapctx.WithFields(ctx, zap.Bool("websocket", true))

	conn := rpc.NewConn(
		jsoncodec.NewWebsocket(wsConn),
		func() rpc.Recorder {
			return recorder{
				start: time.Now(),
			}
		},
	)
	conn.ServeRoot(root, nil, func(err error) error {
		return mapError(err)
	})
	defer conn.Close()
	t := time.AfterFunc(pingTimeout, func() {
		zapctx.Info(ctx, "ping timeout, closing connection")
		conn.Close()
	})
	defer t.Stop()
	root.setPingF(func() { t.Reset(pingTimeout) })
	conn.Start(ctx)
	<-conn.Dead()
}

// mapError maps JIMM errors to errors suitable for use with the juju API.
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

// A modelCommandsServer serves the /commands server for a model.
type modelCommandsServer struct {
	jimm *jimm.JIMM
}

// ServeWS implements jimmhttp.WSServer.
func (s modelCommandsServer) ServeWS(ctx context.Context, clientConn *websocket.Conn) {
	uuid := jimmhttp.PathElementFromContext(ctx, "uuid")
	m := dbmodel.Model{
		UUID: sql.NullString{
			String: uuid,
			Valid:  uuid != "",
		},
	}
	sendClientError := func(err error) {
		msg := jujuparams.CLICommandStatus{
			Done:  true,
			Error: mapError(err),
		}
		if err := clientConn.WriteJSON(msg); err != nil {
			zapctx.Error(ctx, "cannot send commands response", zap.Error(err))
		}
	}
	if err := s.jimm.Database.GetModel(context.Background(), &m); err != nil {
		sendClientError(err)
		return
	}
	mt := names.NewModelTag(uuid)
	controllerConn, err := jujuclient.ProxyDial(ctx, &m.Controller, mt)
	if err != nil {
		zapctx.Error(ctx, "cannot dial controller", zap.String("controller", m.Controller.Name), zap.Error(err))
		sendClientError(err)
		return
	}
	defer controllerConn.Close()
	var user *openfga.User
	var authErr error
	var accessMapCache map[string]string
	var once sync.Once
	var callCount int
	// Note: The authFunc could be generalised as a callback that receives every request/response msg
	// and then decides what to do with it. Currently it acts only as a callback when auth info is needed.
	authFunc := func(req *jujuparams.LoginRequest, errMap map[string]interface{}) ([]byte, error) {
		// Authorize the user and ensure certain checks are only done once.
		// TODO(Kian): Do an authenticate check every time a login request comes through.
		once.Do(func() {
			if req == nil {
				errors.E("Missing login request.")
			}
			user, authErr = s.jimm.Authenticator.Authenticate(ctx, req)
			if authErr != nil {
				return
			}
			var modelAccess string
			modelAccess, authErr = s.jimm.GetUserModelAccess(ctx, user, mt)
			if authErr != nil {
				return
			}
			accessMapCache[mt.String()] = modelAccess
			// Get the user's access to the JIMM controller, because all users have login access to controllers controlled by JIMM
			// but only JIMM admins have admin access on other controllers.
			var controllerAccess string
			controllerAccess, authErr = s.jimm.GetControllerAccess(ctx, user, user.ResourceTag())
			if authErr != nil {
				return
			}
			accessMapCache[m.Controller.Tag().String()] = controllerAccess
		})
		if authErr != nil {
			return nil, authErr
		}
		if errMap != nil {
			var err error
			accessMapCache, err = checkPermission(ctx, user, accessMapCache, errMap)
			if err != nil {
				return nil, err
			}
		}

		jwt, err := s.jimm.JWTService.NewJWT(ctx, jimmjwx.JWTParams{
			Controller: m.Controller.UUID,
			User:       user.Username,
			Access:     accessMapCache,
		})
		if err != nil {
			return nil, err
		}
		callCount++
		if callCount >= 10 {
			return nil, errors.E("Permission check limit exceeded")
		}
		return jwt, nil
	}
	err = jimmRPC.ProxySockets(ctx, clientConn, controllerConn, authFunc)
	sendClientError(err)
}

func checkPermission(ctx context.Context, user *openfga.User, cachedPerms map[string]string, desiredPerms map[string]interface{}) (map[string]string, error) {
	for key, val := range desiredPerms {
		if _, ok := cachedPerms[key]; !ok {
			stringVal, ok := val.(string)
			if !ok {
				return nil, errors.E("Failed to get permission assertion.")
			}
			check, _, err := openfga.CheckRelation(ctx, user, key, ofgaNames.Relation(stringVal))
			if err != nil {
				return cachedPerms, err
			}
			if !check {
				err := errors.E(fmt.Sprintf("Missing permission for %s:%s", key, val))
				return cachedPerms, err
			}
			cachedPerms[key] = stringVal
		}
	}
	return cachedPerms, nil
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
