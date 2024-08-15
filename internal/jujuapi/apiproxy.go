// Copyright 2024 Canonical.
package jujuapi

import (
	"context"
	"database/sql"
	"regexp"

	"github.com/gorilla/websocket"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/auth"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	jimmRPC "github.com/canonical/jimm/v3/internal/rpc"
)

// apiProxier serves the /commands and /api server for a model by
// proxying all requests through to the controller.
type apiProxier struct {
	apiServer
}

var (
	extractPathInfo = regexp.MustCompile(`^\/(?P<modeluuid>\w{8}-\w{4}-\w{4}-\w{4}-\w{12})\/(?P<finalPath>.*)$`)
	modelIndex      = mustGetSubexpIndex(extractPathInfo, "modeluuid")
	finalPathIndex  = mustGetSubexpIndex(extractPathInfo, "finalPath")
)

func mustGetSubexpIndex(regex *regexp.Regexp, name string) int {
	index := regex.SubexpIndex(name)
	if index == -1 {
		panic("failed to find subexp index")
	}
	return index
}

// modelInfoFromPath takes a URL path to a model endpoint and returns the uuid
// and final URL segment. I.e. /model/<uuid>/api returns <uuid>, api, err
// Basic validation of the uuid takes place.
func modelInfoFromPath(path string) (uuid string, finalPath string, err error) {
	matches := extractPathInfo.FindStringSubmatch(path)
	if len(matches) != 3 {
		return "", "", errors.E("invalid path")
	}
	return matches[modelIndex], matches[finalPathIndex], nil
}

// ServeWS implements jimmhttp.WSServer.
// It does so by acting as a websocket proxy that intercepts auth requests
// to authenticate the user and create a token with their permissions before
// forwarding their requests to the appropriate Juju controller.
func (s apiProxier) ServeWS(ctx context.Context, clientConn *websocket.Conn) {
	jwtGenerator := jimm.NewJWTGenerator(&s.jimm.Database, s.jimm, s.jimm.JWTService)
	connectionFunc := controllerConnectionFunc(s, &jwtGenerator)
	zapctx.Debug(ctx, "Starting proxier")
	auditLogger := s.jimm.AddAuditLogEntry
	proxyHelpers := jimmRPC.ProxyHelpers{
		ConnClient:              clientConn,
		TokenGen:                &jwtGenerator,
		ConnectController:       connectionFunc,
		AuditLog:                auditLogger,
		JIMM:                    s.jimm,
		AuthenticatedIdentityID: auth.SessionIdentityFromContext(ctx),
	}
	jimmRPC.ProxySockets(ctx, proxyHelpers)
}

// controllerConnectionFunc returns a function that will be used to
// connect to a controller when a client makes a request.
func controllerConnectionFunc(s apiProxier, jwtGenerator *jimm.JWTGenerator) func(context.Context) (jimmRPC.WebsocketConnectionWithMetadata, error) {
	return func(ctx context.Context) (jimmRPC.WebsocketConnectionWithMetadata, error) {
		const op = errors.Op("proxy.controllerConnectionFunc")
		path := jimmhttp.PathElementFromContext(ctx, "path")
		zapctx.Debug(ctx, "grabbing model info from path", zap.String("path", path))
		uuid, finalPath, err := modelInfoFromPath(path)
		if err != nil {
			zapctx.Error(ctx, "error parsing path", zap.Error(err))
			return jimmRPC.WebsocketConnectionWithMetadata{}, errors.E(op, err)
		}
		m := dbmodel.Model{
			UUID: sql.NullString{
				String: uuid,
				Valid:  uuid != "",
			},
		}
		if err := s.jimm.Database.GetModel(context.Background(), &m); err != nil {
			zapctx.Error(ctx, "failed to find model", zap.String("uuid", uuid), zap.Error(err))
			return jimmRPC.WebsocketConnectionWithMetadata{}, errors.E(err, errors.CodeNotFound)
		}
		jwtGenerator.SetTags(m.ResourceTag(), m.Controller.ResourceTag())
		mt := m.ResourceTag()
		zapctx.Debug(ctx, "Dialing Controller", zap.String("path", path))
		controllerConn, err := jimmRPC.Dial(ctx, &m.Controller, mt, finalPath, nil)
		if err != nil {
			zapctx.Error(ctx, "cannot dial controller", zap.String("controller", m.Controller.Name), zap.Error(err))
			return jimmRPC.WebsocketConnectionWithMetadata{}, err
		}
		fullModelName := m.Controller.Name + "/" + m.Name
		return jimmRPC.WebsocketConnectionWithMetadata{
			Conn:           controllerConn,
			ControllerUUID: m.Controller.UUID,
			ModelName:      fullModelName,
		}, nil
	}
}
