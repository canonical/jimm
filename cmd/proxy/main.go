package main

import (
	"context"
	"crypto/tls"
	"path"
	"sync"
	"syscall"

	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	service "github.com/canonical/go-service"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	jujuhttp "github.com/juju/http"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimmjwx"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/wellknownapi"
)

const websocketFrameSize = 65536

type config struct {
	UUID            string     `yaml:"uuid"`
	Hostname        string     `yaml:"hostname"`
	CertificateFile string     `yaml:"cert-file"`
	KeyFile         string     `yaml:"key-file"`
	Controller      controller `yaml:"controller"`
}

type controller struct {
	UUID          string   `yaml:"uuid"`
	APIEndpoints  []string `yaml:"api-endpoints"`
	CACertificate string   `yaml:"ca-cert"`
}

type loginRequest struct {
	jujuparams.LoginRequest

	Token string `json:"token"`
}

// A message encodes a single message received, over an RPC
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

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	// In order to deal with the remote side not handling message
	// fragmentation, we default to largeish frames.
	ReadBufferSize:  websocketFrameSize,
	WriteBufferSize: websocketFrameSize,
}

type wsServer interface {
	ServeWS(context.Context, string, *websocket.Conn, *websocket.Conn)
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
	modelUUID := chi.URLParam(req, "modelUUID")

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
		ws.server.ServeWS(ctx, modelUUID, connClient, connController)
	}()
}

type wsMITM struct {
	mimtUUID       string
	controllerUUID string
	hostname       string
	jwtService     *jimmjwx.JWTService

	mu           sync.Mutex
	loginMessage *message
}

func (ws *wsMITM) mintJWT(ctx context.Context, accessMap map[string]string) (string, error) {
	jwt, err := ws.jwtService.NewJWT(ctx, jimmjwx.JWTParams{
		Controller: ws.controllerUUID,
		User:       "user-fred@external",
		Access:     accessMap,
	})
	if err != nil {
		return "", errors.E(err, "failed to create a JWT")
	}
	return base64.StdEncoding.EncodeToString(jwt), nil
}

func isLoginRequest(msg *message) bool {
	return msg.Type == "Admin" && msg.Request == "Login"
}

func (ws *wsMITM) doLogin(ctx context.Context, request *message, accessMap map[string]string, connController *websocket.Conn) (*message, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.loginMessage = request

	// First we unmarshal the existing LoginRequest.
	var lReq jujuparams.LoginRequest
	if err := json.Unmarshal(request.Params, &lReq); err != nil {
		zapctx.Error(ctx, "failed to unmarshal the login request")
		return nil, errors.E(err)
	}

	jwtString, err := ws.mintJWT(ctx, accessMap)
	if err != nil {
		zapctx.Error(ctx, "failed to mint a JWT")
		return nil, errors.E(err)
	}

	// Add the JWT as base64 encoded string.
	loginRequest := loginRequest{
		LoginRequest: lReq,
		Token:        jwtString,
	}
	// Marshal it again to JSON.
	data, err := json.Marshal(loginRequest)
	if err != nil {
		zapctx.Error(ctx, "failed to marshal the login request")
		return nil, errors.E(err, "failed to marshal login request data")
	}
	// And add it to the message.
	request.Params = data

	zapctx.Error(ctx, "forwarding login request", zap.Any("request", request))
	err = connController.WriteJSON(request)
	if err != nil {
		zapctx.Error(ctx, "failed to forward login request", zap.Error(err))
		return nil, errors.E(err)
	}
	response := new(message)
	if err := connController.ReadJSON(response); err != nil {
		zapctx.Error(ctx, "failed to read JSON response", zap.Error(err))
		return nil, errors.E(err, "failed to read response")
	}

	// First we unmarshal the existing LoginRequest.
	var lResp jujuparams.LoginResult
	if err := json.Unmarshal(response.Response, &lResp); err != nil {
		zapctx.Error(ctx, "failed to unmarshal login result", zap.Any("message", response), zap.Error(err))
		return nil, errors.E(err, "failed to unmarshal login result")
	}

	lResp.PublicDNSName = ws.hostname
	lResp.Servers = nil
	lResp.ControllerTag = names.NewControllerTag(ws.mimtUUID).String()

	// Marshal it again to JSON.
	data, err = json.Marshal(lResp)
	if err != nil {
		zapctx.Error(ctx, "failed to marshal login result", zap.Error(err))
		return nil, errors.E(err, "failed to marshal login result")
	}
	response.Response = data

	return response, nil
}

func (ws *wsMITM) handleMessage(ctx context.Context, request *message, accessMap map[string]string, connController *websocket.Conn) (*message, error) {
	if request.RequestID == 0 {
		// Use a 0 request ID to indicate that the message
		// received was not a valid RPC message.
		return nil, errors.E("received invalid RPC message")
	}
	if !request.isRequest() {
		zapctx.Error(ctx, "received response", zap.Any("message", request))
		return nil, errors.E("received response from client")
	}

	// If this is a login request we need to augment it with the
	// JWT.
	if isLoginRequest(request) {
		response, err := ws.doLogin(ctx, request, accessMap, connController)
		if err != nil {
			zapctx.Error(ctx, "failed to perform login", zap.Error(err))
			return nil, errors.E(err, "failed to forward the login request")
		}
		return response, nil
	}

	zapctx.Error(ctx, "forwarding request", zap.Any("request", request))
	err := connController.WriteJSON(request)
	if err != nil {
		zapctx.Error(ctx, "failed to forward request", zap.Error(err))
		return nil, errors.E(err)
	}
	response := new(message)
	if err := connController.ReadJSON(response); err != nil {
		zapctx.Error(ctx, "failed to read JSON response", zap.Error(err))
		return nil, errors.E(err, "failed to read response")
	}

	zapctx.Error(ctx, "received response", zap.Any("response", response))

	var er jujuparams.ErrorResults
	err = json.Unmarshal(response.Response, &er)
	if err != nil {
		zapctx.Error(ctx, "failed to read response error")
		return nil, errors.E(err, "failed to read response errors")
	}
	requiredPermissions := make(map[string]string)
	for _, e := range er.Results {
		zapctx.Error(ctx, "received error", zap.Any("error", e))
		if e.Error != nil && e.Error.Code == "access required" {
			for k, v := range e.Error.Info {
				accessLevel, ok := v.(string)
				if !ok {
					return nil, errors.E("unknown permission level")
				}
				requiredPermissions[k] = accessLevel
			}
		}
	}
	if len(requiredPermissions) > 0 {
		zapctx.Error(ctx, "XXX additional permissions", zap.Any("permissions", requiredPermissions))
		for k, v := range requiredPermissions {
			accessMap[k] = v
		}

		if ws.loginMessage == nil {
			zapctx.Error(ctx, "need to re-login, but login was never called")
			return nil, errors.E("need to re-login, but login was never called")
		}
		if _, err := ws.doLogin(ctx, ws.loginMessage, accessMap, connController); err != nil {
			zapctx.Error(ctx, "re-login failed", zap.Error(err))
			return nil, errors.E("re-login failed")
		}
		return ws.handleMessage(ctx, request, accessMap, connController)
	}

	zapctx.Info(ctx, "received controller response", zap.Any("message", response))
	return response, nil
}

func (ws *wsMITM) ServeWS(ctx context.Context, modelUUID string, connClient, connController *websocket.Conn) {
	accessMap := map[string]string{
		names.NewControllerTag(ws.controllerUUID).String(): "superuser",
	}
	if modelUUID != "" {
		accessMap[names.NewModelTag(modelUUID).String()] = "admin"
	}

	for {
		request := new(message)
		if err := connClient.ReadJSON(request); err != nil {
			ws.handleError(err, connClient)
			break
		}

		response, err := ws.handleMessage(ctx, request, accessMap, connController)
		if err != nil {
			ws.handleError(errors.E("received invalid RPC message"), connClient)
			break
		}

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

type handlerParams struct {
	hostname       string
	mitmUUID       string
	controllerUUID string
	dialFunc       func(ctx context.Context, requestPath string, header *http.Header) (*websocket.Conn, *http.Response, error)
	jwtService     *jimmjwx.JWTService
}

func newHandler(p handlerParams) http.Handler {
	return &wsHandler{
		dialFunc: p.dialFunc,
		server: &wsMITM{
			hostname:       p.hostname,
			mimtUUID:       p.mitmUUID,
			controllerUUID: p.controllerUUID,
			jwtService:     p.jwtService,
		},
	}
}

func start(ctx context.Context, s *service.Service) error {
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

	controllerURL := config.Controller.APIEndpoints[0]

	tlsConfig := jujuhttp.SecureTLSConfig()
	if config.Controller.CACertificate != "" {
		cp := x509.NewCertPool()
		cp.AppendCertsFromPEM([]byte(config.Controller.CACertificate))
		tlsConfig.RootCAs = cp
		tlsConfig.ServerName = "juju-apiserver"
	}
	dialer := websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}
	dialFunc := func(ctx context.Context, requestPath string, header *http.Header) (*websocket.Conn, *http.Response, error) {
		zapctx.Info(ctx, "dialing", zap.String("url", "wss://"+path.Join(controllerURL, requestPath)))
		return dialer.DialContext(ctx, "wss://"+path.Join(controllerURL, requestPath), nil)
	}

	mux := chi.NewMux()

	st := &jimmtest.InMemoryCredentialStore{}
	jwtService := jimmjwx.NewJWTService(config.Hostname, st, true)
	jwksService := jimmjwx.NewJWKSService(st)
	s.Go(func() error {
		return jwksService.StartJWKSRotator(ctx, time.NewTicker(time.Hour).C, time.Now().UTC().AddDate(0, 3, 0))
	})

	mux.Handle("/api", newHandler(handlerParams{
		mitmUUID:       config.UUID,
		controllerUUID: config.Controller.UUID,
		hostname:       config.Hostname,
		dialFunc:       dialFunc,
		jwtService:     jwtService,
	}))
	mux.Handle("/model/{modelUUID}/*", newHandler(handlerParams{
		mitmUUID:       config.UUID,
		controllerUUID: config.Controller.UUID,
		hostname:       config.Hostname,
		dialFunc:       dialFunc,
		jwtService:     jwtService,
	}))
	mux.Mount("/.well-known", wellknownapi.NewWellKnownHandler(st).Routes())
	mux.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		zapctx.Error(req.Context(), "cannot handle request", zap.String("path", req.URL.String()))
		w.WriteHeader(http.StatusNotFound)
	}))

	serverCert, err := tls.LoadX509KeyPair(config.CertificateFile, config.KeyFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}

	tlscfg := jujuhttp.SecureTLSConfig()
	tlscfg.Certificates = []tls.Certificate{serverCert}
	httpsrv := &http.Server{
		Addr:      ":443",
		Handler:   mux,
		TLSConfig: tlscfg,
	}
	s.OnShutdown(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpsrv.Shutdown(ctx)
	})
	s.Go(func() error {
		err := httpsrv.ListenAndServeTLS("", "")
		fmt.Printf("Listen and serve error: %s\n", err.Error())
		return err
	})
	fmt.Println("started")

	tlsConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 15 * time.Second,
	}
	jwtService.RegisterJWKSCache(ctx, client)

	return nil
}

func main() {
	if err := os.Setenv("JIMM_JWT_EXPIRY", "24h"); err != nil {

		os.Exit(1)
	}

	ctx, s := service.NewService(context.Background(), os.Interrupt, syscall.SIGTERM)

	if err := zapctx.LogLevel.UnmarshalText([]byte("debug")); err != nil {
		zapctx.Error(ctx, "cannot set log level", zap.Error(err))
	}

	s.Go(func() error {
		return start(ctx, s)
	})
	err := s.Wait()
	zapctx.Error(context.Background(), "shutdown", zap.Error(err))
	if _, ok := err.(*service.SignalError); !ok {
		os.Exit(1)
	}
}
