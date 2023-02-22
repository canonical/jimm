package main

import (
	"context"
	"crypto/tls"
	"path"

	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	jujuhttp "github.com/juju/http"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/CanonicalLtd/jimm/internal/errors"
)

const websocketFrameSize = 65536

type config struct {
	CACertificateFile string     `yaml:"ca-cert-file"`
	CertificateFile   string     `yaml:"cert-file"`
	KeyFile           string     `yaml:"key-file"`
	Controller        controller `yaml:"controller"`
}

type controller struct {
	APIEndpoints  []string `yaml:"api-endpoints"`
	CACertificate string   `yaml:"ca-cert"`
}

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	// In order to deal with the remote side not handling message
	// fragmentation, we default to largeish frames.
	ReadBufferSize:  websocketFrameSize,
	WriteBufferSize: websocketFrameSize,
}

type wsServer interface {
	ServeWS(context.Context, *websocket.Conn, *websocket.Conn)
}

type wsHandler struct {
	dialFunc func(ctx context.Context, requestPath string, header *http.Header) (*websocket.Conn, *http.Response, error)
	server   wsServer
}

// ServeHTTP implements http.Handler by upgrading the HTTP request to a
// websocket connection and running Server.ServeWS with the upgraded
// connection. ServeHTTP returns as soon as the websocket connection has
// been started.
func (ws *wsHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	connClient, err := websocketUpgrader.Upgrade(w, req, nil)
	if err != nil {
		// If the upgrader returns an error it will have written an
		// error response, so there is no need to do so here.
		zapctx.Error(ctx, "cannot upgrade websocket", zap.Error(err))
		return
	}

	connController, _, err := ws.dialFunc(req.Context(), req.URL.Path, &req.Header)
	if err != nil {
		zapctx.Error(ctx, "cannot dial controller", zap.Error(err))
		return
	}

	go func() {
		defer connClient.Close()
		defer func() {
			zapctx.Info(ctx, "closing controller connection")
			connController.Close()
		}()
		defer func() {
			if err := recover(); err != nil {
				zapctx.Error(ctx, "websocket panic", zap.Any("err", err), zap.Stack("stack"))
				data := websocket.FormatCloseMessage(websocket.CloseInternalServerErr, fmt.Sprintf("%v", err))
				if err := connClient.WriteControl(websocket.CloseMessage, data, time.Time{}); err != nil {
					zapctx.Error(ctx, "cannot write close message", zap.Error(err))
				}
			}
		}()
		if ws.server == nil {
			data := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
			if err := connClient.WriteControl(websocket.CloseMessage, data, time.Time{}); err != nil {
				zapctx.Error(ctx, "cannot write close message", zap.Error(err))
			}
			return
		}
		ws.server.ServeWS(ctx, connClient, connController)
	}()
}

// A message encodes a single message sent, or received, over an RPC
// connection. It contains the union of fields in a request or response
// message.
type message struct {
	RequestID uint64                 `json:"request-id,omitempty"`
	Type      string                 `json:"type,omitempty"`
	Version   int                    `json:"version,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Request   string                 `json:"request,omitempty"`
	Params    json.RawMessage        `json:"params,omitempty"`
	Error     string                 `json:"error,omitempty"`
	ErrorCode string                 `json:"error-code,omitempty"`
	ErrorInfo map[string]interface{} `json:"error-info,omitempty"`
	Response  json.RawMessage        `json:"response,omitempty"`
}

// isRequest returns whether the message is a request
func (m message) isRequest() bool {
	return m.Type != "" && m.Request != ""
}

type wsMITM struct{}

func (ws *wsMITM) ServeWS(ctx context.Context, connClient, connController *websocket.Conn) {
	for {
		msg := new(message)
		if err := connClient.ReadJSON(msg); err != nil {
			ws.handleError(err, connClient)
			break
		}
		if msg.RequestID == 0 {
			// Use a 0 request ID to indicate that the message
			// received was not a valid RPC message.
			ws.handleError(errors.E("received invalid RPC message"), connClient)
			break
		}
		if !msg.isRequest() {
			// we received a response from the client which is not
			// supported
			zapctx.Error(ctx, "received response", zap.Any("message", msg))
			connClient.WriteJSON(message{
				RequestID: msg.RequestID,
				Error:     "not supported",
				ErrorCode: jujuparams.CodeNotSupported,
			})
			continue
		}
		zapctx.Info(ctx, "forwarding request", zap.Any("message", msg))
		err := connController.WriteJSON(msg)
		if err != nil {
			zapctx.Error(ctx, "cannot forward request", zap.Error(err))
			ws.handleError(err, connClient)
			break
		}
		response := new(message)
		if err := connController.ReadJSON(response); err != nil {
			ws.handleError(err, connClient)
			break
		}
		zapctx.Info(ctx, "received controller response", zap.Any("message", response))
		connClient.WriteJSON(response)
		if err != nil {
			zapctx.Error(ctx, "cannot return response", zap.Error(err))
			ws.handleError(err, connClient)
			break
		}
	}
}

func (ws *wsMITM) handleError(err error, conn *websocket.Conn) {
	// We haven't sent a close message yet, so try to send one.
	cm := websocket.FormatCloseMessage(websocket.CloseProtocolError, err.Error())
	conn.WriteControl(websocket.CloseMessage, cm, time.Time{})
	conn.Close()
}

// MITM service
func newService(ctx context.Context, controllerURL string, controllerTLS *tls.Config) *service {
	dialer := websocket.Dialer{
		TLSClientConfig: controllerTLS,
	}
	s := &service{
		mux: chi.NewMux(),
		dialFunc: func(ctx context.Context, requestPath string, header *http.Header) (*websocket.Conn, *http.Response, error) {
			zapctx.Info(ctx, "dialing", zap.String("url", "wss://"+path.Join(controllerURL, requestPath)))
			return dialer.DialContext(ctx, "wss://"+path.Join(controllerURL, requestPath), nil)
		},
	}

	s.mux.Handle("/api", s.newAPIHandler(ctx))
	s.mux.Handle("/model/*", s.newModelHandler(ctx))

	return s
}

type service struct {
	mux      *chi.Mux
	dialFunc func(context.Context, string, *http.Header) (*websocket.Conn, *http.Response, error)
}

// ServeHTTP implements http.Handler.
func (s *service) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.mux.ServeHTTP(w, req)
}

func (s *service) newAPIHandler(ctx context.Context) http.Handler {
	return &wsHandler{
		dialFunc: s.dialFunc,
		server:   &wsMITM{},
	}
}

func (s *service) newModelHandler(ctx context.Context) http.Handler {
	return &wsHandler{
		dialFunc: s.dialFunc,
		server:   &wsMITM{},
	}
}
func main() {
	fmt.Println("started")
	args := os.Args[1:]
	if len(args) != 1 {
		fmt.Println("./mitm <config file>")
		os.Exit(-1)
	}

	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Printf("cannot read the config file %s: %v\n", args[0], err)
		os.Exit(-1)
	}
	var config config
	if err = yaml.Unmarshal(data, &config); err != nil {
		fmt.Printf("cannot unmarshal the config file %s: %v\n", args[0], err)
		os.Exit(-1)
	}

	tlsConfig := jujuhttp.SecureTLSConfig()
	if config.Controller.CACertificate != "" {
		cp := x509.NewCertPool()
		cp.AppendCertsFromPEM([]byte(config.Controller.CACertificate))
		tlsConfig.RootCAs = cp
		tlsConfig.ServerName = "juju-apiserver"
	}
	s := newService(context.Background(), config.Controller.APIEndpoints[0], tlsConfig)

	err = http.ListenAndServeTLS(":17071", config.CertificateFile, config.KeyFile, s)
	if err != nil {
		fmt.Printf("cannot listen to port 17071: %v\n", err)
		os.Exit(-1)
	}
	fmt.Println("exiting")
}
