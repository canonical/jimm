// Copyright 2015 Canonical Ltd.

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/handlers"
	// Include any providers known to support JEM.
	// Avoid including provider/all to reduce build time.
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/lxd"
	_ "github.com/juju/juju/provider/maas"
	_ "github.com/juju/juju/provider/openstack"
	"github.com/juju/loggo"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/mgo.v2"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/CanonicalLtd/jem"
	"github.com/CanonicalLtd/jem/config"
)

// websocketPingTimeout is the amount of time a webseocket connection
// will wait for a ping before failing the connections. It is hardcoded
// in juju so I see no reason why it can't be here also.
const websocketPingTimeout = 3 * time.Minute

var (
	logger        = loggo.GetLogger("jemd")
	loggingConfig = flag.String("logging-config", "", "specify log levels for modules e.g. <root>=TRACE")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [options] <config path>\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
	}
	if *loggingConfig != "" {
		if err := loggo.ConfigureLoggers(*loggingConfig); err != nil {
			fmt.Fprintf(os.Stderr, "cannot configure loggers: %v", err)
			os.Exit(1)
		}
	}
	if err := serve(flag.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func serve(confPath string) error {
	logger.Debugf("reading configuration")
	conf, err := config.Read(confPath)
	if err != nil {
		return errgo.Notef(err, "cannot read config file %q", confPath)
	}
	conf.IdentityLocation = strings.TrimSuffix(conf.IdentityLocation, "/")
	if strings.HasSuffix(conf.IdentityLocation, "v1/discharger") {
		// It's probably some old code that uses the old IdentityLocation
		// meaning.
		return errgo.Notef(err, "identity location must not contain discharge path")
	}

	logger.Debugf("connecting to mongo")
	session, err := mgo.Dial(conf.MongoAddr)
	if err != nil {
		return errgo.Notef(err, "cannot dial mongo at %q", conf.MongoAddr)
	}
	defer session.Close()
	db := session.DB("jem")

	logger.Debugf("setting up the API server")
	var locator bakery.PublicKeyLocator
	if conf.IdentityPublicKey == nil {
		locator = httpbakery.NewPublicKeyRing(nil, nil)
	} else {
		locator = bakery.PublicKeyLocatorMap{
			conf.IdentityLocation: conf.IdentityPublicKey,
		}
	}
	cfg := jem.ServerParams{
		DB: db,
		//TODO(mhilton) make DBSessionLimit configurable.
		DBSessionLimit:       500,
		ControllerAdmin:      conf.ControllerAdmin,
		IdentityLocation:     conf.IdentityLocation,
		PublicKeyLocator:     locator,
		AgentUsername:        conf.AgentUsername,
		AgentKey:             conf.AgentKey,
		RunMonitor:           true,
		ControllerUUID:       conf.ControllerUUID,
		DefaultCloud:         conf.DefaultCloud,
		WebsocketPingTimeout: websocketPingTimeout,
	}
	server, err := jem.NewServer(cfg)
	if err != nil {
		return errgo.Notef(err, "cannot create new server at %q", conf.APIAddr)
	}
	handler := server.(http.Handler)
	if conf.AccessLog != "" {
		accesslog := &lumberjack.Logger{
			Filename:   conf.AccessLog,
			MaxSize:    500, // megabytes
			MaxBackups: 3,
			MaxAge:     28, //days
		}
		handler = handlers.CombinedLoggingHandler(accesslog, handler)
	}

	logger.Infof("starting the API server")
	httpServer := &http.Server{
		Addr:      conf.APIAddr,
		Handler:   handler,
		TLSConfig: conf.TLSConfig(),
	}
	if httpServer.TLSConfig != nil {
		return httpServer.ListenAndServeTLS("", "")
	}
	return httpServer.ListenAndServe()
}
