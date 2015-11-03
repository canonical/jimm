package proxy

import (
	"bytes"
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/utils/cache"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/mgo.v2"
	"launchpad.net/loggo"

	pnet "github.com/CanonicalLtd/jem/cmd/jem-proxy/internal/net"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem-proxy.internal.proxy")

type Proxy struct {
	pool        *jem.Pool
	handlerPool *sync.Pool
	envCache    *cache.Cache
	serverCache *cache.Cache
}

func NewProxy(db *mgo.Database) (*Proxy, error) {
	pool, err := jem.NewPool(
		jem.ServerParams{
			DB: db,
		},
		bakery.NewServiceParams{},
		nil,
	)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	p := &Proxy{
		pool:        pool,
		envCache:    cache.New(24 * time.Hour),
		serverCache: cache.New(24 * time.Hour),
	}
	p.handlerPool = &sync.Pool{
		New: p.newHandler,
	}
	return p, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger.Debugf("<-- %s %s %s", r.Method, r.URL, r.Proto)
	h := p.handler()
	defer h.Close()
	h.ServeHTTP(w, r)
}

func (p *Proxy) handler() *handler {
	j := p.pool.JEM()
	h := p.handlerPool.Get().(*handler)
	h.jem = j
	return h
}

func (p *Proxy) newHandler() interface{} {
	h := &handler{
		proxy: p,
	}
	h.reverseProxy = &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Scheme = "https"
		},
		Transport: &http.Transport{
			Dial:    h.dial,
			DialTLS: h.dialTLS,
		},
	}
	return h
}

type handler struct {
	proxy        *Proxy
	jem          *jem.JEM
	reverseProxy *httputil.ReverseProxy
}

func (h *handler) Close() {
	h.jem.Close()
	h.jem = nil
	h.proxy.handlerPool.Put(h)
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 || parts[1] != "environment" {
		http.NotFound(w, r)
		return
	}
	uuid := parts[2]
	logger.Debugf("UUID: %s", uuid)
	path, err := h.proxy.envCache.Get(uuid, func() (interface{}, error) {
		doc, err := h.jem.EnvironmentFromUUID(uuid)
		if err != nil {
			return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		return doc.StateServer, nil
	})
	if errgo.Cause(err) == params.ErrNotFound {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.Debugf("State Server: %s", path)
	r.URL.Host = path.(params.EntityPath).String()
	if isWebsocketHandshake(r) {
		h.proxyWebsocket(w, r)
	} else {
		h.reverseProxy.ServeHTTP(w, r)
	}
}

func (h *handler) lookupServer(name string) (*mongodoc.StateServer, error) {
	srv, err := h.proxy.serverCache.Get(name, func() (interface{}, error) {
		var path params.EntityPath
		if err := path.UnmarshalText([]byte(name)); err != nil {
			return nil, errgo.Notef(err, "cannot parse name")
		}
		srv, err := h.jem.StateServer(path)
		if err != nil {
			return nil, errgo.Mask(err)
		}
		return srv, nil
	})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return srv.(*mongodoc.StateServer), nil
}

func (h *handler) Lookup(name string) ([]string, error) {
	srv, err := h.lookupServer(name)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return srv.HostPorts, nil
}

func (h *handler) dial(network, address string) (net.Conn, error) {
	return pnet.ParallelDialer{
		Lookuper: h,
	}.Dial(network, address)
}

func (h *handler) dialTLS(network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse address %q", address)
	}
	srv, err := h.lookupServer(host)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	cp, err := api.CreateCertPool(srv.CACert)
	if err != nil {
		return nil, errgo.Mask(err)
	}	
	tlsConfig := &tls.Config{
		ServerName: "juju-apiserver",
		RootCAs:    cp,
	}
	c, err := h.dial(network, address)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	tc := tls.Client(c, tlsConfig)
	if err := tc.Handshake(); err != nil {
		c.Close()
		return nil, errgo.Mask(err)
	}
	return tc, nil
}

func (h *handler) proxyWebsocket(w http.ResponseWriter, r *http.Request) {
	logger.Debugf("proxyWebsocket")
	hj, ok := w.(http.Hijacker)
	if !ok {
		logger.Errorf("Hijack not supported")
		http.Error(w, "cannot proxy websockets", http.StatusInternalServerError)
		return
	}
	s, err := h.dialTLS("tcp", r.URL.Host + ":jujuapi")
	if err != nil {
		logger.Errorf("cannot dial websocket: %s", err)
		http.Error(w, "cannot connect to state-server: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := r.Write(s); err != nil {
		s.Close()
		logger.Errorf("cannot write handshake: %s", err)
		http.Error(w, "cannot connect to state-server: "+err.Error(), http.StatusInternalServerError)
		return
	}
	c, _, err := hj.Hijack()
	if err != nil {
		s.Close()
		logger.Errorf("cannot hijack connection: %s", err)
		http.Error(w, "cannot connect to state-server: "+err.Error(), http.StatusInternalServerError)
		return
	}
	go func() {
		defer c.Close()
		_, err := io.Copy(c, s)
		if err != nil {
			logger.Warningf("error in server -> client connection: %s", err)
		}
	}()
	go func() {
		_, err := io.Copy(s, c)
		if err != nil {
			logger.Warningf("error in client -> server connection: %s", err)
		}
	}()
}

type debugTransport struct {
	transport http.RoundTripper
}

func (d debugTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	logger.Debugf("--> %s %s", r.Method, r.URL.String())
	w := new(bytes.Buffer)
	r.Header.Write(w)
	logger.Debugf("-->\n%s", w.String())
	resp, err := d.transport.RoundTrip(r)
	if err != nil {
		logger.Debugf("*** %s", err)
		return resp, err
	}
	logger.Debugf("<-- %s", resp.Status)
	if resp.StatusCode >= 400 {
		body, _ := ioutil.ReadAll(resp.Body)
		if body != nil {
			logger.Debugf("<--\n%s", body)
		}
	}
	return resp, err
}

func isWebsocketHandshake(r *http.Request) bool {
	return r.Method == "GET" &&
		strings.EqualFold(r.Header.Get("Connection"), "Upgrade") &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}
