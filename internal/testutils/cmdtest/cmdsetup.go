// Copyright 2024 Canonical.
package cmdtest

import (
	"context"
	"crypto/tls"
	"strings"

	"github.com/gorilla/websocket"
	jujuapi "github.com/juju/juju/api"
	"github.com/juju/juju/rpc/jsoncodec"
)

func TestDialOpts(lp jujuapi.LoginProvider) *jujuapi.DialOpts {
	return &jujuapi.DialOpts{
		LoginProvider: lp,
		DialWebsocket: getDialWebsocketWithInsecureUrl(),
	}
}

// getDialWebsocketWithInsecureUrl forces the URL used for dialing to use insecure websockets
// so that tests don't need to start an HTTPS server and manage certs.
func getDialWebsocketWithInsecureUrl() func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error) {
	// Modified from github.com/juju/juju@v0.0.0-20240304110523-55fb5d03683b/api/apiclient.go gorillaDialWebsocket

	dialWebsocket := func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error) {
		urlStr = strings.Replace(urlStr, "wss", "ws", 1)
		dialer := &websocket.Dialer{}
		c, resp, err := dialer.Dial(urlStr, nil)
		defer resp.Body.Close()
		if err != nil {
			return nil, err
		}
		return jsoncodec.NewWebsocketConn(c), nil
	}
	return dialWebsocket
}
